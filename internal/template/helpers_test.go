package template

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

const sha = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

// ref returns a stored file reference as JSON.
func ref(typ, name string) string {
	return fmt.Sprintf(`{"sha256": %q, "size": 1000, "type": %q, "name": %q}`, sha, typ, name)
}

var (
	webp = ref(media.TypeWebP, "a.webp")
	mp4  = ref(media.TypeMP4, "a.mp4")
	html = ref(media.TypeHTML, "a.html")
	pdf  = ref(media.TypePDF, "a.pdf")
)

// oneSlot is a template with a single required slot "x" of block b.
func oneSlot(b Block) Definition {
	return Definition{Name: "t", Slots: []Slot{{Name: "x", Block: b, Required: true, Label: "X"}}}
}

// kerr asserts err is a kernel.Invalid error and returns it.
func kerr(t *testing.T, err error) *kernel.Error {
	t.Helper()
	if !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("want invalid error, got %v", err)
	}
	return err.(*kernel.Error)
}

// mustContain fails unless s contains every part.
func mustContain(t *testing.T, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Errorf("%q\n  does not contain %q", s, p)
		}
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// upperRenderer is a fake Renderer that makes its work visible.
type upperRenderer struct{}

func (upperRenderer) Render(md string) string { return "<p>" + strings.ToUpper(md) + "</p>" }
