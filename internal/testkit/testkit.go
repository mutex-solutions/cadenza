// Package testkit wires Cadenza's real services onto an in-memory SQLite
// database, with fakes only at the edges (object storage, push providers).
// Tests across packages use it so they exercise the same wiring as the server.
package testkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/sqlite"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Env is a fully wired set of services.
type Env struct {
	Store   *sqlite.Store
	Clock   *kernel.FakeClock
	Bus     *eventlog.Bus
	Blobs   *Blobs
	Push    *Push
	Access  *access.Service
	Review  *review.Service
	Files   *files.Service
	Catalog *catalog.Service
	Notify  *notify.Service
	BaseURL string
}

// Start is the fake clock's start time.
var Start = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

// New builds an Env with an empty database.
func New(t testing.TB) *Env {
	t.Helper()
	st, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &Env{Store: st, Clock: kernel.NewFakeClock(Start), Bus: eventlog.NewBus(), Blobs: NewBlobs(), Push: &Push{Name: "webpush"},
		BaseURL: "https://cadenza.test"}
	e.Access = access.NewService(st.Access(), e.Clock, e.Bus)
	e.Files = files.NewService(st.Files(), e.Blobs, e.Clock)
	e.Catalog = catalog.NewService(st.Catalog(), Markdown{}, e.Clock)
	e.Review = review.NewService(review.Deps{Store: st.Review(), Files: e.Files, Templates: e.Catalog, Renderer: Markdown{},
		Clock: e.Clock, Publisher: e.Bus, BaseURL: e.BaseURL})
	e.Notify = notify.NewService(st.Notify(), []notify.Sender{e.Push}, e.Review, e.Clock,
		slog.New(slog.NewTextHandler(io.Discard, nil)), "vapid-public")
	return e
}

// Markdown is a trivial renderer: escaped text in a paragraph.
type Markdown struct{}

func (Markdown) Render(md string) string { return "<p>" + html.EscapeString(md) + "</p>" }

// Owner sets up the owner and returns their id and a web session token.
func (e *Env) Owner(t testing.TB) (kernel.OwnerID, string) {
	t.Helper()
	ctx := context.Background()
	if err := e.Access.Setup(ctx, "correct horse battery"); err != nil && !kernel.Is(err, kernel.KindConflict) {
		t.Fatal(err)
	}
	tok, err := e.Access.Login(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Access.Authenticate(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	return p.Owner, tok
}

// Machine pairs a machine named name and returns its key.
func (e *Env) Machine(t testing.TB, name string) string {
	t.Helper()
	ctx := context.Background()
	owner, _ := e.Owner(t)
	start, err := e.Access.PairStart(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Access.PairApprove(ctx, owner, start.Code, name); err != nil {
		t.Fatal(err)
	}
	poll, err := e.Access.PairPoll(ctx, start.PairID, start.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return poll.MachineKey
}

// Session mints a session key on a machine and returns the key and the
// review actor it acts as.
func (e *Env) Session(t testing.TB, machineKey, agentSessionID string) (string, review.Actor) {
	t.Helper()
	ctx := context.Background()
	m, err := e.Access.Authenticate(ctx, machineKey)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Access.MintSessionKey(ctx, m, spec.SessionKeyInput{Agent: "claude-code", AgentSessionID: agentSessionID})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Access.Authenticate(ctx, out.SessionKey)
	if err != nil {
		t.Fatal(err)
	}
	return out.SessionKey, review.Actor{Owner: p.Owner, SessionID: p.SessionID, MachineID: p.MachineID, MachineName: p.MachineName, Agent: p.Agent}
}

// Upload stores bytes in the fake blob store as if a client had PUT them, and
// returns their reference.
func (e *Env) Upload(name, contentType string, data []byte) media.FileRef {
	sum := sha256.Sum256(data)
	ref := media.FileRef{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), Type: contentType, Name: name}
	e.Blobs.Put(ref.SHA256, data)
	return ref
}

// Blobs is an in-memory files.BlobStore. Signed links point at URL, which
// tests may set to an httptest server running Handler so clients can really
// PUT and GET bytes.
type Blobs struct {
	mu   sync.Mutex
	objs map[string][]byte
	Down bool
	URL  string
}

// Handler serves PUT <URL>/put/<sha> (checking the hash) and GET <URL>/get/<sha>/<name>.
func (b *Blobs) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodPut && len(parts) == 2 && parts[0] == "put":
			data, _ := io.ReadAll(r.Body)
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != parts[1] {
				http.Error(w, "BadDigest", http.StatusBadRequest)
				return
			}
			b.Put(parts[1], data)
		case r.Method == http.MethodGet && len(parts) >= 2 && parts[0] == "get":
			data, ok := b.Get(parts[1])
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(data)
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	})
}

func (b *Blobs) base() string {
	if b.URL != "" {
		return b.URL
	}
	return "https://blobs.test"
}

func NewBlobs() *Blobs { return &Blobs{objs: map[string][]byte{}} }

func (b *Blobs) Put(sha string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objs[sha] = data
}

func (b *Blobs) Get(sha string) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.objs[sha]
	return d, ok
}

func (b *Blobs) PresignPut(_ context.Context, ref media.FileRef, ttl time.Duration) (spec.Upload, error) {
	if b.Down {
		return spec.Upload{}, kernel.Unavailable("down")
	}
	return spec.Upload{SHA256: ref.SHA256, URL: b.base() + "/put/" + ref.SHA256, Method: "PUT",
		Headers: map[string]string{"Content-Type": ref.Type}}, nil
}

func (b *Blobs) PresignGet(_ context.Context, sha, typ, name string, ttl time.Duration) (spec.FileURL, error) {
	if b.Down {
		return spec.FileURL{}, kernel.Unavailable("down")
	}
	return spec.FileURL{URL: b.base() + "/get/" + sha + "/" + name}, nil
}

func (b *Blobs) Stat(_ context.Context, sha string) (int64, bool, error) {
	if b.Down {
		return 0, false, kernel.Unavailable("down")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.objs[sha]
	return int64(len(d)), ok, nil
}

// Push is a recording notify.Sender.
type Push struct {
	Name string
	mu   sync.Mutex
	Sent []notify.Message
	Gone bool
	Fail error
}

func (p *Push) Provider() string { return p.Name }

func (p *Push) Send(_ context.Context, _ json.RawMessage, msg notify.Message) (notify.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Fail != nil {
		return notify.Result{}, p.Fail
	}
	p.Sent = append(p.Sent, msg)
	return notify.Result{Gone: p.Gone}, nil
}

// Messages returns a copy of what was sent.
func (p *Push) Messages() []notify.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]notify.Message(nil), p.Sent...)
}
