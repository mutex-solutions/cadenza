// Package diskblob is the trial-mode files.BlobStore: files live in a local
// folder and the Cadenza server itself plays the object store. It hands out
// the same kind of signed links as adapter/s3, pointing at Handler, so the
// rest of the system cannot tell the two apart.
package diskblob

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// PathPrefix is where Handler expects requests: {BaseURL}/blob/{sha256}.
const PathPrefix = "/blob/"

// MinSecretBytes is the shortest accepted signing secret.
const MinSecretBytes = 32

// Config says where files live and how links are signed.
type Config struct {
	// Dir holds the files, as Dir/sha256/ab/cdef... (the rest of the hash).
	Dir string
	// BaseURL is the server's external URL, e.g. "https://cadenza.example.com".
	// Links are BaseURL + PathPrefix + sha256.
	BaseURL string
	// Secret signs links with HMAC-SHA256. At least MinSecretBytes long.
	Secret []byte
	// MaxBytes is the largest upload accepted. Zero means media.MaxVideoBytes.
	MaxBytes int64
	// Clock defaults to kernel.SystemClock.
	Clock kernel.Clock
}

// Store keeps files on disk and signs links to its own Handler.
type Store struct {
	dir      string
	baseURL  string
	secret   []byte
	maxBytes int64
	clock    kernel.Clock
}

var _ files.BlobStore = (*Store)(nil)

// New checks cfg and creates Dir if needed.
func New(cfg Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, kernel.Invalid("diskblob: folder is not set")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return nil, kernel.Invalid("diskblob: base URL %q must start with http:// or https://", cfg.BaseURL)
	}
	if len(cfg.Secret) < MinSecretBytes {
		return nil, kernel.Invalid("diskblob: signing secret is %d bytes; it must be at least %d", len(cfg.Secret), MinSecretBytes)
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = media.MaxVideoBytes
	}
	if cfg.Clock == nil {
		cfg.Clock = kernel.SystemClock{}
	}
	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, kernel.Invalid("diskblob: folder %q: %v", cfg.Dir, err)
	}
	for _, d := range []string{dir, filepath.Join(dir, "tmp")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			e := kernel.Unavailable("diskblob: cannot create folder %s", d)
			e.Err = err
			return nil, e
		}
	}
	return &Store{
		dir:      dir,
		baseURL:  base,
		secret:   append([]byte(nil), cfg.Secret...),
		maxBytes: cfg.MaxBytes,
		clock:    cfg.Clock,
	}, nil
}

// Stat reports whether the file is on disk and its size.
func (s *Store) Stat(_ context.Context, sha256 string) (int64, bool, error) {
	if !media.ValidSHA256(sha256) {
		return 0, false, kernel.Invalid("diskblob: sha256 %q is not 64 lowercase hex characters", sha256)
	}
	fi, err := os.Stat(s.path(sha256))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		e := kernel.Unavailable("diskblob: checking file failed")
		e.Err = err
		return 0, false, e
	}
	return fi.Size(), true, nil
}

// path is where a file with this hash lives. Callers must pass a valid
// SHA-256 (64 lowercase hex characters), which cannot contain separators.
func (s *Store) path(sha256 string) string {
	return filepath.Join(s.dir, "sha256", sha256[:2], sha256[2:])
}
