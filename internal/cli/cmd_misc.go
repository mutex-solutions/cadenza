package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

func cmdResponses(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, []string{"peek"})
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	out, err := c.responses(ctx, !f.bool("peek"))
	if err != nil {
		return err
	}
	return a.print(out)
}

// responses fetches new replies, saves their files, then acknowledges them,
// so nothing is lost if this process dies halfway (plan gap G-6).
func (c *client) responses(ctx context.Context, ack bool) (spec.ResponsesOutput, error) {
	var out spec.ResponsesOutput
	if err := c.call(ctx, "responses", nil, nil, nil, &out); err != nil {
		return out, err
	}
	for i := range out.Items {
		if err := c.a.saveResponseFiles(ctx, &out.Items[i]); err != nil {
			return out, err
		}
	}
	if ack && len(out.Items) > 0 {
		if err := c.call(ctx, "responses-ack", nil, nil, spec.AckInput{Cursor: out.Cursor}, nil); err != nil {
			return out, err
		}
	}
	return out, nil
}

func cmdReply(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	if len(f.pos) < 2 {
		return kernel.Invalid("reply needs a reply id and text").WithFix(`cadenza reply r_… "fixed in round 2"`)
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	if err := c.call(ctx, "reply", map[string]string{"id": f.pos[0]}, nil, spec.AgentReplyInput{Text: strings.Join(f.pos[1:], " ")}, nil); err != nil {
		return err
	}
	return a.print(map[string]bool{"ok": true})
}

func cmdDone(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"thread", "summary"}, nil)
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	id := orDefault(f.str("thread"), review.LatestThread)
	if err := c.call(ctx, "done", map[string]string{"id": id}, nil, spec.DoneInput{Summary: f.str("summary")}, nil); err != nil {
		return err
	}
	return a.print(map[string]bool{"ok": true})
}

func cmdThreads(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	var out spec.ThreadList
	if err := c.call(ctx, "threads", nil, nil, nil, &out); err != nil {
		return err
	}
	return a.print(out)
}

func cmdTemplates(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	if len(f.pos) > 0 {
		var out spec.TemplateInfo
		if err := c.call(ctx, "template", map[string]string{"name": f.pos[0]}, nil, nil, &out); err != nil {
			return err
		}
		return a.print(out)
	}
	var out spec.TemplateList
	if err := c.call(ctx, "templates", nil, nil, nil, &out); err != nil {
		return err
	}
	return a.print(out)
}

func cmdMakeTemplate(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return kernel.Invalid("make-template needs a JSON file, or - for stdin").WithFix("cadenza make-template ./template.json")
	}
	var b []byte
	if f.pos[0] == "-" {
		b, err = io.ReadAll(io.LimitReader(a.Stdin, 1<<20))
	} else {
		b, err = os.ReadFile(a.abs(f.pos[0]))
	}
	if err != nil {
		return kernel.Invalid("reading the template: %v", err)
	}
	if !json.Valid(b) {
		return kernel.Invalid("the template is not valid JSON")
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	var out spec.MakeTemplateOutput
	if err := c.call(ctx, "make-template", nil, nil, spec.MakeTemplateInput{Definition: b}, &out); err != nil {
		return err
	}
	return a.print(out)
}

func cmdVersion(_ context.Context, a *App, _ []string) error {
	return a.print(map[string]string{"cadenza": a.Version})
}

func cmdStatus(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, nil, nil)
	if err != nil {
		return err
	}
	cfg, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	out := map[string]any{"cli": a.Version, "server": cfg.Server, "machine": cfg.MachineName}
	var v spec.Version
	if err := a.do(ctx, cfg.Server, "version", nil, nil, "", nil, &v); err != nil {
		return err
	}
	out["server_version"] = v.Server
	if olderThan(a.Version, v.MinCLIVersion) {
		out["update"] = "this cadenza is older than the server needs (" + v.MinCLIVersion + "); reinstall it with install.sh"
	}
	if sess, err := a.resolveSession(f); err == nil {
		out["agent"], out["agent_session"] = sess.Agent, sess.ID
	} else {
		out["agent_session"] = "none found: pass --session <id>"
	}
	return a.print(out)
}

// olderThan compares dotted versions like "0.3.1"; "dev" is never older.
func olderThan(v, min string) bool {
	if v == "dev" || min == "" {
		return false
	}
	pv, pm := strings.Split(strings.TrimPrefix(v, "v"), "."), strings.Split(strings.TrimPrefix(min, "v"), ".")
	for i := 0; i < len(pv) || i < len(pm); i++ {
		var a, b int
		if i < len(pv) {
			a = atoi(pv[i])
		}
		if i < len(pm) {
			b = atoi(pm[i])
		}
		if a != b {
			return a < b
		}
	}
	return false
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
