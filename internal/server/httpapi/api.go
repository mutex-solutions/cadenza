// Package httpapi serves Cadenza's HTTP API. Every route comes from
// spec.Endpoints; a test fails if an endpoint has no handler or a handler has
// no endpoint, so the API and its contract cannot drift (plan D4).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// CookieName holds the owner's web session token in browsers.
const CookieName = "cadenza_session"

// Limits.
const (
	maxBody     = 1 << 20 // JSON bodies; file bytes never come here
	maxWait     = 30 * time.Second
	defaultWait = 25 * time.Second
)

// Deps are the services the API serves.
type Deps struct {
	Access  *access.Service
	Review  *review.Service
	Files   *files.Service
	Catalog *catalog.Service
	Notify  *notify.Service
	Events  eventlog.Reader
	Bus     *eventlog.Bus
	Clock   kernel.Clock
	Log     *slog.Logger
	Version string
	// MinCLIVersion is the oldest CLI this server accepts; older CLIs are told to upgrade.
	MinCLIVersion string
	// SecureCookies sets the Secure flag (true when served over HTTPS).
	SecureCookies bool
}

// API is the HTTP handler for /v1.
type API struct {
	d       Deps
	mux     *http.ServeMux
	limiter *limiter
}

// handler is one endpoint's implementation.
type handler func(w http.ResponseWriter, r *http.Request, p *access.Principal) error

func New(d Deps) *API {
	a := &API{d: d, mux: http.NewServeMux(), limiter: newLimiter(d.Clock)}
	hs := a.handlers()
	for _, e := range spec.Endpoints {
		h, ok := hs[e.Name]
		if !ok {
			panic("httpapi: no handler for endpoint " + e.Name)
		}
		a.mux.Handle(e.Method+" "+e.Path, a.wrap(e, h))
	}
	a.mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, a.d.Log, kernel.NotFound("no such API call: %s %s", r.Method, r.URL.Path))
	})
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

// wrap applies authentication, rate limits and error handling.
func (a *API) wrap(e spec.Endpoint, h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && e.Input != nil && !isJSON(r) {
			writeErr(w, a.d.Log, kernel.Invalid("send the body as JSON").WithFix("set Content-Type: application/json"))
			return
		}
		if e.Auth == spec.AuthNone && r.Method == http.MethodPost && !a.limiter.allow(clientIP(r)+e.Name, 10, time.Minute) {
			writeErr(w, a.d.Log, &kernel.Error{Kind: kernel.KindForbidden, Msg: "too many attempts", Fix: "wait a minute and try again"})
			return
		}
		p, err := a.authenticate(r, e.Auth)
		if err != nil {
			writeErr(w, a.d.Log, err)
			return
		}
		if err := h(w, r, p); err != nil {
			writeErr(w, a.d.Log, err)
		}
	})
}

func isJSON(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/json")
}

func (a *API) authenticate(r *http.Request, need spec.Auth) (*access.Principal, error) {
	if need == spec.AuthNone {
		return nil, nil
	}
	token := bearer(r)
	if token == "" {
		if c, err := r.Cookie(CookieName); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		return nil, kernel.Unauthorized("no credentials").WithFix("log in, or pass a Cadenza key as a Bearer token")
	}
	p, err := a.d.Access.Authenticate(r.Context(), token)
	if err != nil {
		return nil, err
	}
	ok := false
	switch need {
	case spec.AuthOwner:
		ok = p.Role == access.RoleOwner
	case spec.AuthMachine:
		ok = p.Role == access.RoleMachine
	case spec.AuthSession:
		ok = p.Role == access.RoleSession
	case spec.AuthAny:
		ok = p.Role == access.RoleOwner || p.Role == access.RoleSession
	}
	if !ok {
		return nil, kernel.Forbidden("a %s credential cannot make this call", p.Role)
	}
	return p, nil
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(t)
	}
	return ""
}

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

// decode reads a JSON body into v.
func decode(r *http.Request, v any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBody)
	dec := json.NewDecoder(body)
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return kernel.Invalid("the request body is larger than 1 MB").WithFix("send files by upload, not inside JSON")
		}
		if errors.Is(err, io.EOF) {
			return kernel.Invalid("the request body is empty")
		}
		return kernel.Invalid("the request body is not valid JSON: %v", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		_, err := w.Write([]byte("{}\n"))
		return err
	}
	return json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error { return writeJSON(w, http.StatusOK, v) }

// StatusOf maps an error kind to an HTTP status.
func StatusOf(k kernel.Kind) int {
	switch k {
	case kernel.KindInvalid:
		return http.StatusBadRequest
	case kernel.KindNotFound:
		return http.StatusNotFound
	case kernel.KindConflict:
		return http.StatusConflict
	case kernel.KindUnauthorized:
		return http.StatusUnauthorized
	case kernel.KindForbidden:
		return http.StatusForbidden
	case kernel.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeErr(w http.ResponseWriter, log *slog.Logger, err error) {
	var ke *kernel.Error
	if !errors.As(err, &ke) {
		ke = kernel.Internal(err, "unexpected error")
	}
	body := spec.Error{Error: spec.ErrorBody{Kind: string(ke.Kind), Message: ke.Msg, Fix: ke.Fix}}
	if ke.Kind == kernel.KindInternal {
		log.Error("internal error", "error", err)
		body.Error.Message = "something went wrong on the server"
	}
	if errors.Is(err, context.Canceled) {
		return // the client went away
	}
	writeJSON(w, StatusOf(ke.Kind), body)
}

// actor turns a session principal into a review actor.
func actor(p *access.Principal) review.Actor {
	return review.Actor{Owner: p.Owner, SessionID: p.SessionID, MachineID: p.MachineID, MachineName: p.MachineName, Agent: p.Agent}
}
