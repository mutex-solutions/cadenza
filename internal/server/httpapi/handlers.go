package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// handlers maps every spec endpoint name to its implementation.
func (a *API) handlers() map[string]handler {
	return map[string]handler{
		// Agent calls.
		"show": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.ShowInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Review.Show(r.Context(), actor(p), in)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, out)
		},
		"ask": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.AskInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Review.Ask(r.Context(), actor(p), in)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, out)
		},
		"wait": a.wait,
		"responses": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Review.Responses(r.Context(), actor(p))
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"responses-ack": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.AckInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return done(w, a.d.Review.Ack(r.Context(), actor(p), in.Cursor))
		},
		"reply": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.AgentReplyInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return done(w, a.d.Review.AgentReply(r.Context(), actor(p), r.PathValue("id"), in.Text))
		},
		"done": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.DoneInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return done(w, a.d.Review.Done(r.Context(), actor(p), r.PathValue("id"), in.Summary))
		},
		"threads": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var out spec.ThreadList
			var err error
			if p.Role == access.RoleSession {
				out, err = a.d.Review.AgentThreads(r.Context(), actor(p))
			} else {
				out, err = a.d.Review.Inbox(r.Context(), p.Owner, r.URL.Query().Get("archived") == "1")
			}
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"templates": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Catalog.List(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"template": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Catalog.Get(r.Context(), p.Owner, r.PathValue("name"))
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"make-template": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.MakeTemplateInput
			if err := decode(r, &in); err != nil {
				return err
			}
			source := catalog.SourceOwner
			if p.Role == access.RoleSession {
				source = catalog.SourceAgent
			}
			out, err := a.d.Catalog.Make(r.Context(), p.Owner, source, in.Definition)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, out)
		},
		"upload": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.UploadRequest
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Files.PrepareUpload(r.Context(), p.Owner, in.Files)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"file-url": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Files.DownloadURL(r.Context(), p.Owner, r.PathValue("sha256"))
			if err != nil {
				return err
			}
			return ok(w, out)
		},

		// Pairing and keys.
		"pair-start": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			var in spec.PairStartInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Access.PairStart(r.Context(), in.Hostname)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, out)
		},
		"pair-poll": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			var in spec.PairPollInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Access.PairPoll(r.Context(), r.PathValue("id"), in.Secret)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"pair-approve": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.PairApproveInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Access.PairApprove(r.Context(), p.Owner, in.Code, in.Name)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"session-key": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.SessionKeyInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Access.MintSessionKey(r.Context(), p, in)
			if err != nil {
				return err
			}
			return ok(w, out)
		},

		// Owner.
		"version": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			has, err := a.d.Access.HasOwner(r.Context())
			if err != nil {
				return err
			}
			return ok(w, spec.Version{Server: a.d.Version, API: spec.APIVersion, MinCLIVersion: a.d.MinCLIVersion, SetupNeeded: !has})
		},
		"setup": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			var in spec.SetupInput
			if err := decode(r, &in); err != nil {
				return err
			}
			if err := a.d.Access.Setup(r.Context(), in.Password); err != nil {
				return err
			}
			return a.login(w, r, in.Password)
		},
		"login": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			var in spec.LoginInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return a.login(w, r, in.Password)
		},
		"logout": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			token := bearer(r)
			if c, err := r.Cookie(CookieName); err == nil && token == "" {
				token = c.Value
			}
			http.SetCookie(w, a.cookie("", -1))
			return done(w, a.d.Access.Logout(r.Context(), token))
		},
		"me": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return ok(w, spec.Me{OwnerID: string(p.Owner)})
		},
		"thread-view": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Review.ThreadView(r.Context(), p.Owner, r.PathValue("id"))
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"thread-read": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return done(w, a.d.Review.MarkRead(r.Context(), p.Owner, r.PathValue("id")))
		},
		"sessions": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Access.Sessions(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"devices": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Notify.Devices(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"thread-status": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.ThreadStatusInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return done(w, a.d.Review.SetThreadStatus(r.Context(), p.Owner, r.PathValue("id"), in.Status))
		},
		"answer": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.AnswerInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Review.Answer(r.Context(), p.Owner, r.PathValue("id"), in)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"dismiss": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return done(w, a.d.Review.Dismiss(r.Context(), p.Owner, r.PathValue("id")))
		},
		"add-reply": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.ReplyInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Review.AddReply(r.Context(), p.Owner, in)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, out)
		},
		"events":        a.events,
		"events-stream": a.stream,
		"machines": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Access.Machines(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"machine-rename": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.RenameMachineInput
			if err := decode(r, &in); err != nil {
				return err
			}
			return done(w, a.d.Access.RenameMachine(r.Context(), p.Owner, r.PathValue("id"), in.Name))
		},
		"machine-remove": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return done(w, a.d.Access.RemoveMachine(r.Context(), p.Owner, r.PathValue("id")))
		},
		"session-revoke": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return done(w, a.d.Access.RevokeSession(r.Context(), p.Owner, r.PathValue("id")))
		},
		"push-config": func(w http.ResponseWriter, r *http.Request, _ *access.Principal) error {
			return ok(w, a.d.Notify.Config())
		},
		"device-register": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			var in spec.DeviceInput
			if err := decode(r, &in); err != nil {
				return err
			}
			out, err := a.d.Notify.Register(r.Context(), p.Owner, in)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
		"device-remove": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			return done(w, a.d.Notify.Remove(r.Context(), p.Owner, r.PathValue("id")))
		},
		"notification-test": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			n, err := a.d.Notify.Test(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			if n == 0 {
				return kernel.Unavailable("no device received the test notification").
					WithFix("allow notifications in this app, then check the log for errors")
			}
			return ok(w, nil)
		},
		"notifications": func(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
			out, err := a.d.Notify.Log(r.Context(), p.Owner)
			if err != nil {
				return err
			}
			return ok(w, out)
		},
	}
}

func done(w http.ResponseWriter, err error) error {
	if err != nil {
		return err
	}
	return ok(w, nil)
}

func (a *API) login(w http.ResponseWriter, r *http.Request, password string) error {
	if !a.limiter.allow("login:"+clientIP(r), 5, time.Minute) {
		return &kernel.Error{Kind: kernel.KindForbidden, Msg: "too many login attempts", Fix: "wait a minute and try again"}
	}
	token, err := a.d.Access.Login(r.Context(), password)
	if err != nil {
		return err
	}
	http.SetCookie(w, a.cookie(token, int(access.WebSessionTTL/time.Second)))
	return ok(w, spec.LoginOutput{Token: token, Expires: kernel.Millis(a.d.Clock.Now().Add(access.WebSessionTTL))})
}

func (a *API) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true,
		Secure: a.d.SecureCookies, SameSite: http.SameSiteLaxMode}
}

// waitTimeout reads ?timeout=<seconds>, clamped to [0, maxWait].
func waitTimeout(r *http.Request) time.Duration {
	v := r.URL.Query().Get("timeout")
	if v == "" {
		return defaultWait
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultWait
	}
	d := time.Duration(n) * time.Second
	if d > maxWait {
		d = maxWait
	}
	return d
}
