package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

type captured struct {
	mu     sync.Mutex
	header http.Header
	body   []byte
}

func pushServer(t *testing.T, status int, c *captured) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.header, c.body = r.Header.Clone(), b
		c.mu.Unlock()
		w.Header().Set("Location", "https://push.example/m/1")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func subscription(t *testing.T, endpoint string) json.RawMessage {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	b, _ := json.Marshal(map[string]any{
		"endpoint": endpoint,
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(auth),
		},
	})
	return b
}

func newSender(t *testing.T, client *http.Client) (*Sender, string) {
	t.Helper()
	pub, priv, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{VAPIDPublicKey: pub, VAPIDPrivateKey: priv, Subscriber: "mailto:ops@example.com", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	return s, pub
}

var msg = notify.Message{Title: "claude · laptop — done", Body: "ready", URL: "/t/t_81"}

func TestSendOK(t *testing.T) {
	var c captured
	ts := pushServer(t, http.StatusCreated, &c)
	s, pub := newSender(t, ts.Client())
	if s.Provider() != "webpush" {
		t.Fatal(s.Provider())
	}
	res, err := s.Send(context.Background(), subscription(t, ts.URL+"/push/abc"), msg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Gone || res.ProviderID != "https://push.example/m/1" {
		t.Fatalf("res = %+v", res)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	auth := c.header.Get("Authorization")
	if !strings.HasPrefix(auth, "vapid t=") || !strings.HasSuffix(auth, ", k="+pub) {
		t.Fatalf("Authorization = %q", auth)
	}
	jwt := strings.TrimSuffix(strings.TrimPrefix(auth, "vapid t="), ", k="+pub)
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt = %q", jwt)
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var cl struct{ Sub, Aud string }
	_ = json.Unmarshal(claims, &cl)
	if cl.Sub != "mailto:ops@example.com" || cl.Aud != ts.URL {
		t.Fatalf("claims = %s", claims)
	}
	if len(c.body) == 0 || strings.Contains(string(c.body), "ready") {
		t.Fatal("body empty or not encrypted")
	}
	if c.header.Get("Content-Encoding") != "aes128gcm" || c.header.Get("TTL") != "86400" || c.header.Get("Urgency") != "high" {
		t.Fatalf("headers = %v", c.header)
	}
}

func TestSendGone(t *testing.T) {
	for _, code := range []int{http.StatusGone, http.StatusNotFound} {
		var c captured
		ts := pushServer(t, code, &c)
		s, _ := newSender(t, ts.Client())
		res, err := s.Send(context.Background(), subscription(t, ts.URL), msg)
		if err != nil || !res.Gone {
			t.Fatalf("%d: res=%+v err=%v", code, res, err)
		}
	}
}

func TestSendServerError(t *testing.T) {
	var c captured
	ts := pushServer(t, http.StatusBadGateway, &c)
	s, _ := newSender(t, ts.Client())
	_, err := s.Send(context.Background(), subscription(t, ts.URL), msg)
	if !kernel.Is(err, kernel.KindUnavailable) || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestSendTransportError(t *testing.T) {
	var c captured
	ts := pushServer(t, http.StatusCreated, &c)
	s, _ := newSender(t, http.DefaultClient) // does not trust the test certificate
	_, err := s.Send(context.Background(), subscription(t, ts.URL), msg)
	if !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestBadKeysFromLibrary(t *testing.T) {
	s, _ := newSender(t, nil)
	tok := json.RawMessage(`{"endpoint":"https://push.example/x","keys":{"p256dh":"!!","auth":"!!"}}`)
	if _, err := s.Send(context.Background(), tok, msg); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidToken(t *testing.T) {
	s, _ := newSender(t, nil)
	cases := []string{
		`not json`,
		`"a string"`,
		`{"endpoint":"http://push.example/x","keys":{"p256dh":"a","auth":"b"}}`,
		`{"endpoint":"https://","keys":{"p256dh":"a","auth":"b"}}`,
		`{"endpoint":"https://push.example/x","keys":{"p256dh":"","auth":"b"}}`,
		`{"endpoint":"https://push.example/x"}`,
	}
	for _, tok := range cases {
		if _, err := s.Send(context.Background(), json.RawMessage(tok), msg); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v", tok, err)
		}
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New(Config{Subscriber: "mailto:a@b.c"}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("missing keys: %v", err)
	}
	if _, err := New(Config{VAPIDPublicKey: "a", VAPIDPrivateKey: "b"}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("missing subscriber: %v", err)
	}
	s, err := New(Config{VAPIDPublicKey: "a", VAPIDPrivateKey: "b", Subscriber: "https://example.com", TTL: time.Minute})
	if err != nil || s.cfg.TTL != time.Minute || s.cfg.HTTPClient == nil {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestGenerateVAPIDKeys(t *testing.T) {
	pub, priv, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	p, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil || len(p) != 65 || p[0] != 4 {
		t.Fatalf("public key %q: %v", pub, err)
	}
	k, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil || len(k) != 32 {
		t.Fatalf("private key %q: %v", priv, err)
	}
}
