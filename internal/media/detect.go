package media

import (
	"bytes"
	"path/filepath"
	"strings"
)

// Source is what the CLI sees before conversion: what a local file is and
// what it must become.
type Source string

const (
	SourceWebP    Source = "webp"   // already the stored image format
	SourceRaster  Source = "raster" // PNG, JPEG or GIF: convert to WebP
	SourceMP4     Source = "mp4"    // check codec and fast-start
	SourceVideo   Source = "video"  // other video: needs ffmpeg
	SourceHTML    Source = "html"   // must be one self-contained file
	SourcePDF     Source = "pdf"
	SourceMermaid Source = "mermaid" // diagram text
	SourceVega    Source = "vega"    // chart spec
	SourceText    Source = "text"    // markdown or plain text
	SourceOther   Source = "other"   // download only
)

// Detect classifies a file from its name and first bytes. Magic bytes win
// over the extension when they disagree.
func Detect(name string, head []byte) Source {
	switch {
	case len(head) >= 12 && bytes.Equal(head[0:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return SourceWebP
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")),
		bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}),
		bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return SourceRaster
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return SourcePDF
	case len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")):
		brand := string(head[8:12])
		if strings.HasPrefix(brand, "qt") {
			return SourceVideo
		}
		if strings.EqualFold(filepath.Ext(name), ".mp4") || strings.EqualFold(filepath.Ext(name), ".m4v") || brand == "isom" || brand == "mp42" || brand == "mp41" || brand == "avc1" {
			return SourceMP4
		}
		return SourceVideo
	case len(head) >= 4 && bytes.Equal(head[0:4], []byte{0x1A, 0x45, 0xDF, 0xA3}): // WebM / MKV
		return SourceVideo
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return SourceHTML
	case ".mmd", ".mermaid":
		return SourceMermaid
	case ".vl.json", ".vega":
		return SourceVega
	case ".md", ".markdown", ".txt":
		return SourceText
	case ".mov", ".webm", ".mkv", ".avi":
		return SourceVideo
	}
	if strings.HasSuffix(strings.ToLower(name), ".vl.json") {
		return SourceVega
	}
	return SourceOther
}
