package prepare

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

func TestMissingFile(t *testing.T) {
	_, err := PrepareFile(context.Background(), "/nonexistent/x.png", Options{})
	if !kernel.Is(err, kernel.KindNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestFolderRefused(t *testing.T) {
	_, err := PrepareFile(context.Background(), t.TempDir(), Options{})
	e := wantInvalid(t, err)
	if !strings.HasPrefix(e.Fix, "make it one file") {
		t.Fatalf("fix = %q", e.Fix)
	}
}

func TestEmptyFileRefused(t *testing.T) {
	_, err := PrepareFile(context.Background(), writeFile(t, "a.pdf", nil), Options{})
	wantInvalid(t, err)
}

func TestCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareFile(ctx, writeFile(t, "a.pdf", []byte("%PDF-1.7")), Options{}); err == nil {
		t.Fatal("want error for cancelled context")
	}
}

func TestSecretFileRefused(t *testing.T) {
	_, err := PrepareFile(context.Background(), writeFile(t, ".env", []byte("A=1")), Options{})
	wantInvalid(t, err)
}

func TestPDFPassThrough(t *testing.T) {
	path := writeFile(t, "report.pdf", []byte("%PDF-1.7 hello"))
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != media.SourcePDF || p.File.Type != media.TypePDF || p.Path != path || p.Data != nil || p.File.Name != "report.pdf" {
		t.Fatalf("got %+v", p)
	}
	checkRef(t, p)
	if p.Cleanup() != nil {
		t.Fatal("cleanup of pass-through must be a no-op")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("cleanup removed the original file")
	}
}

func TestPDFLimit(t *testing.T) {
	path := writeFile(t, "report.pdf", []byte("%PDF-1.7 hello world"))
	_, err := PrepareFile(context.Background(), path, Options{Limits: Limits{PDFBytes: 5}})
	wantInvalid(t, err)
}

func TestOtherPassThrough(t *testing.T) {
	path := writeFile(t, "data.bin", []byte{1, 2, 3})
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != media.SourceOther || p.File.Type != media.TypeOctetStream {
		t.Fatalf("got %+v", p)
	}
	checkRef(t, p)
	_, err = PrepareFile(context.Background(), path, Options{Limits: Limits{OtherBytes: 2}})
	wantInvalid(t, err)
}

func TestInlineSources(t *testing.T) {
	cases := map[string]media.Source{"flow.mmd": media.SourceMermaid, "chart.vl.json": media.SourceVega, "notes.md": media.SourceText}
	for name, src := range cases {
		p, err := PrepareFile(context.Background(), writeFile(t, name, []byte("graph TD; a-->b")), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !p.Inline || p.Source != src || p.Text != "graph TD; a-->b" || p.File != (media.FileRef{}) {
			t.Fatalf("%s: got %+v", name, p)
		}
		if _, err := p.Open(); err == nil {
			t.Fatalf("%s: Open must fail for inline text", name)
		}
	}
}

func TestInlineRedactsAndLimits(t *testing.T) {
	key := "AKIA" + "ABCDEFGHIJKLMNOP"
	path := writeFile(t, "notes.md", []byte("key "+key+" here"))
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Text, key) || !strings.Contains(p.Text, "[redacted aws-access-key]") || len(p.Warnings) != 1 {
		t.Fatalf("got %+v", p)
	}
	_, err = PrepareFile(context.Background(), path, Options{Limits: Limits{InlineBytes: 3}})
	wantInvalid(t, err)
}

func TestReplaceExt(t *testing.T) {
	if replaceExt("/a/b/shot.final.png", ".webp") != "shot.final.webp" || replaceExt("x", ".mp4") != "x.mp4" {
		t.Fatal("replaceExt wrong")
	}
}
