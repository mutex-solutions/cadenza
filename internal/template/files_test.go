package template

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/media"
)

func TestReplacePathsThenRemoteValidates(t *testing.T) {
	d := testDef()
	data := raw(`{"summary": "a < b & c", "after": {"file": "a.png"}, "shots": [{"file": "a.png", "caption": "A"}],
		"cmp": {"before": {"file": "b.png"}, "after": {"file": "c.png"}}, "page": {"file": "p.html"}}`)
	res, err := d.Validate(data, Local)
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]media.FileRef{}
	for _, p := range res.Paths {
		typ := media.TypeWebP
		if p.Want == media.KindWebpage {
			typ = media.TypeHTML
		}
		refs[p.Path] = media.FileRef{SHA256: sha, Size: 10, Type: typ, Name: p.Path}
	}
	out, err := ReplacePaths(data, refs)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := d.Validate(out, Remote)
	if err != nil {
		t.Fatalf("remote validation after replace: %v\n%s", err, out)
	}
	if len(res2.Refs) != len(res.Paths) {
		t.Fatalf("got %d refs for %d paths", len(res2.Refs), len(res.Paths))
	}
	mustContain(t, string(out), `"a < b & c"`) // no < escaping
}

func TestReplacePathsLeavesUnknownPathsAndKeepsNumbers(t *testing.T) {
	out, err := ReplacePaths(raw(`{"x": {"file": "keep.png"}, "n": 12345678901234567890, "list": [{"file": "a"}], "file": 3}`),
		map[string]media.FileRef{"a": {SHA256: sha, Size: 1, Type: media.TypeWebP, Name: "a.webp"}})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if string(v["n"]) != "12345678901234567890" || string(v["x"]) != `{"file":"keep.png"}` || string(v["file"]) != "3" {
		t.Fatalf("unexpected output %s", out)
	}
	mustContain(t, string(v["list"]), `"sha256":"`+sha+`"`, `"name":"a.webp"`)
}

func TestReplacePathsBadJSON(t *testing.T) {
	_, err := ReplacePaths(raw(`{`), nil)
	mustContain(t, kerr(t, err).Error(), "not valid JSON", "fix:")
}

func TestWantType(t *testing.T) {
	if wantType(media.KindImage) != media.TypeWebP || wantType(media.KindVideo) != media.TypeMP4 ||
		wantType(media.KindWebpage) != media.TypeHTML || wantType("") != "" {
		t.Fatal("wantType wrong")
	}
}

// JPEG and PNG are accepted for the owner's photos, but template image slots
// still require WebP: agents always convert, so posts load fast everywhere.
func TestImageSlotsStillRequireWebP(t *testing.T) {
	d, _ := Builtin("ui-change")
	sha := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	for _, typ := range []string{media.TypeJPEG, media.TypePNG} {
		data := []byte(`{"summary":"x","after":{"file":{"sha256":"` + sha + `","size":10,"type":"` + typ + `","name":"a"}}}`)
		if _, err := d.Validate(data, Remote); err == nil || !strings.Contains(err.Error(), "image/webp") {
			t.Errorf("%s accepted in an image slot: %v", typ, err)
		}
	}
}
