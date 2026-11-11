package cli

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/cli/prepare"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

func cmdAsk(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"choice", "post", "thread", "title", "replaces", "notify-title", "retry-id", "text"}, []string{"waiting", "new"})
	if err != nil {
		return err
	}
	text := f.str("text")
	if text == "" {
		text = strings.Join(f.pos, " ")
	}
	if strings.TrimSpace(text) == "" {
		return kernel.Invalid("the question is empty").WithFix(`cadenza ask "your question" [--choice A --choice B]`)
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	in := spec.AskInput{ThreadID: f.str("thread"), PostID: f.str("post"), Text: prepare.Redact(text), Choices: f.all("choice"),
		Waiting: f.bool("waiting"), Replaces: f.str("replaces"), Notify: f.str("notify-title"), NewThread: f.bool("new"), Title: f.str("title"),
		RetryID: f.str("retry-id")}
	if in.RetryID == "" {
		in.RetryID = retryID(c.sess.ID, "ask", in.Text, in.Choices, in.ThreadID, in.PostID, in.Replaces, in.NewThread)
	}
	var out spec.AskOutput
	if err := c.call(ctx, "ask", nil, nil, in, &out); err != nil {
		return err
	}
	return a.print(map[string]string{"question": out.QuestionID, "thread": out.ThreadID, "url": out.URL,
		"next": "keep working; run `cadenza wait " + out.QuestionID + "` in the background to get the answer"})
}

// serverWait is how long one wait request is held by the server.
const serverWait = 25 * time.Second

func cmdWait(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"timeout"}, nil)
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return kernel.Invalid("wait needs one question id").WithFix("cadenza wait q_… [--timeout 10m]")
	}
	limit, forever, err := parseTimeout(f.str("timeout"))
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	id := f.pos[0]
	deadline := time.Now().Add(limit)
	for {
		hold := serverWait
		if !forever {
			hold = min(hold, time.Until(deadline))
		}
		if hold < 0 {
			hold = 0
		}
		q := url.Values{"timeout": {strconv.Itoa(int(hold.Round(time.Second) / time.Second))}}
		var out spec.WaitOutput
		if err := c.call(ctx, "wait", map[string]string{"id": id}, q, nil, &out); err != nil {
			return err
		}
		switch out.Status {
		case spec.QuestionAnswered:
			if out.Answer != nil {
				if err := a.saveResponseFiles(ctx, out.Answer); err != nil {
					return err
				}
			}
			return a.exit(out, spec.ExitAnswered)
		case spec.QuestionDismissed:
			return a.exit(out, spec.ExitDismissed)
		case spec.QuestionReplaced:
			return a.exit(out, spec.ExitReplaced)
		}
		if !forever && !time.Now().Before(deadline) {
			return a.exit(out, spec.ExitStillOpen)
		}
	}
}

// parseTimeout reads --timeout: a Go duration ("10m", "1h30m"), plain
// seconds, or "none"/empty to wait until answered.
func parseTimeout(v string) (time.Duration, bool, error) {
	switch v {
	case "", "none", "never":
		return 0, true, nil
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, false, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, false, kernel.Invalid("--timeout %q is not a duration", v).WithFix("use e.g. 30s, 10m or 2h, or leave it out to wait until answered")
	}
	return d, false, nil
}

// exit prints v and ends with code.
func (a *App) exit(v any, code int) error {
	if err := a.print(v); err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}
