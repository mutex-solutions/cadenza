// Package prepare turns a local file into bytes Cadenza can store, before
// anything is uploaded. Images become WebP with a preview, MP4 videos are
// checked and made fast-start, HTML pages must be one self-contained file,
// and files that look like secrets are refused. All work is local and pure
// Go: no cgo, no system libraries, and ffmpeg only when the caller allows it.
package prepare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// Defaults used when the matching Options field is zero.
const (
	DefaultWebPQuality = 80
	DefaultWebPMethod  = 4
	// MaxInlineBytes is the largest text source (Mermaid, Vega, Markdown)
	// returned for inlining.
	MaxInlineBytes = 1 * media.MB
	// MaxPixels guards against decompression bombs: images with more pixels
	// than this are refused before they are decoded.
	MaxPixels = 100_000_000
)

// Limits are the size limits PrepareFile enforces. A zero field means the
// media package default. Tests set small values to stay fast.
type Limits struct {
	ImageSide   int   // longest side of the stored image (media.MaxImageSide)
	PreviewSide int   // longest side of the preview (media.PreviewSide)
	VideoHeight int   // tallest stored video (media.MaxVideoHeight)
	VideoBytes  int64 // media.MaxVideoBytes
	PageBytes   int64 // media.MaxPageBytes
	PDFBytes    int64 // media.MaxPDFBytes
	OtherBytes  int64 // media.MaxOtherBytes
	InlineBytes int64 // MaxInlineBytes
}

func (l Limits) withDefaults() Limits {
	def := func(v *int64, d int64) {
		if *v <= 0 {
			*v = d
		}
	}
	if l.ImageSide <= 0 {
		l.ImageSide = media.MaxImageSide
	}
	if l.PreviewSide <= 0 {
		l.PreviewSide = media.PreviewSide
	}
	if l.VideoHeight <= 0 {
		l.VideoHeight = media.MaxVideoHeight
	}
	def(&l.VideoBytes, media.MaxVideoBytes)
	def(&l.PageBytes, media.MaxPageBytes)
	def(&l.PDFBytes, media.MaxPDFBytes)
	def(&l.OtherBytes, media.MaxOtherBytes)
	def(&l.InlineBytes, MaxInlineBytes)
	return l
}

// Runner runs an external command. It exists so tests can replace ffmpeg.
type Runner func(ctx context.Context, name string, args []string) error

// Options tune PrepareFile. The zero value is ready to use.
type Options struct {
	// TempDir is where converted files are written. Empty means os.TempDir().
	TempDir string
	// WebPQuality is 1-100. Zero means DefaultWebPQuality.
	WebPQuality int
	// WebPMethod is 1-6: higher is smaller and slower. Zero means
	// DefaultWebPMethod.
	WebPMethod int
	// AllowFFmpeg lets PrepareFile convert videos that are not H.264 MP4 by
	// running ffmpeg. Without it such videos are refused with the command.
	AllowFFmpeg bool
	// FFmpeg runs ffmpeg. Nil means the ffmpeg found on PATH.
	FFmpeg Runner
	Limits Limits
}

// Preview is the small WebP shown in lists and timelines.
type Preview struct {
	File media.FileRef
	Data []byte
}

// Prepared is a file ready to upload, or text the caller must inline.
type Prepared struct {
	// Source is what the original file was detected as.
	Source media.Source
	// File describes the bytes to upload. It is zero when Inline is true.
	File media.FileRef
	// Path holds the bytes to upload when Data is nil: the original file, or
	// a converted file in a temp folder.
	Path string
	// Data holds the bytes to upload when they were made in memory
	// (converted images, redacted pages).
	Data []byte
	// Preview is set for images.
	Preview *Preview
	// Inline is true for Mermaid, Vega and text sources: they are not
	// uploaded, the caller puts Text into the post instead.
	Inline bool
	// Text is the inline text, with secret-looking parts redacted.
	Text string
	// Warnings are problems that did not stop the file, e.g. external links
	// in a page or redacted secrets. Show them to the agent.
	Warnings []string

	tempDir string
}

// Open returns the bytes to upload.
func (p *Prepared) Open() (io.ReadCloser, error) {
	if p.Inline {
		return nil, kernel.Invalid("%s is inline text, not a file to upload", p.Source)
	}
	if p.Data != nil {
		return io.NopCloser(bytes.NewReader(p.Data)), nil
	}
	return os.Open(p.Path)
}

// Cleanup removes temp files made for this file. It is safe to call twice.
func (p *Prepared) Cleanup() error {
	if p.tempDir == "" {
		return nil
	}
	err := os.RemoveAll(p.tempDir)
	p.tempDir = ""
	return err
}

// PrepareFile checks and converts one local file. The returned error is a
// *kernel.Error; KindInvalid errors carry the exact fix in Fix. Call
// Cleanup on the result when the upload is done.
func PrepareFile(ctx context.Context, path string, opts Options) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, kernel.Internal(err, "prepare %s cancelled", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, kernel.NotFound("file %q does not exist", path).WithFix("check the path; it is relative to the current folder")
		}
		return nil, kernel.Internal(err, "cannot read %s", path)
	}
	if st.IsDir() {
		return nil, kernel.Invalid("%s is a folder, not a file", path).
			WithFix("make it one file: bundle the folder into a single self-contained HTML file, or pass each file by name")
	}
	if st.Size() == 0 {
		return nil, kernel.Invalid("file %s is empty", path)
	}
	head, err := readHead(path, 512)
	if err != nil {
		return nil, err
	}
	if err := CheckSecrets(path, head); err != nil {
		return nil, err
	}
	opts.Limits = opts.Limits.withDefaults()

	src := media.Detect(filepath.Base(path), head)
	var p *Prepared
	switch src {
	case media.SourceRaster, media.SourceWebP:
		p, err = prepareImage(path, src, st.Size(), opts)
	case media.SourceMP4, media.SourceVideo:
		p, err = prepareVideo(ctx, path, src, st.Size(), opts)
	case media.SourceHTML:
		p, err = prepareHTML(path, st.Size(), opts.Limits.PageBytes)
	case media.SourcePDF:
		p, err = preparePassThrough(path, st.Size(), media.TypePDF, opts.Limits.PDFBytes)
	case media.SourceMermaid, media.SourceVega, media.SourceText:
		p, err = prepareInline(path, src, st.Size(), opts.Limits.InlineBytes)
	default:
		p, err = preparePassThrough(path, st.Size(), media.TypeOctetStream, opts.Limits.OtherBytes)
	}
	if err != nil {
		return nil, err
	}
	p.Source = src
	return p, nil
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, kernel.Internal(err, "cannot open %s", path)
	}
	defer f.Close()
	buf := make([]byte, n)
	m, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, kernel.Internal(err, "cannot read %s", path)
	}
	return buf[:m], nil
}

// tooBig is the error for a file over its limit.
func tooBig(path string, size, limit int64, fix string) *kernel.Error {
	return kernel.Invalid("file %s is %s; the limit is %s", filepath.Base(path), media.HumanSize(size), media.HumanSize(limit)).
		WithFix("%s", fix)
}

func preparePassThrough(path string, size int64, contentType string, limit int64) (*Prepared, error) {
	if size > limit {
		return nil, tooBig(path, size, limit, "make the file smaller, or link to it instead of attaching it")
	}
	sum, n, err := HashFile(path)
	if err != nil {
		return nil, err
	}
	ref := media.FileRef{SHA256: sum, Size: n, Type: contentType, Name: filepath.Base(path)}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return &Prepared{File: ref, Path: path}, nil
}

func prepareInline(path string, src media.Source, size, limit int64) (*Prepared, error) {
	if size > limit {
		return nil, tooBig(path, size, limit, "shorten the text, or attach it as a file with a .txt name")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, kernel.Internal(err, "cannot read %s", path)
	}
	p := &Prepared{Inline: true, Text: string(b)}
	if found := ScanText(p.Text); len(found) > 0 {
		p.Text = Redact(p.Text)
		p.Warnings = append(p.Warnings, redactedWarning(filepath.Base(path), found))
	}
	return p, nil
}

func redactedWarning(name string, found []Finding) string {
	rules := make([]string, 0, len(found))
	seen := map[string]bool{}
	for _, f := range found {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			rules = append(rules, f.Rule)
		}
	}
	return fmt.Sprintf("%s: hid %d secret-looking value(s) (%s)", name, len(found), strings.Join(rules, ", "))
}

// replaceExt returns the base name of path with its extension set to ext.
func replaceExt(path, ext string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base)) + ext
}
