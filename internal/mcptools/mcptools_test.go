package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

type fakeBackend struct {
	calls  []string
	waited time.Duration
	ask    spec.AskInput
}

func (f *fakeBackend) rec(name string) { f.calls = append(f.calls, name) }

func (f *fakeBackend) Show(_ context.Context, in ShowArgs) (any, error) {
	f.rec("show")
	return spec.ShowOutput{ThreadID: "t_1", PostID: "p_1"}, nil
}
func (f *fakeBackend) Ask(_ context.Context, in spec.AskInput) (any, error) {
	f.rec("ask")
	f.ask = in
	return spec.AskOutput{QuestionID: "q_1"}, nil
}
func (f *fakeBackend) Wait(_ context.Context, id string, d time.Duration) (any, error) {
	f.rec("wait")
	f.waited = d
	return spec.WaitOutput{QuestionID: id, Status: "open"}, nil
}
func (f *fakeBackend) Responses(context.Context) (any, error) {
	f.rec("responses")
	return spec.ResponsesOutput{}, nil
}
func (f *fakeBackend) Reply(context.Context, string, string) (any, error) {
	f.rec("reply")
	return nil, errors.New("reply r_9 not found")
}
func (f *fakeBackend) Done(context.Context, string, string) (any, error) {
	f.rec("done")
	return map[string]bool{"ok": true}, nil
}
func (f *fakeBackend) Threads(context.Context) (any, error) {
	f.rec("threads")
	return spec.ThreadList{}, nil
}
func (f *fakeBackend) Templates(context.Context, string) (any, error) {
	f.rec("templates")
	return spec.TemplateList{}, nil
}
func (f *fakeBackend) MakeTemplate(context.Context, json.RawMessage) (any, error) {
	f.rec("make-template")
	return spec.MakeTemplateOutput{}, nil
}
func (f *fakeBackend) Upload(context.Context, []media.FileRef) (any, error) {
	f.rec("upload")
	return spec.UploadResponse{}, nil
}

func connect(t *testing.T, remote bool) (*mcp.ClientSession, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{}
	s := mcp.NewServer(&mcp.Implementation{Name: "cadenza", Version: "test"}, &mcp.ServerOptions{Instructions: Instructions})
	Register(s, b, remote)
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, b
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(r *mcp.CallToolResult) string { return r.Content[0].(*mcp.TextContent).Text }

func TestToolsListMatchesSpec(t *testing.T) {
	for _, remote := range []bool{false, true} {
		cs, _ := connect(t, remote)
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]string{}
		for _, tool := range res.Tools {
			names[tool.Name] = tool.Description
			if len(tool.Description) > 1500 {
				t.Errorf("%s description is %d bytes; Claude Code cuts at about 2 KB", tool.Name, len(tool.Description))
			}
		}
		// Every agent endpoint is reachable as a tool (template is folded into templates,
		// responses-ack into responses).
		for _, e := range spec.Endpoints {
			if !e.Agent || e.Name == "template" || e.Name == "responses-ack" {
				continue
			}
			if _, ok := names[e.Name]; !ok {
				t.Errorf("remote=%v: no tool for agent endpoint %s", remote, e.Name)
			}
		}
		if _, ok := names["upload"]; ok != remote {
			t.Errorf("upload tool present=%v, want %v", ok, remote)
		}
	}
}

func TestToolCalls(t *testing.T) {
	cs, b := connect(t, true)
	if r := call(t, cs, "show", map[string]any{"title": "x", "template": "ui-change", "data": map[string]any{"summary": "s"}}); r.IsError || !strings.Contains(text(r), `"thread":"t_1"`) {
		t.Fatalf("show: %+v", text(r))
	}
	call(t, cs, "ask", map[string]any{"text": "Keep?", "choices": []string{"yes", "no"}})
	if !strings.HasPrefix(b.ask.RetryID, "auto-") {
		t.Fatalf("ask must get an automatic retry id, got %q", b.ask.RetryID)
	}
	first := b.ask.RetryID
	call(t, cs, "ask", map[string]any{"text": "Keep?", "choices": []string{"yes", "no"}})
	if b.ask.RetryID != first {
		t.Fatal("identical asks must share a retry id")
	}
	call(t, cs, "wait", map[string]any{"question_id": "q_1", "seconds": 600})
	if b.waited != MaxWait {
		t.Fatalf("wait must be capped at %v, got %v", MaxWait, b.waited)
	}
	call(t, cs, "wait", map[string]any{"question_id": "q_1", "seconds": 5})
	if b.waited != 5*time.Second {
		t.Fatal("short waits are kept")
	}
	if r := call(t, cs, "reply", map[string]any{"reply_id": "r_9", "text": "x"}); !r.IsError || text(r) != "reply r_9 not found" {
		t.Fatalf("errors must come back as tool errors: %+v", r)
	}
	call(t, cs, "upload", map[string]any{"files": []any{}})
	for _, name := range []string{"responses", "threads", "templates", "done"} {
		if r := call(t, cs, name, map[string]any{}); r.IsError {
			t.Fatalf("%s: %s", name, text(r))
		}
	}
	call(t, cs, "make-template", map[string]any{"definition": map[string]any{"name": "x"}})
	if len(b.calls) != 12 {
		t.Fatalf("calls %v", b.calls)
	}
}

func TestResultAndRetryID(t *testing.T) {
	if r := Result(map[string]int{"a": 1}, nil); r.IsError || text(r) != `{"a":1}` {
		t.Fatal("Result value")
	}
	if r := Result(nil, errors.New("boom")); !r.IsError || text(r) != "boom" {
		t.Fatal("Result error")
	}
	if r := Result(make(chan int), nil); !r.IsError {
		t.Fatal("unmarshalable value must be an error")
	}
	if RetryID("a") == RetryID("b") || RetryID("a") != RetryID("a") {
		t.Fatal("RetryID must be deterministic and distinct")
	}
}
