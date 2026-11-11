package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	testAccess = "AKIDTEST"
	testSecret = "secret/test+key"
	testRegion = "us-east-1"
	testBucket = "cadenza"
)

// fakeS3 is a tiny S3: it checks SigV4 presigned PUTs and GETs like a real
// store would (signature, signed headers, x-amz-checksum-sha256) and answers
// HEAD from what was stored.
type fakeS3 struct {
	t       *testing.T
	mu      sync.Mutex
	objects map[string][]byte // path -> bytes
	heads   map[string]int    // path -> forced HEAD status
}

func newFakeS3(t *testing.T) *fakeS3 {
	return &fakeS3{t: t, objects: map[string][]byte{}, heads: map[string]int{}}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodHead:
		if code, ok := f.heads[r.URL.Path]; ok {
			w.WriteHeader(code)
			return
		}
		b, ok := f.objects[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		if msg := verifyPresigned(r); msg != "" {
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if want := r.Header.Get("X-Amz-Checksum-Sha256"); want != "" {
			sum := sha256.Sum256(body)
			if base64.StdEncoding.EncodeToString(sum[:]) != want {
				http.Error(w, "BadDigest", http.StatusBadRequest)
				return
			}
		}
		f.objects[r.URL.Path] = body
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if msg := verifyPresigned(r); msg != "" {
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		b, ok := f.objects[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		w.Header().Set("Content-Type", q.Get("response-content-type"))
		w.Header().Set("Content-Disposition", q.Get("response-content-disposition"))
		_, _ = w.Write(b)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// verifyPresigned recomputes the SigV4 query signature from the request as
// received. It returns "" when the signature matches.
func verifyPresigned(r *http.Request) string {
	q := r.URL.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		return "missing algorithm"
	}
	if q.Has("x-amz-sdk-checksum-algorithm") || q.Has("X-Amz-Sdk-Checksum-Algorithm") {
		return "unexpected x-amz-sdk-checksum-algorithm"
	}
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	var hdrs strings.Builder
	for _, h := range signed {
		v := r.Header.Get(h)
		if h == "host" {
			v = r.Host
		}
		if v == "" {
			return "signed header " + h + " not sent"
		}
		hdrs.WriteString(h + ":" + strings.TrimSpace(v) + "\n")
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		if k != "X-Amz-Signature" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var qs []string
	for _, k := range keys {
		qs = append(qs, awsEscape(k)+"="+awsEscape(q.Get(k)))
	}
	canonical := strings.Join([]string{
		r.Method, r.URL.EscapedPath(), strings.Join(qs, "&"),
		hdrs.String(), q.Get("X-Amz-SignedHeaders"), "UNSIGNED-PAYLOAD",
	}, "\n")
	date := q.Get("X-Amz-Date")
	scope := date[:8] + "/" + testRegion + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + date + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := []byte("AWS4" + testSecret)
	for _, part := range []string{date[:8], testRegion, "s3", "aws4_request"} {
		key = hmacSHA(key, part)
	}
	if hex.EncodeToString(hmacSHA(key, toSign)) != q.Get("X-Amz-Signature") {
		return "SignatureDoesNotMatch"
	}
	return ""
}

func hmacSHA(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
