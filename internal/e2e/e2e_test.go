// Package e2e runs the whole system in one process: the real server (as
// cadenza-server builds it, with SQLite and trial-mode disk storage behind
// real HTTP) and the real CLI, with no fakes. It is the integration test for
// the plan's done-when scenario, and the home of the performance budgets
// (plan §12), which run when CADENZA_PERF=1.
package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/mutex-solutions/cadenza/internal/cli"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/server"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// system is a running server plus a CLI on an imaginary agent machine.
type system struct {
	t      *testing.T
	url    string
	owner  string // web session token
	dir    string
	stderr *lockedBuffer
	env    map[string]string
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func start(t *testing.T) *system {
	t.Helper()
	dir := t.TempDir()
	ts := httptest.NewUnstartedServer(nil)
	url := "http://" + ts.Listener.Addr().String()
	cfg, err := server.LoadConfig(func(k string) string {
		return map[string]string{"CADENZA_DATA_DIR": filepath.Join(dir, "data"), "CADENZA_PUBLIC_URL": url}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := server.New(context.Background(), cfg, "e2e", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = s.Handler()
	ts.Start()
	t.Cleanup(func() { ts.Close(); s.Close() })
	sys := &system{t: t, url: url, dir: dir, stderr: &lockedBuffer{}, env: map[string]string{"CLAUDE_CODE_SESSION_ID": "e2e-session"}}
	var login spec.LoginOutput
	sys.api("POST", "/v1/auth/setup", spec.SetupInput{Password: "correct horse battery"}, &login)
	sys.owner = login.Token
	return sys
}

// api calls the server as the owner (or anonymously before setup).
func (s *system) api(method, path string, in, out any) {
	s.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.url+path, body)
	req.Header.Set("Content-Type", "application/json")
	if s.owner != "" {
		req.Header.Set("Authorization", "Bearer "+s.owner)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		s.t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}

// cadenza runs the CLI on the agent machine and returns stdout and the exit code.
func (s *system) cadenza(args ...string) (string, int) {
	s.t.Helper()
	cwd := filepath.Join(s.dir, "work")
	os.MkdirAll(cwd, 0o755)
	var out bytes.Buffer
	app := &cli.App{Stdin: strings.NewReader(""), Stdout: &out, Stderr: s.stderr, Getenv: func(k string) string { return s.env[k] },
		HTTP: http.DefaultClient, ConfigDir: filepath.Join(s.dir, "cfg"), Cwd: cwd, Home: filepath.Join(s.dir, "home"),
		Version: "0.1.0", Hostname: "vm-03", Sleep: s.ownerApproves}
	code := app.Run(args)
	return out.String(), code
}

func (s *system) mustCadenza(args ...string) map[string]any {
	s.t.Helper()
	out, code := s.cadenza(args...)
	if code != 0 {
		s.t.Fatalf("cadenza %v: exit %d: %s %s", args, code, out, s.stderr.String())
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	return m
}

// ownerApproves is the owner typing the pairing code into the app while
// `cadenza login` waits.
func (s *system) ownerApproves(ctx context.Context, _ time.Duration) error {
	if m := regexp.MustCompile(`Pairing code: (\S+)`).FindStringSubmatch(s.stderr.String()); m != nil {
		s.api("POST", "/v1/pair/approve", spec.PairApproveInput{Code: m[1], Name: "vm-03"}, nil)
	}
	return nil
}

func (s *system) file(name string, data []byte) {
	p := filepath.Join(s.dir, "work", name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func screenshot(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 3), uint8(y * 5), uint8(x ^ y), 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// The plan's done-when scenario, end to end with nothing faked: an agent on a
// VM shows a ui-change and asks; the owner picks an answer and adds a photo;
// the agent continues with both.
func TestDoneWhenScenario(t *testing.T) {
	s := start(t)

	// The owner's browser registers for Web Push (the server made its keys on start).
	var push spec.PushConfig
	s.api("GET", "/v1/push/config", nil, &push)
	if push.VAPIDPublicKey == "" || push.Providers[0] != "webpush" {
		t.Fatalf("push config %+v", push)
	}

	s.mustCadenza("login", s.url)
	s.mustCadenza("setup")
	s.file("shots/before.png", screenshot(1600, 1000))
	s.file("shots/after.png", screenshot(1600, 1000))
	show := s.mustCadenza("show", "--title", "Checkout redesign", "--template", "ui-change", "--notify-title", "Checkout redesign",
		"--data", `{"summary":"Promo field below the total","before":{"file":"./shots/before.png"},"after":{"file":"./shots/after.png"},"changes":["Promo field moved"]}`)
	ask := s.mustCadenza("ask", "Keep the promo field visible?", "--choice", "yes", "--choice", "no", "--waiting", "--post", show["post"].(string))
	qid := ask["question"].(string)

	// The owner sees it in the inbox and opens the thread: both images load by signed link.
	var inbox spec.ThreadList
	s.api("GET", "/v1/threads", nil, &inbox)
	if len(inbox.Threads) != 1 || inbox.Threads[0].Agent != "claude-code" || inbox.Threads[0].Machine != "vm-03" || inbox.Threads[0].Open != 1 {
		t.Fatalf("inbox %+v", inbox)
	}
	var view spec.ThreadView
	s.api("GET", "/v1/threads/"+show["thread"].(string), nil, &view)
	var v struct {
		Slots []struct {
			Name  string `json:"name"`
			Value struct {
				File *struct {
					SHA256  string `json:"sha256"`
					Type    string `json:"type"`
					Preview string `json:"preview_sha256"`
				} `json:"file"`
			} `json:"value"`
		} `json:"slots"`
	}
	json.Unmarshal(view.Posts[0].Attachments[0].Data, &v)
	images := 0
	for _, slot := range v.Slots {
		if f := slot.Value.File; f != nil {
			images++
			for _, sha := range []string{f.SHA256, f.Preview} {
				var u spec.FileURL
				s.api("GET", "/v1/files/"+sha, nil, &u)
				res, err := http.Get(u.URL)
				if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/webp" {
					t.Fatalf("%s: GET signed link: %v %v %q", slot.Name, err, res.StatusCode, res.Header.Get("Content-Type"))
				}
				res.Body.Close()
			}
		}
	}
	if images != 2 {
		t.Fatalf("want 2 images, got %d", images)
	}

	// The owner answers with a choice and a photo taken on the phone (uploaded by signed link).
	photo := []byte("RIFF\x00\x00\x00\x00WEBPVP8 photo-from-the-phone")
	ref := uploadAsOwner(t, s, "photo.webp", photo)
	waitDone := make(chan struct{})
	var waitOut string
	var waitCode int
	go func() { waitOut, waitCode = s.cadenza("wait", qid, "--timeout", "30s"); close(waitDone) }()
	time.Sleep(200 * time.Millisecond)
	answered := time.Now()
	s.api("POST", "/v1/questions/"+qid+"/answer", spec.AnswerInput{Choice: "yes", Text: "and make it bold", Files: []media.FileRef{ref}}, nil)
	select {
	case <-waitDone:
	case <-time.After(10 * time.Second):
		t.Fatal("wait did not return")
	}
	if d := time.Since(answered); d > 5*time.Second {
		t.Fatalf("answer took %v to reach the agent; the target is under 5 s", d)
	}
	if waitCode != spec.ExitAnswered {
		t.Fatalf("wait exit %d: %s %s", waitCode, waitOut, s.stderr.String())
	}
	var w spec.WaitOutput
	json.Unmarshal([]byte(waitOut), &w)
	if w.Answer.Choice != "yes" || len(w.Answer.Files) != 1 {
		t.Fatalf("answer %+v", w.Answer)
	}
	got, err := os.ReadFile(filepath.Join(s.dir, "work", w.Answer.Files[0].Path))
	if err != nil || !bytes.Equal(got, photo) {
		t.Fatalf("the photo must be saved on the agent's machine: %v", err)
	}

	// The agent carries on: reads everything, shows round 2 and finishes.
	// wait already handed over (and acknowledged) the answer, so responses has nothing new.
	resp := s.mustCadenza("responses")
	if !strings.HasPrefix(resp["next_step"].(string), "Nothing new") {
		t.Fatalf("responses %v", resp)
	}
	s.mustCadenza("show", "--title", "Round 2: promo field in bold", "--template", "ui-change",
		"--data", `{"summary":"Bold promo field","after":{"file":"./shots/after.png"}}`)
	s.mustCadenza("done", "--summary", "Checkout redesign finished")
	s.api("GET", "/v1/threads/"+show["thread"].(string), nil, &view)
	if view.Thread.Status != spec.ThreadReady || len(view.Posts) != 2 || !view.Replies[0].Delivered {
		t.Fatalf("final thread %+v", view.Thread)
	}

	// Every send was attempted and logged (no device is registered, so nothing went out).
	var log spec.NotificationLog
	s.api("GET", "/v1/notifications", nil, &log)

	// The web app is served by the same server.
	res, err := http.Get(s.url + "/t/" + show["thread"].(string))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("web app: %v", err)
	}
	res.Body.Close()
}

// uploadAsOwner stores bytes the way the apps do: ask for a signed link, then
// PUT the bytes straight to storage with exactly the given headers.
func uploadAsOwner(t *testing.T, s *system, name string, data []byte) media.FileRef {
	t.Helper()
	sum := sha256.Sum256(data)
	ref := media.FileRef{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), Type: media.TypeWebP, Name: name}
	var up spec.UploadResponse
	s.api("POST", "/v1/uploads", spec.UploadRequest{Files: []media.FileRef{ref}}, &up)
	if len(up.Uploads) != 1 {
		t.Fatalf("uploads %+v", up)
	}
	req, _ := http.NewRequest(up.Uploads[0].Method, up.Uploads[0].URL, bytes.NewReader(data))
	for k, v := range up.Uploads[0].Headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode >= 300 {
		t.Fatalf("PUT: %v %v", err, res.StatusCode)
	}
	res.Body.Close()
	return ref
}
