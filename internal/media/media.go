// Package media holds the allowed file formats and limits (plan D10) and the
// content-addressed file reference. It is pure: the CLI uses it to check
// files before uploading and the server uses it again at the trust boundary.
package media

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Kind is what a file is shown as.
type Kind string

const (
	KindImage    Kind = "image"    // WebP, viewable
	KindVideo    Kind = "video"    // MP4 H.264, playable
	KindWebpage  Kind = "webpage"  // one self-contained HTML file, sandboxed
	KindDocument Kind = "document" // PDF
	KindOther    Kind = "other"    // download only
)

// Content types Cadenza stores. Anything else is stored as OctetStream.
const (
	TypeWebP        = "image/webp"
	TypeMP4         = "video/mp4"
	TypeHTML        = "text/html"
	TypePDF         = "application/pdf"
	TypeOctetStream = "application/octet-stream"
)

const (
	MB = 1 << 20

	MaxImageSide   = 2560 // longest side of a stored image, in pixels
	PreviewSide    = 640  // longest side of the preview image
	MaxVideoHeight = 1080
	MaxVideoBytes  = 500 * MB
	MaxPageBytes   = 10 * MB
	MaxPDFBytes    = 25 * MB
	MaxOtherBytes  = 25 * MB
)

// KindOf maps a stored content type to its kind.
func KindOf(contentType string) Kind {
	switch contentType {
	case TypeWebP:
		return KindImage
	case TypeMP4:
		return KindVideo
	case TypeHTML:
		return KindWebpage
	case TypePDF:
		return KindDocument
	default:
		return KindOther
	}
}

// MaxBytes is the size limit for a stored content type.
func MaxBytes(contentType string) int64 {
	switch KindOf(contentType) {
	case KindVideo:
		return MaxVideoBytes
	case KindWebpage:
		return MaxPageBytes
	case KindDocument:
		return MaxPDFBytes
	case KindImage:
		return MaxOtherBytes
	default:
		return MaxOtherBytes
	}
}

// FileRef points at stored bytes by their SHA-256. It is what posts, replies
// and template data carry; the bytes live only in object storage.
type FileRef struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	// PreviewSHA256 is set for images: a PreviewSide-pixel WebP.
	PreviewSHA256 string `json:"preview_sha256,omitempty"`
}

// Validate checks a reference coming from an untrusted caller.
func (f FileRef) Validate() error {
	if !ValidSHA256(f.SHA256) {
		return kernel.Invalid("file %q has a bad sha256", f.Name).WithFix("send the lowercase hex SHA-256 of the file")
	}
	if f.PreviewSHA256 != "" && !ValidSHA256(f.PreviewSHA256) {
		return kernel.Invalid("file %q has a bad preview sha256", f.Name)
	}
	if f.Size <= 0 {
		return kernel.Invalid("file %q is empty", f.Name)
	}
	if !Allowed(f.Type) {
		return kernel.Invalid("file %q has type %q, which is not stored", f.Name, f.Type).
			WithFix("use %s, %s, %s, %s or %s", TypeWebP, TypeMP4, TypeHTML, TypePDF, TypeOctetStream)
	}
	if max := MaxBytes(f.Type); f.Size > max {
		return kernel.Invalid("file %q is %s; the limit for %s is %s", f.Name, HumanSize(f.Size), KindOf(f.Type), HumanSize(max))
	}
	if err := ValidName(f.Name); err != nil {
		return err
	}
	return nil
}

// Allowed reports whether Cadenza stores this content type.
func Allowed(contentType string) bool {
	switch contentType {
	case TypeWebP, TypeMP4, TypeHTML, TypePDF, TypeOctetStream:
		return true
	}
	return false
}

// ValidSHA256 reports whether s is 64 lowercase hex characters.
func ValidSHA256(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// ValidName checks a display file name: a base name, no path parts.
func ValidName(name string) error {
	if name == "" || len(name) > 255 || name != filepath.Base(name) || name == "." || name == ".." ||
		strings.ContainsAny(name, "/\\\x00") {
		return kernel.Invalid("file name %q is not allowed", name).WithFix("use a plain file name without folders")
	}
	return nil
}

// ObjectKey is where the bytes live in object storage. Content addressing
// means the same file is stored once.
func ObjectKey(sha256 string) string { return "sha256/" + sha256 }

// HumanSize formats a byte count as e.g. "12.5 MB".
func HumanSize(n int64) string {
	switch {
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
