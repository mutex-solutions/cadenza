// Package files manages content-addressed files: which uploads are needed,
// signed links, and confirming bytes exist before a post may use them. File
// bytes never pass through the server (plan D9); a BlobStore only signs.
package files

import (
	"context"
	"time"

	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// BlobStore is object storage addressed by media.ObjectKey(sha256).
// Implementations: adapter/s3 (real use) and adapter/diskblob (trial mode).
type BlobStore interface {
	// PresignPut returns a signed upload for ref. The store must reject bytes
	// whose SHA-256 does not match ref.SHA256 where it can (S3 checksum header).
	PresignPut(ctx context.Context, ref media.FileRef, ttl time.Duration) (spec.Upload, error)
	// PresignGet returns a signed download link that serves the file with this
	// content type and, for downloads, this file name.
	PresignGet(ctx context.Context, sha256, contentType, name string, ttl time.Duration) (spec.FileURL, error)
	// Stat reports whether the object exists and its size.
	Stat(ctx context.Context, sha256 string) (size int64, exists bool, err error)
}
