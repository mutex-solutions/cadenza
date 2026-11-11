package fcm

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKey = k
	})
	return testKey
}

func pkcs8PEM(t *testing.T, k *rsa.PrivateKey) string {
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

func saJSON(t *testing.T, tokenURI, keyPEM string) []byte {
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "proj-1",
		"private_key_id": "kid-1",
		"private_key":    keyPEM,
		"client_email":   "push@proj-1.iam.gserviceaccount.com",
		"token_uri":      tokenURI,
	})
	return b
}

// fakeGoogle is a token endpoint and an FCM endpoint in one server.
type fakeGoogle struct {
	t          *testing.T
	pub        *rsa.PublicKey
	aud        string
	tokenCalls atomic.Int32
	sendCalls  atomic.Int32
	send       func(w http.ResponseWriter, r *http.Request, body map[string]any)
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/token":
		f.tokenCalls.Add(1)
		f.checkToken(w, r)
	case r.URL.Path == "/v1/projects/proj-1/messages:send":
		f.sendCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer at-1" {
			f.t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &body); err != nil {
			f.t.Errorf("send body: %v", err)
		}
		f.send(w, r, body)
	default:
		f.t.Errorf("unexpected path %s", r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}
}

func (f *fakeGoogle) checkToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.t.Error(err)
	}
	if g := r.PostForm.Get("grant_type"); g != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		f.t.Errorf("grant_type = %q", g)
	}
	parts := strings.Split(r.PostForm.Get("assertion"), ".")
	if len(parts) != 3 {
		f.t.Errorf("assertion has %d parts", len(parts))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, sum[:], sig); err != nil {
		f.t.Errorf("bad JWT signature: %v", err)
	}
	var header, claims map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(hb, &header)
	json.Unmarshal(cb, &claims)
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "kid-1" {
		f.t.Errorf("header = %v", header)
	}
	if claims["iss"] != "push@proj-1.iam.gserviceaccount.com" || claims["scope"] != Scope || claims["aud"] != f.aud {
		f.t.Errorf("claims = %v", claims)
	}
	iat, exp := claims["iat"].(float64), claims["exp"].(float64)
	if exp-iat != 3600 {
		f.t.Errorf("exp-iat = %v", exp-iat)
	}
	io.WriteString(w, `{"access_token":"at-1","expires_in":3600,"token_type":"Bearer"}`)
}

func newFake(t *testing.T, send func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*fakeGoogle, *Sender, *kernel.FakeClock) {
	t.Helper()
	k := rsaKey(t)
	f := &fakeGoogle{t: t, pub: &k.PublicKey, send: send}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	f.aud = srv.URL + "/token"
	clock := kernel.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := New(Config{ServiceAccountJSON: saJSON(t, f.aud, pkcs8PEM(t, k)), Endpoint: srv.URL + "/", Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return f, s, clock
}

func okSend(w http.ResponseWriter, r *http.Request, body map[string]any) {
	io.WriteString(w, `{"name":"projects/proj-1/messages/0:123"}`)
}

func TestSendOK(t *testing.T) {
	var got map[string]any
	_, s, _ := newFake(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		got = body
		okSend(w, r, body)
	})
	res, err := s.Send(context.Background(), json.RawMessage(`"reg-token-1"`), notify.Message{
		Title: "t", Body: "b", ThreadID: "t_1", URL: "/t/t_1", Data: map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProviderID != "projects/proj-1/messages/0:123" || res.Gone {
		t.Fatalf("res = %+v", res)
	}
	m := got["message"].(map[string]any)
	if m["token"] != "reg-token-1" {
		t.Errorf("token = %v", m["token"])
	}
	n := m["notification"].(map[string]any)
	if n["title"] != "t" || n["body"] != "b" {
		t.Errorf("notification = %v", n)
	}
	d := m["data"].(map[string]any)
	if d["k"] != "v" || d["thread_id"] != "t_1" || d["url"] != "/t/t_1" {
		t.Errorf("data = %v", d)
	}
	if m["android"].(map[string]any)["priority"] != "high" {
		t.Errorf("android = %v", m["android"])
	}
	if s.Provider() != "fcm" {
		t.Errorf("provider = %s", s.Provider())
	}
}

func TestTokenCaching(t *testing.T) {
	f, s, clock := newFake(t, okSend)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := s.Send(ctx, json.RawMessage(`"x"`), notify.Message{}); err != nil {
			t.Fatal(err)
		}
	}
	if f.tokenCalls.Load() != 1 || f.sendCalls.Load() != 3 {
		t.Fatalf("token calls %d, sends %d", f.tokenCalls.Load(), f.sendCalls.Load())
	}
	clock.Advance(58 * time.Minute)
	s.Send(ctx, json.RawMessage(`"x"`), notify.Message{})
	if f.tokenCalls.Load() != 1 {
		t.Fatalf("refreshed too early")
	}
	clock.Advance(90 * time.Second) // within a minute of expiry
	s.Send(ctx, json.RawMessage(`"x"`), notify.Message{})
	if f.tokenCalls.Load() != 2 {
		t.Fatalf("token calls = %d, want refresh", f.tokenCalls.Load())
	}
}

func TestGone(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"404":          {404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Requested entity was not found."}}`},
		"unregistered": {400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`},
		"status":       {400, `{"error":{"code":400,"status":"UNREGISTERED"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			_, s, _ := newFake(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			res, err := s.Send(context.Background(), json.RawMessage(`"x"`), notify.Message{})
			if err != nil || !res.Gone {
				t.Fatalf("res=%+v err=%v", res, err)
			}
		})
	}
}

func TestSendErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"invalid": {400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"bad"}}`},
		"5xx":     {503, `oops`},
	} {
		t.Run(name, func(t *testing.T) {
			_, s, _ := newFake(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, err := s.Send(context.Background(), json.RawMessage(`"x"`), notify.Message{})
			if !kernel.Is(err, kernel.KindUnavailable) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestUnauthorizedDropsCachedToken(t *testing.T) {
	var n atomic.Int32
	f, s, _ := newFake(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"code":401,"status":"UNAUTHENTICATED"}}`)
			return
		}
		io.WriteString(w, `{"name":"n"}`)
	})
	if _, err := s.Send(context.Background(), json.RawMessage(`"x"`), notify.Message{}); err == nil {
		t.Fatal("want error")
	}
	if _, err := s.Send(context.Background(), json.RawMessage(`"x"`), notify.Message{}); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls.Load() != 2 {
		t.Fatalf("token calls = %d", f.tokenCalls.Load())
	}
}

func TestInvalidToken(t *testing.T) {
	_, s, _ := newFake(t, okSend)
	for _, tok := range []string{`""`, `123`, `nope`} {
		if _, err := s.Send(context.Background(), json.RawMessage(tok), notify.Message{}); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v", tok, err)
		}
	}
}

func TestFCMUnreachable(t *testing.T) {
	_, s, _ := newFake(t, okSend)
	s.endpoint = "http://127.0.0.1:1"
	if _, err := s.Send(context.Background(), json.RawMessage(`"x"`), notify.Message{}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestMessageDataEmpty(t *testing.T) {
	if messageData(notify.Message{}) != nil {
		t.Fatal("want nil")
	}
	if len(truncate(make([]byte, 300))) != 200 {
		t.Fatal("not truncated")
	}
}
