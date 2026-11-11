package expopush

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

const goodToken = `"ExponentPushToken[abc123_-XYZ]"`

// fake returns a server that answers each request with the next handler
// output and records every request body.
func fake(t *testing.T, h http.HandlerFunc) (*httptest.Server, *Sender, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var slept []time.Duration
	s := New(Config{
		BaseURL:     srv.URL,
		AccessToken: "tok-1",
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		},
	})
	return srv, s, &slept
}

func TestSendOK(t *testing.T) {
	var got []map[string]any
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/--/api/v2/push/send" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if a := r.Header.Get("Authorization"); a != "Bearer tok-1" {
			t.Errorf("Authorization = %q", a)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("body: %v", err)
		}
		io.WriteString(w, `{"data":[{"status":"ok","id":"tkt-9"}]}`)
	})
	res, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{
		Title: "claude · laptop — done", Body: "b", ThreadID: "t_1", URL: "/t/t_1",
		Data: map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProviderID != "tkt-9" || res.Gone {
		t.Fatalf("res = %+v", res)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages", len(got))
	}
	m := got[0]
	want := map[string]any{
		"to": "ExponentPushToken[abc123_-XYZ]", "title": "claude · laptop — done", "body": "b",
		"sound": "default", "priority": "high", "channelId": "default",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	data := m["data"].(map[string]any)
	if data["thread_id"] != "t_1" || data["url"] != "/t/t_1" || data["k"] != "v" {
		t.Errorf("data = %v", data)
	}
}

func TestSendNoAccessTokenNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header")
		}
		b, _ := io.ReadAll(r.Body)
		var msgs []map[string]any
		json.Unmarshal(b, &msgs)
		if _, ok := msgs[0]["data"]; ok {
			t.Errorf("empty data should be omitted")
		}
		// Single-object form of data.
		io.WriteString(w, `{"data":{"status":"ok","id":"x"}}`)
	}))
	defer srv.Close()
	res, err := New(Config{BaseURL: srv.URL + "/"}).Send(context.Background(), json.RawMessage(`"ExpoPushToken[a]"`), notify.Message{Title: "t"})
	if err != nil || res.ProviderID != "x" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestSendGone(t *testing.T) {
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"status":"error","message":"not registered","details":{"error":"DeviceNotRegistered"}}]}`)
	})
	res, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if err != nil || !res.Gone {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestSendTicketError(t *testing.T) {
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"status":"error","message":"too big","details":{"error":"MessageTooBig"}}]}`)
	})
	_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSendRequestErrors(t *testing.T) {
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"errors":[{"code":"VALIDATION_ERROR","message":"bad"}]}`)
	})
	_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSendBadResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"empty list":       {200, `{"data":[]}`},
		"no ticket":        {200, `{"data":"nope"}`},
		"not json":         {200, `<html>`},
		"plain 400":        {400, `bad`},
		"unauthorized":     {401, `{"message":"nope"}`},
		"missing data":     {200, `{}`},
		"object no status": {200, `{"data":{}}`},
	} {
		t.Run(name, func(t *testing.T) {
			_, s, slept := fake(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
			if !kernel.Is(err, kernel.KindUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if len(*slept) != 0 {
				t.Fatalf("should not retry, slept %v", *slept)
			}
		})
	}
}

func TestSendRetriesThenOK(t *testing.T) {
	var n atomic.Int32
	_, s, slept := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			io.WriteString(w, `{"data":[{"status":"ok","id":"t3"}]}`)
		}
	})
	res, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if err != nil || res.ProviderID != "t3" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; len(*slept) != 2 || (*slept)[0] != want[0] || (*slept)[1] != want[1] {
		t.Fatalf("slept %v", *slept)
	}
}

func TestSendRetriesBounded(t *testing.T) {
	var n atomic.Int32
	_, s, slept := fake(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 4 || len(*slept) != 3 {
		t.Fatalf("requests=%d sleeps=%d", n.Load(), len(*slept))
	}
}

func TestNoRetries(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	s := New(Config{BaseURL: srv.URL, MaxRetries: -1})
	if _, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{}); err == nil {
		t.Fatal("want error")
	}
	if n.Load() != 1 {
		t.Fatalf("requests = %d", n.Load())
	}
}

func TestSleepErrorStopsRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	s := New(Config{BaseURL: srv.URL, Sleep: func(context.Context, time.Duration) error { return errors.New("stop") }})
	_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestNetworkErrorRetried(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	var slept int
	s := New(Config{BaseURL: url, MaxRetries: 2, Sleep: func(context.Context, time.Duration) error { slept++; return nil }})
	_, err := s.Send(context.Background(), json.RawMessage(goodToken), notify.Message{})
	if !kernel.Is(err, kernel.KindUnavailable) || slept != 2 {
		t.Fatalf("err=%v slept=%d", err, slept)
	}
}

func TestInvalidTokens(t *testing.T) {
	s := New(Config{BaseURL: "http://127.0.0.1:1"})
	for _, tok := range []string{`123`, `"abc"`, `"ExponentPushToken[]"`, `"ExponentPushToken[a b]"`, `not json`, `"ExponentPushToken[a]x"`} {
		_, err := s.Send(context.Background(), json.RawMessage(tok), notify.Message{})
		if !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v", tok, err)
		}
	}
}

func TestProviderAndDefaults(t *testing.T) {
	s := New(Config{})
	if s.Provider() != "expo" || s.base != DefaultBaseURL || s.retries != 3 || s.client == nil {
		t.Fatalf("defaults wrong: %+v", s)
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); err == nil {
		t.Fatal("want error on cancelled context")
	}
}

func TestTruncate(t *testing.T) {
	b := make([]byte, 300)
	if len(truncate(b)) != 200 {
		t.Fatal("not truncated")
	}
}
