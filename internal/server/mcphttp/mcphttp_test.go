package mcphttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/server/mcphttp"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func setup(t *testing.T) (*testkit.Env, *httptest.Server, string) {
	e := testkit.New(t)
	e.Owner(t)
	mk := e.Machine(t, "cloud-1")
	sk, _ := e.Session(t, mk, "cloud-session")
	srv := httptest.NewServer(mcphttp.New(mcphttp.Deps{Access: e.Access, Review: e.Review, Files: e.Files, Catalog: e.Catalog, Bus: e.Bus, Version: "test"}))
	t.Cleanup(srv.Close)
	return e, srv, sk
}

func connect(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	tr := &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "cloud-agent", Version: "1"}, nil).Connect(context.Background(), tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestRequiresSessionKey(t *testing.T) {
	e, srv, _ := setup(t)
	res, _ := http.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if res.StatusCode != 401 {
		t.Fatalf("no token: %d", res.StatusCode)
	}
	_, ownerTok := e.Owner(t)
	req, _ := http.NewRequest("POST", srv.URL, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 401 {
		t.Fatal("owner tokens are not agent sessions")
	}
}

func TestCloudAgentFlow(t *testing.T) {
	e, srv, sk := setup(t)
	cs := connect(t, srv.URL, sk)
	ref := media.FileRef{SHA256: strings.Repeat("d", 64), Size: 4, Type: media.TypeHTML, Name: "report.html"}
	out, isErr := call(t, cs, "upload", map[string]any{"files": []media.FileRef{ref}})
	if isErr || !strings.Contains(out, "blobs.test/put") {
		t.Fatalf("upload: %s", out)
	}
	e.Blobs.Put(ref.SHA256, []byte("<p>x"))
	if out, isErr = call(t, cs, "show", map[string]any{"title": "Report", "files": []string{"./a.html"}}); !isErr || !strings.Contains(out, "file_refs") {
		t.Fatalf("local paths must be refused with a fix: %s", out)
	}
	out, isErr = call(t, cs, "show", map[string]any{"title": "Report", "file_refs": []media.FileRef{ref}})
	if isErr || !strings.Contains(out, `"thread":"t_`) {
		t.Fatalf("show: %s", out)
	}
	again, _ := call(t, cs, "show", map[string]any{"title": "Report", "file_refs": []media.FileRef{ref}})
	if again != out {
		t.Fatal("an identical show must not duplicate")
	}
	out, _ = call(t, cs, "ask", map[string]any{"text": "Ship it?", "choices": []string{"yes", "no"}})
	if !strings.Contains(out, `"question":"q_`) {
		t.Fatalf("ask: %s", out)
	}
	qid := out[strings.Index(out, "q_") : strings.Index(out, "q_")+18]
	start := time.Now()
	out, _ = call(t, cs, "wait", map[string]any{"question_id": qid, "seconds": 1})
	if !strings.Contains(out, `"status":"open"`) || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("wait: %s after %v", out, time.Since(start))
	}
	owner, _ := e.Owner(t)
	if _, err := e.Review.Answer(context.Background(), owner, qid, spec.AnswerInput{Choice: "yes"}); err != nil {
		t.Fatal(err)
	}
	out, _ = call(t, cs, "wait", map[string]any{"question_id": qid})
	if !strings.Contains(out, `"status":"answered"`) {
		t.Fatalf("answered wait: %s", out)
	}
	out, _ = call(t, cs, "responses", map[string]any{})
	if !strings.Contains(out, `"kind":"answer"`) {
		t.Fatalf("responses: %s", out)
	}
	out, _ = call(t, cs, "responses", map[string]any{})
	if strings.Contains(out, `"kind":"answer"`) {
		t.Fatal("responses must be acknowledged once returned")
	}
	if out, isErr = call(t, cs, "done", map[string]any{}); isErr {
		t.Fatalf("done: %s", out)
	}
	if out, isErr = call(t, cs, "templates", map[string]any{"name": "ui-change"}); isErr || !strings.Contains(out, "guide") {
		t.Fatalf("templates: %s", out)
	}
	if out, isErr = call(t, cs, "threads", map[string]any{}); isErr || !strings.Contains(out, "Report") {
		t.Fatalf("threads: %s", out)
	}
	if out, isErr = call(t, cs, "make-template", map[string]any{"definition": map[string]any{"name": "x"}}); !isErr {
		t.Fatalf("bad template accepted: %s", out)
	}
	if out, isErr = call(t, cs, "reply", map[string]any{"reply_id": "r_nope", "text": "x"}); !isErr {
		t.Fatalf("unknown reply: %s", out)
	}
}
