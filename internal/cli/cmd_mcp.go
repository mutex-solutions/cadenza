package cli

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/mcptools"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// cmdMCP serves the tools on stdin/stdout. The session key is resolved per
// call, so a session id that changes (e.g. after /clear) is picked up.
func cmdMCP(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "cadenza", Version: a.Version}, &mcp.ServerOptions{Instructions: mcptools.Instructions})
	mcptools.Register(s, &localBackend{a: a, f: f}, false)
	return s.Run(ctx, &mcp.StdioTransport{})
}

// localBackend runs MCP tools through the same code as the CLI commands.
type localBackend struct {
	a *App
	f *flags
}

func (b *localBackend) client(ctx context.Context) (*client, error) { return b.a.agentClient(ctx, b.f) }

func (b *localBackend) Show(ctx context.Context, in mcptools.ShowArgs) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	f := &flags{values: map[string][]string{}, bools: map[string]bool{"new": in.NewThread}, pos: in.Files}
	set := func(k, v string) {
		if v != "" {
			f.values[k] = []string{v}
		}
	}
	set("title", in.Title)
	set("summary", in.Summary)
	set("template", in.Template)
	set("thread", in.ThreadID)
	set("notify-title", in.NotifyTitle)
	if d := in.DataJSON(); d != nil {
		set("data", string(d))
	}
	show, warnings, cleanup, err := b.a.buildShow(ctx, c, f)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	for i := range in.FileRefs {
		ref := in.FileRefs[i]
		show.Attachments = append(show.Attachments, spec.Attachment{File: &ref})
	}
	var out spec.ShowOutput
	if err := c.call(ctx, "show", nil, nil, show, &out); err != nil {
		return nil, err
	}
	return withWarnings(out, warnings), nil
}

func (b *localBackend) Ask(ctx context.Context, in spec.AskInput) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	var out spec.AskOutput
	return out, c.call(ctx, "ask", nil, nil, in, &out)
}

func (b *localBackend) Wait(ctx context.Context, id string, d time.Duration) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	var out spec.WaitOutput
	q := url.Values{"timeout": {strconv.Itoa(int(d / time.Second))}}
	if err := c.call(ctx, "wait", map[string]string{"id": id}, q, nil, &out); err != nil {
		return nil, err
	}
	if out.Answer != nil {
		if err := c.deliverAnswer(ctx, out.Answer); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (b *localBackend) Responses(ctx context.Context) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.responses(ctx, true)
}

func (b *localBackend) Reply(ctx context.Context, id, text string) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, c.call(ctx, "reply", map[string]string{"id": id}, nil, spec.AgentReplyInput{Text: text}, nil)
}

func (b *localBackend) Done(ctx context.Context, threadID, summary string) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	id := orDefault(threadID, review.LatestThread)
	return map[string]bool{"ok": true}, c.call(ctx, "done", map[string]string{"id": id}, nil, spec.DoneInput{Summary: summary}, nil)
}

func (b *localBackend) Threads(ctx context.Context) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	var out spec.ThreadList
	return out, c.call(ctx, "threads", nil, nil, nil, &out)
}

func (b *localBackend) Templates(ctx context.Context, name string) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	if name != "" {
		var out spec.TemplateInfo
		return out, c.call(ctx, "template", map[string]string{"name": name}, nil, nil, &out)
	}
	var out spec.TemplateList
	return out, c.call(ctx, "templates", nil, nil, nil, &out)
}

func (b *localBackend) MakeTemplate(ctx context.Context, def json.RawMessage) (any, error) {
	c, err := b.client(ctx)
	if err != nil {
		return nil, err
	}
	var out spec.MakeTemplateOutput
	return out, c.call(ctx, "make-template", nil, nil, spec.MakeTemplateInput{Definition: def}, &out)
}

func (b *localBackend) Upload(context.Context, []media.FileRef) (any, error) {
	return nil, kernel.Invalid("upload is not needed locally").WithFix("pass file paths to show")
}
