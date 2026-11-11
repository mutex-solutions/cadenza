package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// TestRealStore runs against a real S3-compatible store when
// CADENZA_TEST_S3_ENDPOINT is set, e.g. a local MinIO:
//
//	CADENZA_TEST_S3_ENDPOINT=http://localhost:9000 CADENZA_TEST_S3_REGION=us-east-1
//	CADENZA_TEST_S3_BUCKET=cadenza CADENZA_TEST_S3_ACCESS_KEY=... CADENZA_TEST_S3_SECRET_KEY=...
//	CADENZA_TEST_S3_CHECKSUM=sha256 (or none)
func TestRealStore(t *testing.T) {
	endpoint := os.Getenv("CADENZA_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("CADENZA_TEST_S3_ENDPOINT not set")
	}
	cfg := Config{
		Endpoint:     endpoint,
		Region:       os.Getenv("CADENZA_TEST_S3_REGION"),
		Bucket:       os.Getenv("CADENZA_TEST_S3_BUCKET"),
		AccessKey:    os.Getenv("CADENZA_TEST_S3_ACCESS_KEY"),
		SecretKey:    os.Getenv("CADENZA_TEST_S3_SECRET_KEY"),
		ChecksumMode: os.Getenv("CADENZA_TEST_S3_CHECKSUM"),
	}
	s := newStore(t, cfg)
	ctx := context.Background()

	data := make([]byte, 4096)
	_, _ = rand.Read(data)
	ref := media.FileRef{SHA256: shaHex(data), Size: int64(len(data)), Type: media.TypeOctetStream, Name: "blob.bin"}

	if _, ok, err := s.Stat(ctx, ref.SHA256); err != nil || ok {
		t.Fatalf("Stat before upload = %v %v", ok, err)
	}
	up, err := s.PresignPut(ctx, ref, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	do := func(body []byte) int {
		req, _ := http.NewRequest(up.Method, up.URL, bytes.NewReader(body))
		for k, v := range up.Headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if s.checksum == ChecksumSHA256 {
		bad := append([]byte{}, data...)
		bad[0] ^= 0xff
		if code := do(bad); code < 400 {
			t.Fatalf("store accepted bytes with the wrong hash (status %d)", code)
		}
	}
	if code := do(data); code != http.StatusOK {
		t.Fatalf("PUT = %d", code)
	}
	if size, ok, err := s.Stat(ctx, ref.SHA256); err != nil || !ok || size != ref.Size {
		t.Fatalf("Stat after upload = %d %v %v", size, ok, err)
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
	if !bytes.Equal(got, data) {
		t.Fatalf("GET returned %d bytes, status %d", len(got), resp.StatusCode)
	}
}
