package template

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/media"
)

func TestRenderView(t *testing.T) {
	d := Definition{Name: "r", Version: 3, Slots: []Slot{
		{Name: "summary", Block: BlockText, Required: true, Label: "Summary"},
		{Name: "note", Block: BlockCallout, Label: "Note"},
		{Name: "steps", Block: BlockChecklist, Label: "Steps"},
		{Name: "changes", Block: BlockList, Label: "Changes"},
		{Name: "after", Block: BlockImage, Label: "After"},
		{Name: "opts", Block: BlockChoices, Label: "Options"},
		{Name: "code", Block: BlockCode, Label: "Code"},
		{Name: "empty", Block: BlockList, Label: "Empty"},
	}}
	data := raw(`{
		"opts": [{"title": "A", "text": "go *a*", "pros": ["p"]}, {"title": "B"}],
		"summary": "hello **x**",
		"note": {"text": "careful"},
		"steps": [{"text": "one"}],
		"changes": ["c1"],
		"after": {"file": ` + webp + `},
		"code": {"code": "if a < b {}"}
	}`)
	out, err := d.Render(data, upperRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Template string `json:"template"`
		Version  int    `json:"version"`
		Slots    []struct {
			Name  string          `json:"name"`
			Block Block           `json:"block"`
			Label string          `json:"label"`
			Value json.RawMessage `json:"value"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.Template != "r" || v.Version != 3 || len(v.Slots) != 7 {
		t.Fatalf("view header wrong: %s", out)
	}
	order := []string{"summary", "note", "steps", "changes", "after", "opts", "code"}
	for i, s := range v.Slots {
		if s.Name != order[i] {
			t.Fatalf("slot %d is %q, want %q", i, s.Name, order[i])
		}
	}
	want := map[string]string{
		"summary": `{"md":"hello **x**","html":"<p>HELLO **X**</p>"}`,
		"note":    `{"text":{"md":"careful","html":"<p>CAREFUL</p>"},"tone":"info"}`,
		"steps":   `[{"done":false,"text":{"md":"one","html":"<p>ONE</p>"}}]`,
		"changes": `[{"md":"c1","html":"<p>C1</p>"}]`,
		"opts":    `[{"pros":["p"],"text":{"md":"go *a*","html":"<p>GO *A*</p>"},"title":"A"},{"title":"B"}]`,
		"code":    `{"code":"if a < b {}"}`,
	}
	for _, s := range v.Slots {
		if w, ok := want[s.Name]; ok && string(s.Value) != w {
			t.Errorf("%s:\n got %s\nwant %s", s.Name, s.Value, w)
		}
	}
	var img struct{ File media.FileRef }
	if err := json.Unmarshal(v.Slots[4].Value, &img); err != nil || img.File.SHA256 != sha || v.Slots[4].Label != "After" {
		t.Fatalf("image slot wrong: %s", v.Slots[4].Value)
	}
}

func TestRenderNeedsRemoteData(t *testing.T) {
	d := Definition{Name: "r", Slots: []Slot{{Name: "after", Block: BlockImage, Required: true}}}
	_, err := d.Render(raw(`{"after": {"file": "./a.png"}}`), upperRenderer{})
	mustContain(t, kerr(t, err).Error(), "is a local path", "upload the file first")
}

func TestRenderEmptyOptionalSlots(t *testing.T) {
	d := Definition{Name: "r", Slots: []Slot{{Name: "a", Block: BlockText}}}
	out, err := d.Render(raw(`{}`), upperRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"slots":[]`) {
		t.Fatalf("want empty slot list, got %s", out)
	}
}

func TestRenderBuiltinExamplesAfterUpload(t *testing.T) {
	for _, d := range Builtins() {
		res, err := d.Validate(d.Example, Local)
		if err != nil {
			t.Fatal(err)
		}
		refs := map[string]media.FileRef{}
		for _, p := range res.Paths {
			typ := map[media.Kind]string{media.KindImage: media.TypeWebP, media.KindVideo: media.TypeMP4, media.KindWebpage: media.TypeHTML, "": media.TypePDF}[p.Want]
			refs[p.Path] = media.FileRef{SHA256: sha, Size: 100, Type: typ, Name: "f"}
		}
		data, err := ReplacePaths(d.Example, refs)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Render(data, upperRenderer{}); err != nil {
			t.Errorf("%s: %v", d.Name, err)
		}
	}
}
