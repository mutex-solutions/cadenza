package diskblob

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// link is what a signed URL grants: one method on one file until exp
// (unix seconds), served or accepted as typ. Size is set only for PUT.
type link struct {
	method string
	sha    string
	exp    int64
	typ    string
	name   string
	size   int64
}

// PresignPut signs a PUT of ref's bytes to Handler. The client must send the
// returned Content-Type; Handler rejects bytes whose size or SHA-256 differ
// from ref.
func (s *Store) PresignPut(_ context.Context, ref media.FileRef, ttl time.Duration) (spec.Upload, error) {
	if !media.ValidSHA256(ref.SHA256) {
		return spec.Upload{}, kernel.Invalid("diskblob: sha256 %q is not 64 lowercase hex characters", ref.SHA256)
	}
	if ref.Size <= 0 || ref.Size > s.maxBytes {
		return spec.Upload{}, kernel.Invalid("diskblob: size %d must be between 1 and %d bytes", ref.Size, s.maxBytes)
	}
	if ttl <= 0 {
		return spec.Upload{}, kernel.Invalid("diskblob: link lifetime must be positive")
	}
	l := link{method: http.MethodPut, sha: ref.SHA256, exp: s.expiry(ttl), typ: contentType(ref.Type), size: ref.Size}
	return spec.Upload{
		SHA256:  ref.SHA256,
		URL:     s.url(l),
		Method:  http.MethodPut,
		Headers: map[string]string{"Content-Type": l.typ},
		Expires: l.exp * 1000,
	}, nil
}

// PresignGet signs a GET that Handler serves with contentType and a
// Content-Disposition made from name.
func (s *Store) PresignGet(_ context.Context, sha256, ct, name string, ttl time.Duration) (spec.FileURL, error) {
	if !media.ValidSHA256(sha256) {
		return spec.FileURL{}, kernel.Invalid("diskblob: sha256 %q is not 64 lowercase hex characters", sha256)
	}
	if ttl <= 0 {
		return spec.FileURL{}, kernel.Invalid("diskblob: link lifetime must be positive")
	}
	if media.ValidName(name) != nil {
		name = ""
	}
	l := link{method: http.MethodGet, sha: sha256, exp: s.expiry(ttl), typ: contentType(ct), name: name}
	return spec.FileURL{URL: s.url(l), Expires: l.exp * 1000}, nil
}

// expiry rounds up to whole seconds so a link never expires early.
func (s *Store) expiry(ttl time.Duration) int64 {
	t := s.clock.Now().Add(ttl)
	exp := t.Unix()
	if t.Nanosecond() > 0 {
		exp++
	}
	return exp
}

func (s *Store) url(l link) string {
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(l.exp, 10))
	q.Set("type", l.typ)
	if l.name != "" {
		q.Set("name", l.name)
	}
	if l.method == http.MethodPut {
		q.Set("size", strconv.FormatInt(l.size, 10))
	}
	q.Set("sig", s.sign(l))
	return s.baseURL + PathPrefix + l.sha + "?" + q.Encode()
}

// sign is hex HMAC-SHA256 over every field of l. Each field is length
// prefixed, so no value can spill into the next.
func (s *Store) sign(l link) string {
	m := hmac.New(sha256.New, s.secret)
	for _, f := range []string{l.method, l.sha, strconv.FormatInt(l.exp, 10), l.typ, l.name, strconv.FormatInt(l.size, 10)} {
		fmt.Fprintf(m, "%d:%s;", len(f), f)
	}
	return hex.EncodeToString(m.Sum(nil))
}

// parse reads and checks the link for request r on file sha. It fails when
// the signature does not match or the link has expired.
func (s *Store) parse(r *http.Request, sha string) (link, error) {
	q := r.URL.Query()
	l := link{method: r.Method, sha: sha, typ: q.Get("type"), name: q.Get("name")}
	if r.Method == http.MethodHead {
		l.method = http.MethodGet // a GET link also allows HEAD
	}
	var err error
	if l.exp, err = strconv.ParseInt(q.Get("exp"), 10, 64); err != nil {
		return link{}, kernel.Forbidden("link has no valid expiry")
	}
	if l.method == http.MethodPut {
		if l.size, err = strconv.ParseInt(q.Get("size"), 10, 64); err != nil || l.size <= 0 {
			return link{}, kernel.Forbidden("link has no valid size")
		}
	}
	got, err := hex.DecodeString(q.Get("sig"))
	if err != nil {
		return link{}, kernel.Forbidden("link signature is malformed")
	}
	want, _ := hex.DecodeString(s.sign(l))
	if !hmac.Equal(got, want) {
		return link{}, kernel.Forbidden("link signature does not match")
	}
	if s.clock.Now().Unix() > l.exp {
		return link{}, kernel.Forbidden("link has expired")
	}
	return l, nil
}
