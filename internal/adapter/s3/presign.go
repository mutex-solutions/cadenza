package s3

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// MaxTTL is the longest a SigV4 link can stay valid.
const MaxTTL = 7 * 24 * time.Hour

// PresignPut signs a PUT of ref's bytes to media.ObjectKey(ref.SHA256). The
// returned headers are exactly the ones the client must send; they are part
// of the signature. In ChecksumSHA256 mode they include
// x-amz-checksum-sha256, so the store refuses bytes with another hash.
func (s *Store) PresignPut(ctx context.Context, ref media.FileRef, ttl time.Duration) (spec.Upload, error) {
	if err := checkTTL(ttl); err != nil {
		return spec.Upload{}, err
	}
	if !media.ValidSHA256(ref.SHA256) {
		return spec.Upload{}, kernel.Invalid("s3: sha256 %q is not 64 lowercase hex characters", ref.SHA256)
	}
	in := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(media.ObjectKey(ref.SHA256)),
		ContentType: aws.String(contentType(ref.Type)),
	}
	if s.checksum == ChecksumSHA256 {
		in.ChecksumSHA256 = aws.String(checksumHeader(ref.SHA256))
	}
	req, err := s.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return spec.Upload{}, kernel.Internal(err, "s3: signing upload failed")
	}
	exp, err := expiry(req.URL)
	if err != nil {
		return spec.Upload{}, kernel.Internal(err, "s3: signed upload link is malformed")
	}
	return spec.Upload{
		SHA256:  ref.SHA256,
		URL:     req.URL,
		Method:  req.Method,
		Headers: clientHeaders(req.SignedHeader),
		Expires: exp,
	}, nil
}

// PresignGet signs a GET that the store answers with contentType and a
// Content-Disposition from disposition(contentType, name).
func (s *Store) PresignGet(ctx context.Context, sha256, ct, name string, ttl time.Duration) (spec.FileURL, error) {
	if err := checkTTL(ttl); err != nil {
		return spec.FileURL{}, err
	}
	if !media.ValidSHA256(sha256) {
		return spec.FileURL{}, kernel.Invalid("s3: sha256 %q is not 64 lowercase hex characters", sha256)
	}
	in := &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(media.ObjectKey(sha256)),
		ResponseContentType:        aws.String(contentType(ct)),
		ResponseContentDisposition: aws.String(disposition(ct, name)),
	}
	req, err := s.presign.PresignGetObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return spec.FileURL{}, kernel.Internal(err, "s3: signing download failed")
	}
	exp, err := expiry(req.URL)
	if err != nil {
		return spec.FileURL{}, kernel.Internal(err, "s3: signed download link is malformed")
	}
	return spec.FileURL{URL: req.URL, Expires: exp}, nil
}

func checkTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > MaxTTL {
		return kernel.Invalid("s3: link lifetime %s must be between 1s and %s", ttl, MaxTTL)
	}
	return nil
}

// checksumHeader converts a hex SHA-256 to the value S3 expects in
// x-amz-checksum-sha256: base64 of the raw 32 bytes.
func checksumHeader(hexSHA string) string {
	raw, _ := hex.DecodeString(hexSHA) // validated by the caller
	return base64.StdEncoding.EncodeToString(raw)
}

// clientHeaders is every signed header except Host, which the HTTP client
// sets from the URL.
func clientHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if http.CanonicalHeaderKey(k) == "Host" || len(v) == 0 {
			continue
		}
		out[http.CanonicalHeaderKey(k)] = v[0]
	}
	return out
}

// expiry reads the exact expiry, in unix ms, from a SigV4 link's
// X-Amz-Date and X-Amz-Expires parameters.
func expiry(link string) (int64, error) {
	u, err := url.Parse(link)
	if err != nil {
		return 0, err
	}
	q := u.Query()
	signed, err := time.Parse("20060102T150405Z", q.Get("X-Amz-Date"))
	if err != nil {
		return 0, err
	}
	secs, err := strconv.Atoi(q.Get("X-Amz-Expires"))
	if err != nil {
		return 0, err
	}
	return kernel.Millis(signed.Add(time.Duration(secs) * time.Second)), nil
}
