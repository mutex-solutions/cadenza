package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/server/httpapi"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

// world is a real API server (in-memory database, fake storage endpoint)
// plus a CLI configured to talk to it.
type world struct {
	*testkit.Env
	t      *testing.T
	srv    *httptest.Server
	owner  kernel.OwnerID
	env    map[string]string
	stderr *syncBuffer
	dir    string
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func newWorld(t *testing.T) *world {
	e := testkit.New(t)
	blobSrv := httptest.NewServer(e.Blobs.Handler())
	t.Cleanup(blobSrv.Close)
	e.Blobs.URL = blobSrv.URL
	api := httpapi.New(httpapi.Deps{Access: e.Access, Review: e.Review, Files: e.Files, Catalog: e.Catalog, Notify: e.Notify,
		Events: e.Store.Events(), Bus: e.Bus, Clock: kernel.SystemClock{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version: "test", MinCLIVersion: "0.1.0"})
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	owner, _ := e.Owner(t)
	dir := t.TempDir()
	return &world{Env: e, t: t, srv: srv, owner: owner, dir: dir, stderr: &syncBuffer{},
		env: map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1"}}
}

// app returns a CLI whose files live in the world's temp dir.
func (w *world) app(stdout io.Writer) *App {
	cwd := filepath.Join(w.dir, "work")
	os.MkdirAll(cwd, 0o755)
	return &App{Stdin: strings.NewReader(""), Stdout: stdout, Stderr: w.stderr, Getenv: func(k string) string { return w.env[k] },
		HTTP: http.DefaultClient, ConfigDir: filepath.Join(w.dir, "config"), Cwd: cwd, Home: filepath.Join(w.dir, "home"),
		Version: "0.1.0", Hostname: "vm-03", Sleep: w.approveOnSleep}
}

// approveOnSleep stands in for the owner: when login waits, approve the code it printed.
func (w *world) approveOnSleep(ctx context.Context, _ time.Duration) error {
	m := regexp.MustCompile(`Pairing code: (\S+)`).FindStringSubmatch(w.stderr.String())
	if m != nil {
		w.Access.PairApprove(ctx, w.owner, m[1], "")
	}
	return nil
}

// run runs the CLI and returns stdout and the exit code.
func (w *world) run(args ...string) (string, int) {
	w.t.Helper()
	var out bytes.Buffer
	code := w.app(&out).Run(args)
	return out.String(), code
}

func (w *world) mustRun(args ...string) map[string]any {
	w.t.Helper()
	out, code := w.run(args...)
	if code != 0 {
		w.t.Fatalf("cadenza %v: exit %d\nstdout: %s\nstderr: %s", args, code, out, w.stderr.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		w.t.Fatalf("cadenza %v: output is not JSON: %q", args, out)
	}
	return m
}

func (w *world) login() {
	w.t.Helper()
	m := w.mustRun("login", w.srv.URL)
	if m["paired"] != "vm-03" {
		w.t.Fatalf("login: %v", m)
	}
}

func (w *world) writeFile(name string, data []byte) string {
	p := filepath.Join(w.dir, "work", name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		w.t.Fatal(err)
	}
	return name
}

func pngBytes(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestLoginStoresKeysPrivately(t *testing.T) {
	w := newWorld(t)
	w.login()
	if !strings.Contains(w.stderr.String(), "Pairing code:") {
		t.Fatal("login must show the code")
	}
	for _, f := range []string{"config.json", "machine.key"} {
		st, err := os.Stat(filepath.Join(w.dir, "config", f))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v mode %v", f, err, st.Mode())
		}
	}
	st := w.mustRun("status")
	if st["machine"] != "vm-03" || st["agent"] != "claude-code" || st["agent_session"] != "sess-1" {
		t.Fatalf("status %v", st)
	}
	out := w.mustRun("logout")
	if out["logged_out"] == nil {
		t.Fatal("logout")
	}
	if _, code := w.run("threads"); code != spec.ExitError {
		t.Fatal("after logout, commands must fail")
	}
	if !strings.Contains(w.stderr.String(), "cadenza login") {
		t.Fatal("the error must say how to fix it")
	}
}

func TestLoginErrors(t *testing.T) {
	w := newWorld(t)
	if _, code := w.run("login"); code != spec.ExitError {
		t.Fatal("login without URL")
	}
	if _, code := w.run("login", "not a url"); code != spec.ExitError {
		t.Fatal("bad URL")
	}
	if _, code := w.run("login", "http://127.0.0.1:1"); code != spec.ExitUnreachable {
		t.Fatal("unreachable server must exit 5")
	}
}

// The plan's done-when scenario, through the CLI.
func TestShowAskWaitResponses(t *testing.T) {
	w := newWorld(t)
	w.login()
	w.writeFile("shots/before.png", pngBytes(t, 3000, 1600))
	w.writeFile("shots/after.png", pngBytes(t, 400, 300))
	data := `{"summary":"Promo field below total","before":{"file":"./shots/before.png"},"after":{"file":"./shots/after.png"},"changes":["Promo field moved"]}`
	show := w.mustRun("show", "--title", "Checkout redesign", "--template", "ui-change", "--data", data)
	thread, post := show["thread"].(string), show["post"].(string)
	if !strings.HasPrefix(thread, "t_") || !strings.HasPrefix(post, "p_") {
		t.Fatalf("show %v", show)
	}
	again := w.mustRun("show", "--title", "Checkout redesign", "--template", "ui-change", "--data", data)
	if again["post"] != post {
		t.Fatal("repeating the same show must not duplicate it")
	}
	v, err := w.Review.ThreadView(context.Background(), w.owner, thread)
	if err != nil {
		t.Fatal(err)
	}
	view := string(v.Posts[0].Attachments[0].Data)
	if !strings.Contains(view, `"image/webp"`) || !strings.Contains(view, "before.webp") || !strings.Contains(view, "preview_sha256") {
		t.Fatalf("PNG must arrive as WebP with a preview: %s", view)
	}
	ask := w.mustRun("ask", "Keep promo visible?", "--choice", "yes", "--choice", "no", "--waiting", "--post", post)
	qid := ask["question"].(string)
	if ask["thread"] != thread || !strings.Contains(ask["next"].(string), "cadenza wait") {
		t.Fatalf("ask %v", ask)
	}
	if out, code := w.run("wait", qid, "--timeout", "0"); code != spec.ExitStillOpen || !strings.Contains(out, `"status":"open"`) {
		t.Fatalf("wait --timeout 0: %d %s", code, out)
	}
	photo := w.Upload("photo.webp", "image/webp", []byte("photo-bytes"))
	go func() {
		time.Sleep(200 * time.Millisecond)
		w.Review.Answer(context.Background(), w.owner, qid, spec.AnswerInput{Choice: "yes", Text: "make it bold", Files: []media.FileRef{photo}})
	}()
	out, code := w.run("wait", qid, "--timeout", "10s")
	if code != spec.ExitAnswered {
		t.Fatalf("wait: exit %d %s %s", code, out, w.stderr.String())
	}
	var wo spec.WaitOutput
	json.Unmarshal([]byte(out), &wo)
	if wo.Answer == nil || wo.Answer.Choice != "yes" || len(wo.Answer.Files) != 1 || wo.Answer.Files[0].Path == "" {
		t.Fatalf("answer %s", out)
	}
	saved, err := os.ReadFile(filepath.Join(w.dir, "work", wo.Answer.Files[0].Path))
	if err != nil || string(saved) != "photo-bytes" {
		t.Fatalf("photo not saved: %v %q", err, saved)
	}
	// wait acknowledged the answer after saving the photo: responses does not
	// repeat it, and the owner's app shows it as delivered.
	if items := w.mustRun("responses")["items"].([]any); len(items) != 0 {
		t.Fatalf("the answer wait returned must not come back in responses: %v", items)
	}
	tv, _ := w.Review.ThreadView(context.Background(), w.owner, thread)
	if len(tv.Replies) != 1 || !tv.Replies[0].Delivered {
		t.Fatalf("the answer must show as delivered: %+v", tv.Replies)
	}
	threads := w.mustRun("threads")["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("threads %v", threads)
	}
	w.mustRun("done", "--summary", "shipped")
	v, _ = w.Review.ThreadView(context.Background(), w.owner, thread)
	if v.Thread.Status != spec.ThreadReady {
		t.Fatalf("done: status %s", v.Thread.Status)
	}
}

func TestShowPlainFilesAndTextSummary(t *testing.T) {
	w := newWorld(t)
	w.login()
	w.writeFile("report.html", []byte("<!doctype html><title>r</title><p>all good"))
	w.writeFile("notes.md", []byte("# Notes\nTwo fixes."))
	show := w.mustRun("show", "--title", "Perf report", "report.html", "notes.md")
	v, _ := w.Review.ThreadView(context.Background(), w.owner, show["thread"].(string))
	p := v.Posts[0]
	if len(p.Attachments) != 1 || p.Attachments[0].File.Type != "text/html" || !strings.Contains(p.SummaryHTML, "Two fixes") {
		t.Fatalf("post %+v", p)
	}
	w.writeFile("flow.mmd", []byte("graph TD; a-->b"))
	if _, code := w.run("show", "--title", "x", "flow.mmd"); code != spec.ExitError || !strings.Contains(w.stderr.String(), "needs a template") {
		t.Fatal("diagram source files must point at templates")
	}
	w.writeFile(".env", []byte("SECRET=1"))
	if _, code := w.run("show", "--title", "x", ".env"); code != spec.ExitError {
		t.Fatal(".env must be refused")
	}
}

func TestShowErrorsGiveFixes(t *testing.T) {
	w := newWorld(t)
	w.login()
	cases := [][]string{
		{"show", "x.png"},                                   // no title
		{"show", "--title", "x"},                            // nothing to show
		{"show", "--title", "x", "--data", "{}"},            // data without template
		{"show", "--title", "x", "--template", "ui-change"}, // template without data
		{"show", "--title", "x", "--template", "ui-change", "--data", "{"},
		{"show", "--title", "x", "--template", "ui-change", "--data", `{"after":{"file":"./a.png"}}`}, // missing required slot
		{"show", "--title", "x", "--bogus"},
	}
	for _, c := range cases {
		before := len(w.stderr.String())
		if _, code := w.run(c...); code != spec.ExitError {
			t.Errorf("%v: want exit 1, got %d", c, code)
			continue
		}
		line := w.stderr.String()[before:]
		var e map[string]string
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &e) != nil || e["error"] == "" {
			t.Errorf("%v: stderr is not a JSON error: %q", c, line)
		}
	}
}

func TestTemplatesAndCustomTemplate(t *testing.T) {
	w := newWorld(t)
	w.login()
	list := w.mustRun("templates")["templates"].([]any)
	if len(list) != 15 {
		t.Fatalf("templates %d", len(list))
	}
	if info := w.mustRun("templates", "bug-fix"); info["guide"] == "" {
		t.Fatal("template guide")
	}
	def := `{"name":"db-migration","use_for":"Schema changes","guide":"Say what changes.","slots":[{"name":"summary","block":"text","required":true}],"example":{"summary":"Add index"}}`
	w.writeFile("t.json", []byte(def))
	if out := w.mustRun("make-template", "t.json"); out["version"] != float64(1) {
		t.Fatalf("make-template %v", out)
	}
	// A custom template is usable at once, validated against its definition from the server.
	w.mustRun("show", "--title", "Migration", "--template", "db-migration", "--data", `{"summary":"Add index on t(c)"}`, "--new")
	if _, code := w.run("show", "--title", "x", "--template", "db-migration", "--data", `{"nope":1}`); code != spec.ExitError {
		t.Fatal("custom template validation")
	}
}

func TestReplyAndCommentFlow(t *testing.T) {
	w := newWorld(t)
	w.login()
	w.writeFile("a.html", []byte("<p>x"))
	show := w.mustRun("show", "--title", "x", "a.html")
	rep, err := w.Review.AddReply(context.Background(), w.owner, spec.ReplyInput{PostID: show["post"].(string), Kind: spec.ReplyComment,
		Text: "move left", Spot: &spec.Spot{X: 0.1, Y: 0.2}})
	if err != nil {
		t.Fatal(err)
	}
	resp := w.mustRun("responses")
	if !strings.HasPrefix(resp["next_step"].(string), "Read each item") {
		t.Fatalf("next step %v", resp["next_step"])
	}
	w.mustRun("reply", rep.ReplyID, "fixed", "in", "round", "2")
	v, _ := w.Review.ThreadView(context.Background(), w.owner, show["thread"].(string))
	if v.Replies[0].AgentReply != "fixed in round 2" {
		t.Fatalf("agent reply %+v", v.Replies[0])
	}
}

func TestWaitExitCodes(t *testing.T) {
	w := newWorld(t)
	w.login()
	q1 := w.mustRun("ask", "First?", "--title", "T")["question"].(string)
	q2 := w.mustRun("ask", "Second?", "--replaces", q1)["question"].(string)
	if out, code := w.run("wait", q1); code != spec.ExitReplaced || !strings.Contains(out, q2) {
		t.Fatalf("replaced: %d %s", code, out)
	}
	w.Review.Dismiss(context.Background(), w.owner, q2)
	if _, code := w.run("wait", q2, "--timeout", "1m"); code != spec.ExitDismissed {
		t.Fatalf("dismissed: %d", code)
	}
	if _, code := w.run("wait"); code != spec.ExitError {
		t.Fatal("wait without id")
	}
	if _, code := w.run("wait", q2, "--timeout", "soon"); code != spec.ExitError {
		t.Fatal("bad timeout")
	}
}

func TestSessionKeysAreRenewedAutomatically(t *testing.T) {
	w := newWorld(t)
	w.login()
	w.mustRun("threads")
	files, _ := os.ReadDir(filepath.Join(w.dir, "config", "sessions"))
	if len(files) != 1 {
		t.Fatalf("session key files: %d", len(files))
	}
	key, _ := os.ReadFile(filepath.Join(w.dir, "config", "sessions", files[0].Name()))
	p, _ := w.Access.Authenticate(context.Background(), strings.TrimSpace(string(key)))
	w.Access.RevokeSession(context.Background(), w.owner, p.SessionID)
	w.mustRun("threads") // renews the revoked key, same session
	// Another agent session gets its own key.
	w.env["CLAUDE_CODE_SESSION_ID"] = "sess-2"
	w.mustRun("threads")
	files, _ = os.ReadDir(filepath.Join(w.dir, "config", "sessions"))
	if len(files) != 2 {
		t.Fatalf("want one key per agent session, got %d", len(files))
	}
}

func TestNoSessionIDExplainsFix(t *testing.T) {
	w := newWorld(t)
	w.login()
	delete(w.env, "CLAUDE_CODE_SESSION_ID")
	if _, code := w.run("threads"); code != spec.ExitError || !strings.Contains(w.stderr.String(), "--session") {
		t.Fatal("missing session must explain --session")
	}
	w.mustRun("threads", "--session", "hermes-123", "--agent-name", "hermes")
	w.env["CADENZA_SESSION"] = "env-session"
	w.mustRun("threads")
}

func TestServerDownExitsFive(t *testing.T) {
	w := newWorld(t)
	w.login()
	w.srv.Close()
	if _, code := w.run("ask", "x?"); code != spec.ExitUnreachable {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(w.stderr.String(), "unreachable") {
		t.Fatal("must say unreachable")
	}
}

func TestSetupInstallsGuideIdempotently(t *testing.T) {
	w := newWorld(t)
	claude := filepath.Join(w.dir, "home", ".claude", "CLAUDE.md")
	os.MkdirAll(filepath.Dir(claude), 0o755)
	os.WriteFile(claude, []byte("# My rules\nBe nice."), 0o644)
	out := w.mustRun("setup")
	if len(out["installed"].(map[string]any)) != 3 {
		t.Fatalf("setup %v", out)
	}
	w.mustRun("setup")
	b, _ := os.ReadFile(claude)
	text := string(b)
	if strings.Count(text, markStart) != 1 || !strings.HasPrefix(text, "# My rules\nBe nice.") || !strings.Contains(text, "@"+filepath.Join(w.dir, "config", "cadenza.md")) {
		t.Fatalf("CLAUDE.md:\n%s", text)
	}
	codex, _ := os.ReadFile(filepath.Join(w.dir, "home", ".codex", "AGENTS.md"))
	if !strings.Contains(string(codex), "cadenza wait") {
		t.Fatal("agents without imports get a copy of the guide")
	}
	w.mustRun("setup", "--project", "--agent", "codex")
	if _, err := os.Stat(filepath.Join(w.dir, "work", "AGENTS.md")); err != nil {
		t.Fatal("--project writes into the working directory")
	}
	if _, code := w.run("setup", "--agent", "vim"); code != spec.ExitError {
		t.Fatal("unknown agent")
	}
	var buf bytes.Buffer
	w.app(&buf).Run([]string{"setup", "--print"})
	if !strings.HasPrefix(buf.String(), "<!-- Generated") {
		t.Fatal("setup --print prints the guide")
	}
}

func TestHelpAndVersion(t *testing.T) {
	w := newWorld(t)
	out, code := w.run("help")
	if code != 0 || !strings.Contains(out, "cadenza show") || !strings.Contains(out, "cadenza wait") {
		t.Fatalf("help: %s", out)
	}
	if _, code := w.run("frobnicate"); code != spec.ExitError {
		t.Fatal("unknown command")
	}
	if v := w.mustRun("version"); v["cadenza"] != "0.1.0" {
		t.Fatal("version")
	}
}
