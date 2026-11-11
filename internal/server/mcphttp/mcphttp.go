// Package mcphttp serves Cadenza's MCP tools over Streamable HTTP at /mcp,
// for cloud agents with nothing installed (plan D3). Callers authenticate
// with a session key as a Bearer token; files are uploaded by signed link
// (the upload tool) before they are shown.
package mcphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/mcptools"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Deps are the services the tools call.
type Deps struct {
	Access  *access.Service
	Review  *review.Service
	Files   *files.Service
	Catalog *catalog.Service
	Bus     *eventlog.Bus
	Version string
}

type principalKey struct{}

// New returns the /mcp handler.
func New(d Deps) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := r.Context().Value(principalKey{}).(*access.Principal)
		s := mcp.NewServer(&mcp.Implementation{Name: "cadenza", Version: d.Version}, &mcp.ServerOptions{Instructions: mcptools.Instructions})
		mcptools.Register(s, &backend{d: d, p: p}, true)
		return s
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		p, err := d.Access.Authenticate(r.Context(), strings.TrimSpace(token))
		if err != nil || p.Role != access.RoleSession {
			w.Header().Set("WWW-Authenticate", `Bearer realm="cadenza"`)
			http.Error(w, "a Cadenza session key is required as a Bearer token", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// backend runs tools as one agent session, calling the services directly.
type backend struct {
	d Deps
	p *access.Principal
}

func (b *backend) actor() review.Actor {
	return review.Actor{Owner: b.p.Owner, SessionID: b.p.SessionID, MachineID: b.p.MachineID, MachineName: b.p.MachineName, Agent: b.p.Agent}
}

func (b *backend) Show(ctx context.Context, in mcptools.ShowArgs) (any, error) {
	if len(in.Files) > 0 {
		return nil, kernel.Invalid("this server cannot read files on your machine").
			WithFix("upload them with the upload tool and pass file_refs, or run `cadenza mcp` locally")
	}
	show := spec.ShowInput{ThreadID: in.ThreadID, NewThread: in.NewThread, Title: in.Title, Summary: in.Summary, Notify: in.NotifyTitle}
	if in.Template != "" {
		show.Attachments = append(show.Attachments, spec.Attachment{Template: in.Template, Data: in.DataJSON()})
	}
	for i := range in.FileRefs {
		f := in.FileRefs[i]
		show.Attachments = append(show.Attachments, spec.Attachment{File: &f})
	}
	show.RetryID = mcptools.RetryID(in)
	return b.d.Review.Show(ctx, b.actor(), show)
}

func (b *backend) Ask(ctx context.Context, in spec.AskInput) (any, error) {
	return b.d.Review.Ask(ctx, b.actor(), in)
}

// Wait polls the question until it changes or d passes.
func (b *backend) Wait(ctx context.Context, id string, d time.Duration) (any, error) {
	deadline := time.Now().Add(d)
	for {
		out, _, err := b.d.Review.QuestionState(ctx, b.actor(), id)
		if err != nil || out.Status != spec.QuestionOpen || !time.Now().Before(deadline) {
			return out, err
		}
		select {
		case <-ctx.Done():
			return out, nil
		case <-time.After(min(time.Second, time.Until(deadline))):
		}
	}
}

// Responses returns and acknowledges new replies; a cloud agent downloads
// any files from the signed links itself.
func (b *backend) Responses(ctx context.Context) (any, error) {
	out, err := b.d.Review.Responses(ctx, b.actor())
	if err != nil {
		return nil, err
	}
	if len(out.Items) > 0 {
		if err := b.d.Review.Ack(ctx, b.actor(), out.Cursor); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (b *backend) Reply(ctx context.Context, id, text string) (any, error) {
	return map[string]bool{"ok": true}, b.d.Review.AgentReply(ctx, b.actor(), id, text)
}

func (b *backend) Done(ctx context.Context, threadID, summary string) (any, error) {
	return map[string]bool{"ok": true}, b.d.Review.Done(ctx, b.actor(), threadID, summary)
}

func (b *backend) Threads(ctx context.Context) (any, error) {
	return b.d.Review.AgentThreads(ctx, b.actor())
}

func (b *backend) Templates(ctx context.Context, name string) (any, error) {
	if name != "" {
		return b.d.Catalog.Get(ctx, b.p.Owner, name)
	}
	return b.d.Catalog.List(ctx, b.p.Owner)
}

func (b *backend) MakeTemplate(ctx context.Context, def json.RawMessage) (any, error) {
	return b.d.Catalog.Make(ctx, b.p.Owner, catalog.SourceAgent, def)
}

func (b *backend) Upload(ctx context.Context, refs []media.FileRef) (any, error) {
	return b.d.Files.PrepareUpload(ctx, b.p.Owner, refs)
}
