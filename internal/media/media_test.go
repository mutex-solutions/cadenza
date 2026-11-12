package media

import (
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

const sha = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

func TestFileRefValidate(t *testing.T) {
	ok := FileRef{SHA256: sha, Size: 1000, Type: TypeWebP, Name: "after.webp"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	cases := map[string]FileRef{
		"bad sha":      {SHA256: "xyz", Size: 1, Type: TypeWebP, Name: "a"},
		"upper sha":    {SHA256: strings.ToUpper(sha), Size: 1, Type: TypeWebP, Name: "a"},
		"empty":        {SHA256: sha, Size: 0, Type: TypeWebP, Name: "a"},
		"bad type":     {SHA256: sha, Size: 1, Type: "image/gif", Name: "a.gif"},
		"video limit":  {SHA256: sha, Size: MaxVideoBytes + 1, Type: TypeMP4, Name: "a.mp4"},
		"page limit":   {SHA256: sha, Size: MaxPageBytes + 1, Type: TypeHTML, Name: "a.html"},
		"path in name": {SHA256: sha, Size: 1, Type: TypePDF, Name: "../a.pdf"},
		"no name":      {SHA256: sha, Size: 1, Type: TypePDF, Name: ""},
		"bad preview":  {SHA256: sha, Size: 1, Type: TypeWebP, Name: "a", PreviewSHA256: "nope"},
	}
	for name, ref := range cases {
		if err := ref.Validate(); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
}

func TestPNGFixMessage(t *testing.T) {
	err := FileRef{SHA256: sha, Size: 1, Type: "image/gif", Name: "a.gif"}.Validate()
	if !strings.Contains(err.Error(), "fix: use image/webp") {
		t.Fatalf("missing exact fix: %v", err)
	}
}

func TestOwnerPhotoFallbackTypes(t *testing.T) {
	for _, typ := range []string{TypeJPEG, TypePNG} {
		ref := FileRef{SHA256: sha, Size: 1000, Type: typ, Name: "photo"}
		if err := ref.Validate(); err != nil || KindOf(typ) != KindImage {
			t.Errorf("%s: owner photos from browsers without WebP must be stored as images: %v", typ, err)
		}
	}
}

func TestKindsAndLimits(t *testing.T) {
	if KindOf(TypeWebP) != KindImage || KindOf(TypeMP4) != KindVideo || KindOf(TypeHTML) != KindWebpage ||
		KindOf(TypePDF) != KindDocument || KindOf("x/y") != KindOther {
		t.Fatal("KindOf wrong")
	}
	if MaxBytes(TypeMP4) != 500*MB || MaxBytes(TypeHTML) != 10*MB || MaxBytes(TypePDF) != 25*MB || MaxBytes(TypeOctetStream) != 25*MB {
		t.Fatal("MaxBytes wrong")
	}
	if ObjectKey(sha) != "sha256/"+sha {
		t.Fatal("ObjectKey wrong")
	}
	if HumanSize(512) != "512 B" || HumanSize(2048) != "2.0 KB" || HumanSize(3*MB) != "3.0 MB" {
		t.Fatal("HumanSize wrong")
	}
}

func TestDetect(t *testing.T) {
	ftyp := func(brand string) []byte { return append([]byte{0, 0, 0, 0x20}, []byte("ftyp"+brand)...) }
	cases := []struct {
		name string
		head []byte
		want Source
	}{
		{"a.webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), SourceWebP},
		{"a.png", []byte("\x89PNG\r\n\x1a\nxxxx"), SourceRaster},
		{"a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, SourceRaster},
		{"a.gif", []byte("GIF89a......"), SourceRaster},
		{"a.pdf", []byte("%PDF-1.7"), SourcePDF},
		{"a.mp4", ftyp("isom"), SourceMP4},
		{"clip", ftyp("mp42"), SourceMP4},
		{"a.mov", ftyp("qt  "), SourceVideo},
		{"a.webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, SourceVideo},
		{"report.html", []byte("<!doctype html>"), SourceHTML},
		{"flow.mmd", []byte("graph TD"), SourceMermaid},
		{"chart.vl.json", []byte("{"), SourceVega},
		{"notes.md", []byte("# hi"), SourceText},
		{"data.bin", []byte{1, 2, 3}, SourceOther},
		{"lying.png", []byte("%PDF-1.4"), SourcePDF}, // magic wins
	}
	for _, c := range cases {
		if got := Detect(c.name, c.head); got != c.want {
			t.Errorf("Detect(%q) = %s, want %s", c.name, got, c.want)
		}
	}
}
