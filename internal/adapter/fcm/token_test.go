package fcm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

func TestNewDefaults(t *testing.T) {
	s, err := New(Config{ServiceAccountJSON: saJSON(t, "", pkcs8PEM(t, rsaKey(t)))})
	if err != nil {
		t.Fatal(err)
	}
	if s.endpoint != DefaultEndpoint || s.tokens.tokenURL != "https://oauth2.googleapis.com/token" || s.project != "proj-1" {
		t.Fatalf("defaults: %s %s %s", s.endpoint, s.tokens.tokenURL, s.project)
	}
}

func TestTokenURLOverride(t *testing.T) {
	s, err := New(Config{ServiceAccountJSON: saJSON(t, "https://oauth2.googleapis.com/token", pkcs8PEM(t, rsaKey(t))), TokenURL: "http://fake/token"})
	if err != nil {
		t.Fatal(err)
	}
	if s.tokens.tokenURL != "http://fake/token" || s.tokens.sa.TokenURI != "https://oauth2.googleapis.com/token" {
		t.Fatal("override not applied")
	}
}

func TestPKCS1Key(t *testing.T) {
	k := rsaKey(t)
	p := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	if _, err := New(Config{ServiceAccountJSON: saJSON(t, "", p)}); err != nil {
		t.Fatal(err)
	}
}

func TestBadServiceAccounts(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecDER, _ := x509.MarshalPKCS8PrivateKey(ec)
	ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}))
	garbage := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")}))
	for name, raw := range map[string][]byte{
		"not json":    []byte("nope"),
		"missing":     []byte(`{"project_id":"p"}`),
		"not pem":     saJSON(t, "", "not a key"),
		"not rsa":     saJSON(t, "", ecPEM),
		"garbage der": saJSON(t, "", garbage),
	} {
		if _, err := New(Config{ServiceAccountJSON: raw}); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func tokenSender(t *testing.T, h http.HandlerFunc) *Sender {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := New(Config{ServiceAccountJSON: saJSON(t, srv.URL, pkcs8PEM(t, rsaKey(t)))})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTokenEndpointErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"401": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":"invalid_grant"}`)
		},
		"no token": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) },
		"not json": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `<html>`) },
	} {
		s := tokenSender(t, h)
		if _, err := s.tokens.Token(context.Background()); !kernel.Is(err, kernel.KindUnavailable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestTokenDefaultExpiry(t *testing.T) {
	s := tokenSender(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"access_token":"a"}`)
	})
	tok, err := s.tokens.Token(context.Background())
	if err != nil || tok != "a" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if d := s.tokens.expires.Sub(s.tokens.clock.Now()); d < 3500e9 || d > 3700e9 {
		t.Fatalf("expiry %v", d)
	}
}

func TestTokenUnreachable(t *testing.T) {
	s, _ := New(Config{ServiceAccountJSON: saJSON(t, "http://127.0.0.1:1/token", pkcs8PEM(t, rsaKey(t)))})
	if _, err := s.tokens.Token(context.Background()); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
