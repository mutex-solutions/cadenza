package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestServesAppWithFallback(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"assets/app-1.js":      {Data: []byte("js")},
		"manifest.webmanifest": {Data: []byte("{}")},
	})
	if r := get(h, "GET", "/"); r.Code != 200 || r.Body.String() != "<html>app</html>" || r.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("index: %d %q %q", r.Code, r.Body, r.Header().Get("Cache-Control"))
	}
	if r := get(h, "GET", "/t/t_abc"); r.Body.String() != "<html>app</html>" {
		t.Fatal("app routes must serve index.html")
	}
	if r := get(h, "GET", "/assets/app-1.js"); r.Body.String() != "js" || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("hashed assets must be cached")
	}
	if r := get(h, "GET", "/assets/missing.js"); r.Code != 404 {
		t.Fatal("missing assets must 404")
	}
	if r := get(h, "GET", "/favicon.ico"); r.Code != 404 {
		t.Fatal("missing files with extensions must 404")
	}
	if r := get(h, "POST", "/"); r.Code != 405 {
		t.Fatal("POST must be refused")
	}
	if r := get(h, "GET", "/manifest.webmanifest"); r.Code != 200 {
		t.Fatal("root files must be served")
	}
}

func TestNotBuiltPage(t *testing.T) {
	h := handler(fstest.MapFS{".gitkeep": {}})
	if r := get(h, "GET", "/anything"); !strings.Contains(r.Body.String(), "without the web app") {
		t.Fatal("missing build page")
	}
}

func TestEmbeddedHandlerWorks(t *testing.T) {
	if r := get(Handler(), "GET", "/"); r.Code != 200 {
		t.Fatalf("embedded handler: %d", r.Code)
	}
}
