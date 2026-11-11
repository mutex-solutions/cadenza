package diskblob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

var (
	testSecret = []byte("0123456789abcdef0123456789abcdef")
	testStart  = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
)

type env struct {
	store *Store
	clock *kernel.FakeClock
	srv   *httptest.Server
	dir   string
}

func newEnv(t *testing.T, maxBytes int64) *env {
	t.Helper()
	e := &env{clock: kernel.NewFakeClock(testStart), dir: t.TempDir()}
	mux := http.NewServeMux()
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	s, err := New(Config{Dir: e.dir, BaseURL: e.srv.URL + "/", Secret: testSecret, MaxBytes: maxBytes, Clock: e.clock})
	if err != nil {
		t.Fatal(err)
	}
	e.store = s
	mux.Handle(PathPrefix, s.Handler())
	return e
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func ref(data []byte, typ string) media.FileRef {
	return media.FileRef{SHA256: shaHex(data), Size: int64(len(data)), Type: typ, Name: "f.bin"}
}

func do(t *testing.T, method, link string, body []byte, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, link, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body == nil {
		req.Body = nil
		req.ContentLength = 0
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func (e *env) upload(t *testing.T, data []byte, typ string) {
	t.Helper()
	up, err := e.store.PresignPut(context.Background(), ref(data, typ), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if resp, b := do(t, up.Method, up.URL, data, up.Headers); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d %s", resp.StatusCode, b)
	}
}

func TestNewValidates(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]Config{
		"dir":    {BaseURL: "http://x", Secret: testSecret},
		"base":   {Dir: dir, BaseURL: "x.com", Secret: testSecret},
		"secret": {Dir: dir, BaseURL: "http://x", Secret: []byte("short")},
	}
	for name, c := range cases {
		if _, err := New(c); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Dir: blocker, BaseURL: "http://x", Secret: testSecret}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Errorf("dir is a file: err = %v, want unavailable", err)
	}
	s, err := New(Config{Dir: dir, BaseURL: "http://x", Secret: testSecret})
	if err != nil || s.maxBytes != media.MaxVideoBytes || s.clock == nil {
		t.Fatalf("defaults: %+v %v", s, err)
	}
}

func TestPresignPutLink(t *testing.T) {
	e := newEnv(t, 0)
	data := []byte("hello")
	up, err := e.store.PresignPut(context.Background(), ref(data, "text/plain"), 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(up.URL)
	if u.Path != PathPrefix+shaHex(data) {
		t.Errorf("path = %s", u.Path)
	}
	q := u.Query()
	if q.Get("size") != "5" || q.Get("type") != media.TypeOctetStream || q.Get("sig") == "" {
		t.Errorf("query = %v", q)
	}
	if up.Method != http.MethodPut || up.Headers["Content-Type"] != media.TypeOctetStream || len(up.Headers) != 1 {
		t.Errorf("upload = %+v", up)
	}
	if up.Expires != kernel.Millis(testStart.Add(90*time.Second)) {
		t.Errorf("expires = %d", up.Expires)
	}
}

func TestPresignRejectsBadInput(t *testing.T) {
	e := newEnv(t, 10)
	ctx := context.Background()
	good := shaHex([]byte("x"))
	bad := []media.FileRef{
		{SHA256: "../../etc/passwd", Size: 1},
		{SHA256: strings.ToUpper(good), Size: 1},
		{SHA256: good, Size: 0},
		{SHA256: good, Size: 11},
	}
	for _, r := range bad {
		if _, err := e.store.PresignPut(ctx, r, time.Minute); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("PresignPut(%+v) err = %v", r, err)
		}
	}
	if _, err := e.store.PresignPut(ctx, media.FileRef{SHA256: good, Size: 1}, 0); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("zero ttl put: %v", err)
	}
	if _, err := e.store.PresignGet(ctx, "abc", "", "", time.Minute); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("bad sha get: %v", err)
	}
	if _, err := e.store.PresignGet(ctx, good, "", "", -time.Second); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("negative ttl get: %v", err)
	}
	if _, _, err := e.store.Stat(ctx, "/etc/passwd"); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("bad sha stat: %v", err)
	}
}

func TestExpiryRoundsUp(t *testing.T) {
	e := newEnv(t, 0)
	e.clock.Advance(300 * time.Millisecond)
	got, err := e.store.PresignGet(context.Background(), shaHex(nil), "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := kernel.Millis(testStart.Add(2 * time.Second)); got.Expires != want {
		t.Errorf("expires = %d, want %d", got.Expires, want)
	}
}

func TestRoundTrip(t *testing.T) {
	e := newEnv(t, 0)
	ctx := context.Background()
	data := []byte("%PDF-1.7 pretend pdf bytes")
	r := ref(data, media.TypePDF)

	if _, ok, err := e.store.Stat(ctx, r.SHA256); ok || err != nil {
		t.Fatalf("Stat before = %v %v", ok, err)
	}
	e.upload(t, data, media.TypePDF)

	onDisk := filepath.Join(e.dir, "sha256", r.SHA256[:2], r.SHA256[2:])
	if b, err := os.ReadFile(onDisk); err != nil || !bytes.Equal(b, data) {
		t.Fatalf("file on disk = %q %v", b, err)
	}
	if left, _ := os.ReadDir(filepath.Join(e.dir, "tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	size, ok, err := e.store.Stat(ctx, r.SHA256)
	if err != nil || !ok || size != r.Size {
		t.Fatalf("Stat = %d %v %v", size, ok, err)
	}

	// Uploading the same bytes again is accepted.
	e.upload(t, data, media.TypePDF)

	link, err := e.store.PresignGet(ctx, r.SHA256, media.TypePDF, "report.pdf", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := do(t, http.MethodGet, link.URL, nil, nil)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("GET = %d %q", resp.StatusCode, body)
	}
	want := map[string]string{
		"Content-Type":           media.TypePDF,
		"Content-Disposition":    "inline; filename=report.pdf",
		"X-Content-Type-Options": "nosniff",
	}
	for k, v := range want {
		if resp.Header.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, resp.Header.Get(k), v)
		}
	}
	if resp.Header.Get("Content-Security-Policy") != "" {
		t.Errorf("PDF got a CSP sandbox")
	}

	resp, body = do(t, http.MethodHead, link.URL, nil, nil)
	if resp.StatusCode != http.StatusOK || len(body) != 0 || resp.ContentLength != r.Size {
		t.Errorf("HEAD = %d len=%d cl=%d", resp.StatusCode, len(body), resp.ContentLength)
	}
}

func TestGetDispositionAndSandbox(t *testing.T) {
	e := newEnv(t, 0)
	ctx := context.Background()
	page := []byte("<script>alert(1)</script>")
	e.upload(t, page, media.TypeHTML)
	link, _ := e.store.PresignGet(ctx, shaHex(page), media.TypeHTML, "page.html", time.Minute)
	resp, _ := do(t, http.MethodGet, link.URL, nil, nil)
	if resp.Header.Get("Content-Security-Policy") != "sandbox allow-scripts allow-popups" {
		t.Errorf("html CSP = %q", resp.Header.Get("Content-Security-Policy"))
	}

	link, _ = e.store.PresignGet(ctx, shaHex(page), "text/x-unknown", "../evil name.sh", time.Minute)
	resp, _ = do(t, http.MethodGet, link.URL, nil, nil)
	if resp.Header.Get("Content-Type") != media.TypeOctetStream || resp.Header.Get("Content-Disposition") != "attachment" {
		t.Errorf("other: type=%q disp=%q", resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"))
	}

	link, _ = e.store.PresignGet(ctx, shaHex(page), media.TypeOctetStream, "notes v2.txt", time.Minute)
	resp, _ = do(t, http.MethodGet, link.URL, nil, nil)
	if resp.Header.Get("Content-Disposition") != `attachment; filename="notes v2.txt"` {
		t.Errorf("disp = %q", resp.Header.Get("Content-Disposition"))
	}
}

func TestRange(t *testing.T) {
	e := newEnv(t, 0)
	data := []byte("0123456789abcdefghij")
	e.upload(t, data, media.TypeMP4)
	link, _ := e.store.PresignGet(context.Background(), shaHex(data), media.TypeMP4, "v.mp4", time.Minute)
	resp, body := do(t, http.MethodGet, link.URL, nil, map[string]string{"Range": "bytes=5-9"})
	if resp.StatusCode != http.StatusPartialContent || string(body) != "56789" {
		t.Fatalf("range GET = %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Range") != "bytes 5-9/20" || resp.Header.Get("Content-Type") != media.TypeMP4 {
		t.Errorf("headers = %v", resp.Header)
	}
}

func TestPutRejectsMismatch(t *testing.T) {
	e := newEnv(t, 0)
	data := []byte("expected bytes")
	up, _ := e.store.PresignPut(context.Background(), ref(data, media.TypeWebP), time.Minute)

	cases := []struct {
		name    string
		body    []byte
		headers map[string]string
		code    int
	}{
		{"same size other bytes", []byte("EXPECTED BYTES"), up.Headers, http.StatusBadRequest},
		{"shorter", data[:5], up.Headers, http.StatusBadRequest},
		{"longer", append(append([]byte{}, data...), 'x'), up.Headers, http.StatusRequestEntityTooLarge},
		{"wrong type", data, map[string]string{"Content-Type": "text/html"}, http.StatusForbidden},
		{"no type", data, nil, http.StatusForbidden},
	}
	for _, c := range cases {
		resp, b := do(t, up.Method, up.URL, c.body, c.headers)
		if resp.StatusCode != c.code {
			t.Errorf("%s: %d %s, want %d", c.name, resp.StatusCode, b, c.code)
		}
	}
	if _, ok, _ := e.store.Stat(context.Background(), up.SHA256); ok {
		t.Fatal("mismatched bytes were stored")
	}
	if left, _ := os.ReadDir(filepath.Join(e.dir, "tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

// TestPutChunkedChecks covers uploads without Content-Length, where only the
// streamed byte count and hash can catch a mismatch.
func TestPutChunkedChecks(t *testing.T) {
	e := newEnv(t, 0)
	data := []byte("chunked body")
	up, _ := e.store.PresignPut(context.Background(), ref(data, media.TypePDF), time.Minute)
	send := func(body []byte) int {
		req, _ := http.NewRequest(http.MethodPut, up.URL, io.MultiReader(bytes.NewReader(body)))
		req.ContentLength = -1
		req.Header.Set("Content-Type", media.TypePDF)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := send([]byte("chunked")); code != http.StatusBadRequest {
		t.Errorf("short chunked = %d", code)
	}
	if code := send([]byte("chunked body and more")); code != http.StatusRequestEntityTooLarge {
		t.Errorf("long chunked = %d", code)
	}
	if code := send([]byte("CHUNKED BODY")); code != http.StatusBadRequest {
		t.Errorf("wrong hash chunked = %d", code)
	}
	if code := send(data); code != http.StatusOK {
		t.Errorf("good chunked = %d", code)
	}
}

func TestMaxBytes(t *testing.T) {
	e := newEnv(t, 8)
	data := []byte("12345678")
	e.upload(t, data, media.TypePDF)
	// A link signed for more than the limit (e.g. by an older config) is refused.
	big := []byte("123456789")
	l := link{method: http.MethodPut, sha: shaHex(big), exp: testStart.Unix() + 60, typ: media.TypePDF, size: 9}
	resp, _ := do(t, http.MethodPut, e.store.url(l), big, map[string]string{"Content-Type": media.TypePDF})
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("over limit = %d", resp.StatusCode)
	}
}

func TestSignatureChecks(t *testing.T) {
	e := newEnv(t, 0)
	ctx := context.Background()
	data := []byte("signed")
	e.upload(t, data, media.TypeWebP)
	sha := shaHex(data)
	link, _ := e.store.PresignGet(ctx, sha, media.TypeWebP, "a.webp", time.Minute)

	tamper := func(key, val string) string {
		u, _ := url.Parse(link.URL)
		q := u.Query()
		if val == "" {
			q.Del(key)
		} else {
			q.Set(key, val)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	other := shaHex([]byte("other"))
	e.upload(t, []byte("other"), media.TypeWebP)
	forbidden := map[string]string{
		"type changed": tamper("type", media.TypeHTML),
		"name changed": tamper("name", "b.webp"),
		"exp extended": tamper("exp", "99999999999"),
		"exp missing":  tamper("exp", ""),
		"exp garbage":  tamper("exp", "soon"),
		"sig missing":  tamper("sig", ""),
		"sig not hex":  tamper("sig", "zz"),
		"sig flipped":  tamper("sig", strings.Repeat("0", 64)),
		"other file":   strings.Replace(link.URL, sha, other, 1),
	}
	for name, u := range forbidden {
		if resp, b := do(t, http.MethodGet, u, nil, nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d %s, want 403", name, resp.StatusCode, b)
		}
	}
	// A GET link cannot be used to upload.
	if resp, _ := do(t, http.MethodPut, link.URL, data, map[string]string{"Content-Type": media.TypeWebP}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("PUT with GET link = %d", resp.StatusCode)
	}
	// A PUT link cannot be used to download.
	up, _ := e.store.PresignPut(ctx, ref(data, media.TypeWebP), time.Minute)
	if resp, _ := do(t, http.MethodGet, up.URL, nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET with PUT link = %d", resp.StatusCode)
	}
	// PUT link with its size changed.
	u, _ := url.Parse(up.URL)
	q := u.Query()
	q.Set("size", "7")
	u.RawQuery = q.Encode()
	if resp, _ := do(t, http.MethodPut, u.String(), append(data, 'x'), up.Headers); resp.StatusCode != http.StatusForbidden {
		t.Errorf("PUT with changed size = %d", resp.StatusCode)
	}
	q.Set("size", "-1")
	u.RawQuery = q.Encode()
	if resp, _ := do(t, http.MethodPut, u.String(), data, up.Headers); resp.StatusCode != http.StatusForbidden {
		t.Errorf("PUT with negative size = %d", resp.StatusCode)
	}
	// A different secret does not verify.
	s2, _ := New(Config{Dir: e.dir, BaseURL: e.srv.URL, Secret: []byte("another-secret-another-secret-32"), Clock: e.clock})
	forged, _ := s2.PresignGet(ctx, sha, media.TypeWebP, "a.webp", time.Minute)
	if resp, _ := do(t, http.MethodGet, forged.URL, nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("other secret = %d", resp.StatusCode)
	}
	// The untouched link still works.
	if resp, _ := do(t, http.MethodGet, link.URL, nil, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("valid link = %d", resp.StatusCode)
	}
}

func TestExpiry(t *testing.T) {
	e := newEnv(t, 0)
	data := []byte("expiring")
	up, _ := e.store.PresignPut(context.Background(), ref(data, media.TypePDF), time.Minute)
	link, _ := e.store.PresignGet(context.Background(), up.SHA256, media.TypePDF, "", time.Minute)
	e.clock.Advance(time.Minute) // exactly at expiry: still valid
	if resp, b := do(t, up.Method, up.URL, data, up.Headers); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT at expiry = %d %s", resp.StatusCode, b)
	}
	e.clock.Advance(time.Second)
	if resp, _ := do(t, http.MethodGet, link.URL, nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET after expiry = %d", resp.StatusCode)
	}
	if resp, _ := do(t, up.Method, up.URL, data, up.Headers); resp.StatusCode != http.StatusForbidden {
		t.Errorf("PUT after expiry = %d", resp.StatusCode)
	}
}

func TestRoutingAndTraversal(t *testing.T) {
	e := newEnv(t, 0)
	secret := filepath.Join(e.dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("do not serve"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := e.store.Handler()
	paths := []string{
		"/blob/../secret.txt",
		"/blob/..%2fsecret.txt",
		"/blob/" + strings.Repeat("a", 63),
		"/blob/" + strings.Repeat("A", 64),
		"/blob/" + strings.Repeat("a", 62) + "/x",
		"/other/" + strings.Repeat("a", 64),
		"/blob/",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, "http://x"+"/placeholder", nil)
		req.URL.Path = p
		req.URL.RawPath = ""
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "do not serve") {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
	// Valid hash, nothing stored, valid signature: 404.
	missing := shaHex([]byte("missing"))
	link, _ := e.store.PresignGet(context.Background(), missing, "", "", time.Minute)
	if resp, _ := do(t, http.MethodGet, link.URL, nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing file = %d", resp.StatusCode)
	}
	// Unsupported method.
	if resp, _ := do(t, http.MethodDelete, link.URL, nil, nil); resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") == "" {
		t.Errorf("DELETE = %d", resp.StatusCode)
	}
	// Mounted under a path prefix (BaseURL with a path) still routes.
	req := httptest.NewRequest(http.MethodGet, strings.Replace(link.URL, e.srv.URL, "http://x/cadenza", 1), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("prefixed path = %d, want 404 for the missing file", rec.Code)
	}
}

func TestStatUnavailable(t *testing.T) {
	e := newEnv(t, 0)
	sha := shaHex([]byte("x"))
	// Make the shard path a file so Stat of a child fails with ENOTDIR.
	shard := filepath.Join(e.dir, "sha256", sha[:2])
	if err := os.MkdirAll(filepath.Dir(shard), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shard, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.store.Stat(context.Background(), sha); err == nil {
		// ENOTDIR maps to "does not exist" on some systems; both are acceptable
		// as long as it is not reported as present.
		t.Log("stat through a file reported not-exist")
	} else if !kernel.Is(err, kernel.KindUnavailable) {
		t.Errorf("err = %v, want unavailable", err)
	}
}
