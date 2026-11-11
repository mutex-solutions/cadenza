package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func testConfig(endpoint string) Config {
	return Config{
		Endpoint:    endpoint,
		Region:      testRegion,
		Bucket:      testBucket,
		AccessKey:   testAccess,
		SecretKey:   testSecret,
		MaxAttempts: 1,
	}
}

func newStore(t *testing.T, cfg Config) *Store {
	t.Helper()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNewValidates(t *testing.T) {
	ok := testConfig("http://minio:9000")
	cases := map[string]func(*Config){
		"bucket":   func(c *Config) { c.Bucket = "" },
		"region":   func(c *Config) { c.Region = "" },
		"keys":     func(c *Config) { c.SecretKey = "" },
		"mode":     func(c *Config) { c.ChecksumMode = "md5" },
		"endpoint": func(c *Config) { c.Endpoint = "minio:9000" },
		"public":   func(c *Config) { c.PublicEndpoint = "ftp://x" },
	}
	for name, mutate := range cases {
		c := ok
		mutate(&c)
		if _, err := New(c); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	if _, err := New(ok); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestPresignPutSignsChecksumAndType(t *testing.T) {
	s := newStore(t, testConfig("http://minio.local:9000"))
	data := []byte("hello cadenza")
	sha := shaHex(data)
	up, err := s.PresignPut(context.Background(), media.FileRef{SHA256: sha, Size: int64(len(data)), Type: media.TypeWebP, Name: "a.webp"}, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if up.Method != http.MethodPut || up.SHA256 != sha {
		t.Fatalf("method/sha = %s/%s", up.Method, up.SHA256)
	}
	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "minio.local:9000" || u.Path != "/"+testBucket+"/sha256/"+sha {
		t.Errorf("url = %s, want path-style key", up.URL)
	}
	q := u.Query()
	signed := q.Get("X-Amz-SignedHeaders")
	for _, h := range []string{"content-type", "host", "x-amz-checksum-sha256"} {
		if !strings.Contains(signed, h) {
			t.Errorf("signed headers %q missing %s", signed, h)
		}
	}
	if strings.Contains(strings.ToLower(up.URL), "x-amz-sdk-checksum-algorithm") {
		t.Errorf("url has x-amz-sdk-checksum-algorithm: %s", up.URL)
	}
	if strings.Contains(strings.ToLower(u.RawQuery), "x-amz-checksum-sha256=") {
		t.Errorf("checksum was hoisted into the query: %s", up.URL)
	}
	sum := sha256.Sum256(data)
	want := map[string]string{
		"Content-Type":          media.TypeWebP,
		"X-Amz-Checksum-Sha256": base64.StdEncoding.EncodeToString(sum[:]),
	}
	if len(up.Headers) != len(want) {
		t.Errorf("headers = %v, want %v", up.Headers, want)
	}
	for k, v := range want {
		if up.Headers[k] != v {
			t.Errorf("header %s = %q, want %q", k, up.Headers[k], v)
		}
	}
	if q.Get("X-Amz-Expires") != "900" {
		t.Errorf("X-Amz-Expires = %s", q.Get("X-Amz-Expires"))
	}
	exp := kernel.FromMillis(up.Expires)
	if d := time.Until(exp); d < 14*time.Minute || d > 16*time.Minute {
		t.Errorf("expires in %s, want ~15m", d)
	}
}

func TestPresignPutChecksumNone(t *testing.T) {
	cfg := testConfig("https://s3.us-west-004.backblazeb2.com")
	cfg.ChecksumMode = ChecksumNone
	s := newStore(t, cfg)
	up, err := s.PresignPut(context.Background(), media.FileRef{SHA256: shaHex([]byte("x")), Size: 1, Type: "text/plain", Name: "x"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(up.URL), "checksum") {
		t.Errorf("url mentions checksum: %s", up.URL)
	}
	if len(up.Headers) != 1 || up.Headers["Content-Type"] != media.TypeOctetStream {
		t.Errorf("headers = %v, want only octet-stream content type", up.Headers)
	}
}

func TestPresignUsesPublicEndpointAndVirtualHost(t *testing.T) {
	cfg := testConfig("http://minio:9000")
	cfg.PublicEndpoint = "https://files.example.com"
	s := newStore(t, cfg)
	sha := shaHex([]byte("p"))
	up, err := s.PresignPut(context.Background(), media.FileRef{SHA256: sha, Size: 1, Type: media.TypePDF, Name: "p.pdf"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(up.URL, "https://files.example.com/"+testBucket+"/sha256/"+sha+"?") {
		t.Errorf("url = %s", up.URL)
	}

	vh := false
	cfg.UsePathStyle = &vh
	s = newStore(t, cfg)
	got, err := s.PresignGet(context.Background(), sha, media.TypePDF, "p.pdf", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.URL, "https://"+testBucket+".files.example.com/sha256/"+sha+"?") {
		t.Errorf("virtual-host url = %s", got.URL)
	}
}

func TestPresignRejectsBadInput(t *testing.T) {
	s := newStore(t, testConfig("http://minio:9000"))
	ctx := context.Background()
	good := shaHex([]byte("g"))
	if _, err := s.PresignPut(ctx, media.FileRef{SHA256: "ABC", Type: media.TypePDF}, time.Minute); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("bad sha put: %v", err)
	}
	if _, err := s.PresignPut(ctx, media.FileRef{SHA256: good, Type: media.TypePDF}, 0); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("zero ttl put: %v", err)
	}
	if _, err := s.PresignGet(ctx, "../etc", media.TypePDF, "x", time.Minute); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("bad sha get: %v", err)
	}
	if _, err := s.PresignGet(ctx, good, media.TypePDF, "x", 8*24*time.Hour); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("long ttl get: %v", err)
	}
	if _, _, err := s.Stat(ctx, "nope"); !kernel.Is(err, kernel.KindInvalid) {
		t.Errorf("bad sha stat: %v", err)
	}
}

func TestPresignGetDisposition(t *testing.T) {
	s := newStore(t, testConfig("http://minio:9000"))
	sha := shaHex([]byte("d"))
	cases := []struct{ ct, name, typ, disp string }{
		{media.TypeWebP, "shot.webp", media.TypeWebP, `inline; filename=shot.webp`},
		{media.TypeMP4, "", media.TypeMP4, `inline`},
		{media.TypeOctetStream, "report final.zip", media.TypeOctetStream, `attachment; filename="report final.zip"`},
		{"text/x-evil", "a.sh", media.TypeOctetStream, `attachment; filename=a.sh`},
		{media.TypeOctetStream, "../x", media.TypeOctetStream, `attachment`},
	}
	for _, c := range cases {
		got, err := s.PresignGet(context.Background(), sha, c.ct, c.name, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(got.URL)
		q := u.Query()
		if q.Get("response-content-type") != c.typ || q.Get("response-content-disposition") != c.disp {
			t.Errorf("%s %q: type=%q disp=%q, want %q %q", c.ct, c.name,
				q.Get("response-content-type"), q.Get("response-content-disposition"), c.typ, c.disp)
		}
		if d := time.Until(kernel.FromMillis(got.Expires)); d < 59*time.Minute || d > 61*time.Minute {
			t.Errorf("expires in %s", d)
		}
	}
}

func TestStat(t *testing.T) {
	fake := newFakeS3(t)
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s := newStore(t, testConfig(srv.URL))
	ctx := context.Background()

	data := []byte("stat me")
	sha := shaHex(data)
	fake.objects["/"+testBucket+"/sha256/"+sha] = data
	size, ok, err := s.Stat(ctx, sha)
	if err != nil || !ok || size != int64(len(data)) {
		t.Fatalf("Stat existing = %d %v %v", size, ok, err)
	}

	size, ok, err = s.Stat(ctx, shaHex([]byte("missing")))
	if err != nil || ok || size != 0 {
		t.Fatalf("Stat missing = %d %v %v", size, ok, err)
	}

	broken := shaHex([]byte("broken"))
	fake.heads["/"+testBucket+"/sha256/"+broken] = http.StatusInternalServerError
	if _, _, err := s.Stat(ctx, broken); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("Stat 500 err = %v, want unavailable", err)
	}
}

// TestEndToEnd uploads through a presigned PUT to a fake store that checks
// the signature and checksum, then downloads through a presigned GET.
func TestEndToEnd(t *testing.T) {
	fake := newFakeS3(t)
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s := newStore(t, testConfig(srv.URL))
	ctx := context.Background()

	data := []byte("the real bytes")
	ref := media.FileRef{SHA256: shaHex(data), Size: int64(len(data)), Type: media.TypePDF, Name: "doc.pdf"}
	up, err := s.PresignPut(ctx, ref, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	put := func(body []byte, headers map[string]string) (int, string) {
		req, _ := http.NewRequest(up.Method, up.URL, bytes.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, msg := put(data, nil); code != http.StatusForbidden {
		t.Errorf("PUT without headers = %d %s, want 403", code, msg)
	}
	wrongType := map[string]string{}
	for k, v := range up.Headers {
		wrongType[k] = v
	}
	wrongType["Content-Type"] = "text/html"
	if code, msg := put(data, wrongType); code != http.StatusForbidden || !strings.Contains(msg, "SignatureDoesNotMatch") {
		t.Errorf("PUT with changed type = %d %s, want signature mismatch", code, msg)
	}
	if code, msg := put([]byte("other bytes"), up.Headers); code != http.StatusBadRequest {
		t.Errorf("PUT wrong bytes = %d %s, want 400", code, msg)
	}
	if code, msg := put(data, up.Headers); code != http.StatusOK {
		t.Fatalf("PUT = %d %s", code, msg)
	}

	size, ok, err := s.Stat(ctx, ref.SHA256)
	if err != nil || !ok || size != ref.Size {
		t.Fatalf("Stat after PUT = %d %v %v", size, ok, err)
	}

	link, err := s.PresignGet(ctx, ref.SHA256, ref.Type, ref.Name, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, data) {
		t.Fatalf("GET = %d %q", resp.StatusCode, got)
	}
	if resp.Header.Get("Content-Type") != media.TypePDF || resp.Header.Get("Content-Disposition") != "inline; filename=doc.pdf" {
		t.Errorf("GET headers = %v", resp.Header)
	}
}
