package prepare

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// writeFile writes b to name inside a fresh temp folder and returns the path.
func writeFile(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// wantInvalid fails unless err is a KindInvalid kernel error, and returns it.
func wantInvalid(t *testing.T, err error) *kernel.Error {
	t.Helper()
	var ke *kernel.Error
	if !errors.As(err, &ke) || ke.Kind != kernel.KindInvalid {
		t.Fatalf("want invalid error, got %v", err)
	}
	return ke
}

// readAll reads the upload bytes of p.
func readAll(t *testing.T, p *Prepared) []byte {
	t.Helper()
	rc, err := p.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// checkRef verifies that p.File matches the bytes p.Open returns.
func checkRef(t *testing.T, p *Prepared) []byte {
	t.Helper()
	b := readAll(t, p)
	if p.File.SHA256 != hashBytes(b) || p.File.Size != int64(len(b)) {
		t.Fatalf("ref %+v does not match %d bytes with sha %s", p.File, len(b), hashBytes(b))
	}
	if err := p.File.Validate(); err != nil {
		t.Fatalf("ref invalid: %v", err)
	}
	return b
}
