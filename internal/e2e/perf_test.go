package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Performance budgets from plan §12, measured on the CI runner. They run only
// with CADENZA_PERF=1 (CI runs them without -race, which slows everything).
//
//	server: inbox and thread load under 25 ms; under 60 MB memory idle
//	CLI: a templated show returns in under 300 ms (plus upload time)
//	answer delivery: under 5 s from the owner's tap to the agent's wait (TestDoneWhenScenario)
func TestPerformanceBudgets(t *testing.T) {
	if os.Getenv("CADENZA_PERF") != "1" {
		t.Skip("set CADENZA_PERF=1 to run the performance budgets")
	}
	s := start(t)

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("heap in use after start: %.1f MB", float64(ms.HeapInuse)/media.MB)
	if ms.HeapInuse > 60*media.MB {
		t.Errorf("idle memory %.1f MB is over the 60 MB budget", float64(ms.HeapInuse)/media.MB)
	}

	s.mustCadenza("login", s.url)
	s.mustCadenza("threads") // mints this agent session's key
	key := sessionKey(t, s)

	// A realistic inbox: 300 threads, one of them long (60 posts, questions, replies).
	page := uploadAsOwner(t, s, "report.html", []byte("<!doctype html><p>report"))
	page.Type = media.TypeHTML
	var long spec.ShowOutput
	for i := range 300 {
		in := spec.ShowInput{Title: fmt.Sprintf("Task %d", i), NewThread: true, RetryID: fmt.Sprintf("seed-%d", i),
			Summary: "Did **something** useful", Attachments: []spec.Attachment{{Template: "progress",
				Data: []byte(`{"summary":"Halfway there","done":["step one","step two"],"next":["step three"]}`)}}}
		agentCall(t, s, key, "POST", "/v1/posts", in, &long)
	}
	for i := range 59 {
		agentCall(t, s, key, "POST", "/v1/posts", spec.ShowInput{Title: fmt.Sprintf("Round %d", i+2), ThreadID: long.ThreadID,
			RetryID: fmt.Sprintf("long-%d", i), Attachments: []spec.Attachment{{File: &page}}}, nil)
		if i%10 == 0 {
			var q spec.AskOutput
			agentCall(t, s, key, "POST", "/v1/questions", spec.AskInput{ThreadID: long.ThreadID, Text: "Which?", Choices: []string{"a", "b"},
				RetryID: fmt.Sprintf("q-%d", i)}, &q)
			s.api("POST", "/v1/questions/"+q.QuestionID+"/answer", spec.AnswerInput{Choice: "a"}, nil)
		}
	}

	budget := func(name, path string, limit time.Duration) {
		t.Helper()
		var times []time.Duration
		for range 40 {
			start := time.Now()
			s.api("GET", path, nil, nil)
			times = append(times, time.Since(start))
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		p95 := times[len(times)*95/100]
		t.Logf("%s: median %v, p95 %v (budget %v)", name, times[len(times)/2], p95, limit)
		if p95 > limit {
			t.Errorf("%s p95 %v is over the %v budget", name, p95, limit)
		}
	}
	budget("inbox (300 threads)", "/v1/threads", 25*time.Millisecond)
	budget("thread (60 posts)", "/v1/threads/"+long.ThreadID, 25*time.Millisecond)

	var times []time.Duration
	for i := range 10 {
		start := time.Now()
		s.mustCadenza("show", "--title", fmt.Sprintf("Bug fix %d", i), "--template", "bug-fix", "--new",
			"--data", `{"symptom":"Checkout fails with an empty cart","cause":"Nil total","proof":{"language":"text","code":"ok  checkout  0.4s"}}`)
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	t.Logf("cadenza show (templated, no files): median %v, worst %v (budget 300ms)", times[len(times)/2], times[len(times)-1])
	if times[len(times)/2] > 300*time.Millisecond {
		t.Errorf("templated show median %v is over the 300 ms budget", times[len(times)/2])
	}
}

func sessionKey(t *testing.T, s *system) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(s.dir, "cfg", "sessions", "*.key"))
	if len(files) != 1 {
		t.Fatalf("session key files: %v", files)
	}
	b, _ := os.ReadFile(files[0])
	return strings.TrimSpace(string(b))
}

// agentCall calls the API with a session key.
func agentCall(t *testing.T, s *system, key, method, path string, in, out any) {
	t.Helper()
	owner := s.owner
	s.owner = key
	defer func() { s.owner = owner }()
	s.api(method, path, in, out)
}
