package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadConfigDefaults(t *testing.T) {
	c, err := LoadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.PublicURL != "http://localhost:8080" || c.Database != "sqlite:./data/cadenza.db" || !c.Trial() || c.Secure() {
		t.Fatalf("defaults %+v", c)
	}
	if c.ExpoEnabled {
		t.Fatal("Expo is off unless configured")
	}
}

func TestLoadConfigValidation(t *testing.T) {
	bad := []map[string]string{
		{"CADENZA_PUBLIC_URL": "cadenza.example.com"},
		{"CADENZA_DATABASE": "mysql://x"},
		{"CADENZA_S3_BUCKET": "b"},
		{"CADENZA_S3_CHECKSUM": "md5"},
	}
	for _, m := range bad {
		if _, err := LoadConfig(env(m)); err == nil {
			t.Errorf("%v accepted", m)
		}
	}
	c, err := LoadConfig(env(map[string]string{"CADENZA_PUBLIC_URL": "https://c.example.com/", "CADENZA_S3_BUCKET": "b",
		"CADENZA_S3_ACCESS_KEY": "a", "CADENZA_S3_SECRET_KEY": "s", "CADENZA_EXPO_ACCESS_TOKEN": "t", "CADENZA_DATABASE": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicURL != "https://c.example.com" || !c.Secure() || c.Trial() || !c.ExpoEnabled {
		t.Fatalf("config %+v", c)
	}
}

func newServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg, err := LoadConfig(env(map[string]string{"CADENZA_DATA_DIR": dir}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestServerWiresEverything(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	for path, want := range map[string]int{"/healthz": 200, "/v1/version": 200, "/": 200, "/t/t_abc": 200, "/v1/me": 401, "/mcp": 401} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("GET %s: %d, want %d", path, res.StatusCode, want)
		}
		if res.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("GET %s: security headers missing", path)
		}
	}
	// Trial mode serves blobs from the server itself.
	res, _ := http.Get(srv.URL + "/blob/" + strings.Repeat("a", 64))
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("unsigned blob request: %d", res.StatusCode)
	}
}

func TestSecretsPersistAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadConfig(env(map[string]string{"CADENZA_DATA_DIR": dir}))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s1, err := New(context.Background(), cfg, "test", log)
	if err != nil {
		t.Fatal(err)
	}
	pub1 := s1.notify.Config().VAPIDPublicKey
	s1.Close()
	s2, err := New(context.Background(), cfg, "test", log)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if pub1 == "" || s2.notify.Config().VAPIDPublicKey != pub1 {
		t.Fatal("Web Push keys must be generated once and kept")
	}
	if err := s2.ResetPassword(context.Background(), "a long new password"); err != nil {
		t.Fatal(err)
	}
}

func TestCSPAllowsObjectStorage(t *testing.T) {
	s := &Server{cfg: Config{S3: S3Config{PublicEndpoint: "https://s3.example.com", Bucket: "b"}}}
	csp := s.contentSecurityPolicy()
	if !strings.Contains(csp, "connect-src 'self' https://s3.example.com") || !strings.Contains(csp, "img-src 'self' data: blob: https://s3.example.com") {
		t.Fatalf("csp %q", csp)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	s := newServer(t)
	s.cfg.Addr = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
