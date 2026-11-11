package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/server/httpapi"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

type harness struct {
	*testkit.Env
	srv *httptest.Server
}

func newHarness(t *testing.T) *harness {
	e := testkit.New(t)
	api := httpapi.New(httpapi.Deps{Access: e.Access, Review: e.Review, Files: e.Files, Catalog: e.Catalog, Notify: e.Notify,
		Events: e.Store.Events(), Bus: e.Bus, Clock: kernel.SystemClock{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version: "test", MinCLIVersion: "0.1.0"})
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return &harness{Env: e, srv: srv}
}

// call sends a JSON request and decodes the JSON response into out (if non-nil).
func (h *harness) call(t *testing.T, method, path, token string, in, out any) (int, spec.ErrorBody) {
	t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, body)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		var e spec.Error
		json.Unmarshal(raw, &e)
		return res.StatusCode, e.Error
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return res.StatusCode, spec.ErrorBody{}
}

func (h *harness) mustCall(t *testing.T, method, path, token string, in, out any) {
	t.Helper()
	if code, e := h.call(t, method, path, token, in, out); code >= 300 {
		t.Fatalf("%s %s: %d %+v", method, path, code, e)
	}
}

func TestEveryEndpointIsRouted(t *testing.T) {
	h := newHarness(t)
	for _, e := range spec.Endpoints {
		path := strings.NewReplacer("{id}", "x", "{name}", "x", "{sha256}", "x").Replace(e.Path)
		req, _ := http.NewRequest(e.Method, h.srv.URL+path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body spec.Error
		json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if strings.HasPrefix(body.Error.Message, "no such API call") {
			t.Errorf("%s %s is not routed", e.Method, e.Path)
		}
	}
	code, e := h.call(t, "GET", "/v1/nope", "", nil, nil)
	if code != 404 || !strings.HasPrefix(e.Message, "no such API call") {
		t.Fatalf("unknown path: %d %+v", code, e)
	}
}

func TestSetupLoginAndCookies(t *testing.T) {
	h := newHarness(t)
	var v spec.Version
	h.mustCall(t, "GET", "/v1/version", "", nil, &v)
	if !v.SetupNeeded || v.API != 1 || v.MinCLIVersion != "0.1.0" {
		t.Fatalf("version %+v", v)
	}
	var login spec.LoginOutput
	h.mustCall(t, "POST", "/v1/auth/setup", "", spec.SetupInput{Password: "correct horse battery"}, &login)
	if !strings.HasPrefix(login.Token, "cz_ws_") {
		t.Fatalf("setup must log in: %+v", login)
	}
	if code, _ := h.call(t, "POST", "/v1/auth/setup", "", spec.SetupInput{Password: "another long one"}, nil); code != 409 {
		t.Fatalf("second setup: %d", code)
	}
	var me spec.Me
	h.mustCall(t, "GET", "/v1/me", login.Token, nil, &me)
	// Cookie login works for the web app.
	b, _ := json.Marshal(spec.LoginInput{Password: "correct horse battery"})
	res, err := http.Post(h.srv.URL+"/v1/auth/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == httpapi.CookieName {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie %+v", cookie)
	}
	req, _ := http.NewRequest("GET", h.srv.URL+"/v1/me", nil)
	req.AddCookie(cookie)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("cookie auth: %d", res.StatusCode)
	}
	h.mustCall(t, "POST", "/v1/auth/logout", login.Token, nil, nil)
	if code, _ := h.call(t, "GET", "/v1/me", login.Token, nil, nil); code != 401 {
		t.Fatal("logged-out token still works")
	}
}

func TestErrorsCarryKindAndFix(t *testing.T) {
	h := newHarness(t)
	code, e := h.call(t, "GET", "/v1/me", "", nil, nil)
	if code != 401 || e.Kind != "unauthorized" || e.Fix == "" {
		t.Fatalf("no credentials: %d %+v", code, e)
	}
	req, _ := http.NewRequest("POST", h.srv.URL+"/v1/auth/login", strings.NewReader("password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, _ := http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("form posts must be refused (CSRF): %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", h.srv.URL+"/v1/auth/login", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("bad JSON: %d", res.StatusCode)
	}
}

func TestRolesAreEnforced(t *testing.T) {
	h := newHarness(t)
	_, owner := h.Owner(t)
	mk := h.Machine(t, "vm-03")
	sk, _ := h.Session(t, mk, "s1")
	cases := []struct {
		method, path, token string
		want                int
	}{
		{"GET", "/v1/me", sk, 403},           // session is not the owner
		{"GET", "/v1/me", mk, 403},           // machine is not the owner
		{"GET", "/v1/responses", owner, 403}, // owner is not a session
		{"GET", "/v1/responses", mk, 403},    // machine keys only mint session keys
		{"GET", "/v1/templates", mk, 403},
		{"GET", "/v1/templates", sk, 200},
		{"GET", "/v1/templates", owner, 200},
		{"GET", "/v1/responses", sk, 200},
	}
	for _, c := range cases {
		if code, _ := h.call(t, c.method, c.path, c.token, nil, nil); code != c.want {
			t.Errorf("%s %s as %s: %d, want %d", c.method, c.path, c.token[:6], code, c.want)
		}
	}
	if code, _ := h.call(t, "POST", "/v1/sessions", owner, spec.SessionKeyInput{Agent: "a", AgentSessionID: "b"}, nil); code != 403 {
		t.Fatal("owner minted a session key")
	}
}

func TestPairingOverHTTP(t *testing.T) {
	h := newHarness(t)
	_, owner := h.Owner(t)
	var start spec.PairStartOutput
	h.mustCall(t, "POST", "/v1/pair", "", spec.PairStartInput{Hostname: "vm-03"}, &start)
	var poll spec.PairPollOutput
	h.mustCall(t, "POST", "/v1/pair/"+start.PairID+"/poll", "", spec.PairPollInput{Secret: start.Secret}, &poll)
	if poll.Status != spec.PairPending {
		t.Fatal("not pending")
	}
	h.mustCall(t, "POST", "/v1/pair/approve", owner, spec.PairApproveInput{Code: start.Code, Name: "vm-03"}, nil)
	h.mustCall(t, "POST", "/v1/pair/"+start.PairID+"/poll", "", spec.PairPollInput{Secret: start.Secret}, &poll)
	var sess spec.SessionKeyOutput
	h.mustCall(t, "POST", "/v1/sessions", poll.MachineKey, spec.SessionKeyInput{Agent: "claude-code", AgentSessionID: "abc"}, &sess)
	if !strings.HasPrefix(sess.SessionKey, "cz_sk_") {
		t.Fatalf("session key %+v", sess)
	}
	var machines spec.MachineList
	h.mustCall(t, "GET", "/v1/machines", owner, nil, &machines)
	if len(machines.Machines) != 1 || machines.Machines[0].Sessions != 1 {
		t.Fatalf("machines %+v", machines)
	}
	id := machines.Machines[0].ID
	h.mustCall(t, "POST", "/v1/machines/"+id+"/rename", owner, spec.RenameMachineInput{Name: "box"}, nil)
	var sessions spec.SessionList
	h.mustCall(t, "GET", "/v1/sessions", owner, nil, &sessions)
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].ID != sess.SessionID {
		t.Fatalf("sessions %+v", sessions)
	}
	var devices spec.DeviceList
	h.mustCall(t, "GET", "/v1/devices", owner, nil, &devices)
	if devices.Devices == nil {
		t.Fatal("devices must be a list")
	}
	h.mustCall(t, "DELETE", "/v1/sessions/"+sess.SessionID, owner, nil, nil)
	if code, _ := h.call(t, "GET", "/v1/responses", sess.SessionKey, nil, nil); code != 401 {
		t.Fatal("revoked session works")
	}
	h.mustCall(t, "DELETE", "/v1/machines/"+id, owner, nil, nil)
	if code, _ := h.call(t, "POST", "/v1/sessions", poll.MachineKey, spec.SessionKeyInput{Agent: "a", AgentSessionID: "b"}, nil); code != 401 {
		t.Fatal("removed machine works")
	}
}

func TestPairStartIsRateLimited(t *testing.T) {
	h := newHarness(t)
	limited := false
	for range 12 {
		if code, _ := h.call(t, "POST", "/v1/pair", "", spec.PairStartInput{Hostname: "x"}, nil); code == 403 {
			limited = true
		}
	}
	if !limited {
		t.Fatal("pair start must be rate limited")
	}
}

// The done-when scenario from the plan: an agent shows a ui-change and asks;
// the owner answers with a choice and a photo; the agent's wait returns with both.
func TestShowAskWaitAnswerFlow(t *testing.T) {
	h := newHarness(t)
	_, owner := h.Owner(t)
	mk := h.Machine(t, "vm-03")
	sk, _ := h.Session(t, mk, "s1")

	shots := []struct{ name, data string }{{"before.webp", "before"}, {"after.webp", "after"}}
	var refs []media.FileRef
	for _, s := range shots {
		refs = append(refs, media.FileRef{SHA256: sha(s.data), Size: int64(len(s.data)), Type: media.TypeWebP, Name: s.name})
	}
	var up spec.UploadResponse
	h.mustCall(t, "POST", "/v1/uploads", sk, spec.UploadRequest{Files: refs}, &up)
	if len(up.Uploads) != 2 {
		t.Fatalf("uploads %+v", up)
	}
	for _, s := range shots { // the CLI PUTs straight to storage
		h.Blobs.Put(sha(s.data), []byte(s.data))
	}
	data, _ := json.Marshal(map[string]any{"summary": "Promo field moved", "before": map[string]any{"file": refs[0]}, "after": map[string]any{"file": refs[1]}})
	var show spec.ShowOutput
	h.mustCall(t, "POST", "/v1/posts", sk, spec.ShowInput{Title: "Checkout redesign", RetryID: "r1",
		Attachments: []spec.Attachment{{Template: "ui-change", Data: data}}}, &show)
	var ask spec.AskOutput
	h.mustCall(t, "POST", "/v1/questions", sk, spec.AskInput{PostID: show.PostID, Text: "Keep promo visible?", Choices: []string{"yes", "no"}, Waiting: true, RetryID: "q1"}, &ask)

	var immediate spec.WaitOutput
	h.mustCall(t, "GET", "/v1/questions/"+ask.QuestionID+"/wait?timeout=0", sk, nil, &immediate)
	if immediate.Status != spec.QuestionOpen {
		t.Fatal("timeout=0 must return at once")
	}

	got := make(chan spec.WaitOutput, 1)
	go func() {
		var w spec.WaitOutput
		h.mustCall(t, "GET", "/v1/questions/"+ask.QuestionID+"/wait?timeout=10", sk, nil, &w)
		got <- w
	}()
	time.Sleep(100 * time.Millisecond) // let the wait start
	photo := media.FileRef{SHA256: sha("photo"), Size: 5, Type: media.TypeWebP, Name: "photo.webp"}
	h.Blobs.Put(photo.SHA256, []byte("photo"))
	start := time.Now()
	h.mustCall(t, "POST", "/v1/questions/"+ask.QuestionID+"/answer", owner, spec.AnswerInput{Choice: "yes", Files: []media.FileRef{photo}}, nil)
	select {
	case w := <-got:
		if w.Status != spec.QuestionAnswered || w.Answer == nil || w.Answer.Choice != "yes" || len(w.Answer.Files) != 1 {
			t.Fatalf("wait result %+v", w)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("answer took %v to reach the waiting agent (target: under 5 s)", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return after the answer")
	}

	var resp spec.ResponsesOutput
	h.mustCall(t, "GET", "/v1/responses", sk, nil, &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("responses %+v", resp)
	}
	h.mustCall(t, "POST", "/v1/responses/ack", sk, spec.AckInput{Cursor: resp.Cursor}, nil)
	h.mustCall(t, "GET", "/v1/responses", sk, nil, &resp)
	if len(resp.Items) != 0 {
		t.Fatal("acknowledged responses returned again")
	}

	var view spec.ThreadView
	h.mustCall(t, "GET", "/v1/threads/"+show.ThreadID, owner, nil, &view)
	if len(view.Posts) != 1 || len(view.Questions) != 1 || len(view.Replies) != 1 || !view.Replies[0].Delivered {
		t.Fatalf("thread view %+v", view)
	}
	h.mustCall(t, "POST", "/v1/threads/"+show.ThreadID+"/read", owner, nil, nil)
	var inbox spec.ThreadList
	h.mustCall(t, "GET", "/v1/threads", owner, nil, &inbox)
	if inbox.Threads[0].Unread {
		t.Fatal("thread-read did not mark the thread read")
	}
	var agentList spec.ThreadList
	h.mustCall(t, "GET", "/v1/threads", sk, nil, &agentList)
	if len(inbox.Threads) != 1 || len(agentList.Threads) != 1 {
		t.Fatal("thread lists")
	}
	var fu spec.FileURL
	h.mustCall(t, "GET", "/v1/files/"+photo.SHA256, owner, nil, &fu)
	if !strings.Contains(fu.URL, photo.SHA256) {
		t.Fatalf("file url %+v", fu)
	}
}

func TestWaitTimesOut(t *testing.T) {
	h := newHarness(t)
	h.Owner(t)
	mk := h.Machine(t, "vm-03")
	sk, _ := h.Session(t, mk, "s1")
	var ask spec.AskOutput
	h.mustCall(t, "POST", "/v1/questions", sk, spec.AskInput{Title: "T", Text: "q?", RetryID: "q1"}, &ask)
	start := time.Now()
	var w spec.WaitOutput
	h.mustCall(t, "GET", "/v1/questions/"+ask.QuestionID+"/wait?timeout=1", sk, nil, &w)
	if w.Status != spec.QuestionOpen || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("wait returned %s after %v", w.Status, time.Since(start))
	}
}

func TestEventsCatchUpAndStream(t *testing.T) {
	h := newHarness(t)
	_, owner := h.Owner(t)
	mk := h.Machine(t, "vm-03")
	sk, _ := h.Session(t, mk, "s1")
	var list spec.EventList
	h.mustCall(t, "GET", "/v1/events?after=0", owner, nil, &list)
	before := list.Last

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.srv.URL+"/v1/events/stream", nil)
	req.Header.Set("Authorization", "Bearer "+owner)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", res.Header.Get("Content-Type"))
	}
	lines := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	waitFor := func(prefix string) string {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case l := <-lines:
				if strings.HasPrefix(l, prefix) {
					return l
				}
			case <-deadline:
				t.Fatalf("no %q line", prefix)
			}
		}
	}
	waitFor("event: ready")
	h.mustCall(t, "POST", "/v1/questions", sk, spec.AskInput{Title: "T", Text: "q?", RetryID: "q1"}, nil)
	waitFor("event: change")
	data := waitFor("data: ")
	var ev spec.Event
	json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &ev)
	if ev.Seq <= before || ev.Kind == "" {
		t.Fatalf("event %+v", ev)
	}
	h.mustCall(t, "GET", "/v1/events?after="+itoa(before), owner, nil, &list)
	if len(list.Events) < 2 || list.Last <= before || list.Reset {
		t.Fatalf("catch up %+v", list)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
