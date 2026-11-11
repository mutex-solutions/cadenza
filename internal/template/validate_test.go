package template

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

func testDef() Definition {
	return Definition{Name: "demo", Slots: []Slot{
		{Name: "summary", Block: BlockText, Required: true},
		{Name: "after", Block: BlockImage},
		{Name: "shots", Block: BlockGallery},
		{Name: "page", Block: BlockWebpage},
		{Name: "cmp", Block: BlockCompare},
		{Name: "clip", Block: BlockVideo},
		{Name: "any", Block: BlockFile},
	}}
}

func TestValidateLocalListsPathsInSlotOrder(t *testing.T) {
	res, err := testDef().Validate(raw(`{
		"any": {"file": "r.zip"},
		"summary": "hi",
		"shots": [{"file": "a.png"}, {"file": "b.png"}],
		"after": {"file": "a.png"},
		"cmp": {"before": {"file": "c.png"}, "after": {"file": "d.png"}},
		"clip": {"file": "v.mov"},
		"page": {"file": "p.html"}
	}`), Local)
	if err != nil {
		t.Fatal(err)
	}
	want := []LocalFile{
		{"a.png", "after", BlockImage, media.KindImage},
		{"a.png", "shots", BlockImage, media.KindImage},
		{"b.png", "shots", BlockImage, media.KindImage},
		{"p.html", "page", BlockWebpage, media.KindWebpage},
		{"c.png", "cmp", BlockImage, media.KindImage},
		{"d.png", "cmp", BlockImage, media.KindImage},
		{"v.mov", "clip", BlockVideo, media.KindVideo},
		{"r.zip", "any", BlockFile, ""},
	}
	if !reflect.DeepEqual(res.Paths, want) {
		t.Fatalf("paths:\n got %+v\nwant %+v", res.Paths, want)
	}
	if len(res.Refs) != 0 {
		t.Fatalf("local mode returned refs")
	}
}

func TestValidateRemoteListsRefs(t *testing.T) {
	res, err := testDef().Validate(raw(`{"summary": "hi", "after": {"file": `+webp+`}, "any": {"file": `+pdf+`}}`), Remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Refs) != 2 || res.Refs[0].Type != media.TypeWebP || res.Refs[1].Name != "a.pdf" || len(res.Paths) != 0 {
		t.Fatalf("refs = %+v paths = %+v", res.Refs, res.Paths)
	}
}

func TestValidateTopLevelProblems(t *testing.T) {
	d := testDef()
	cases := []struct {
		name, data string
		want       []string
	}{
		{"not json", `{"summary": `, []string{"not valid JSON", "pass one JSON object"}},
		{"trailing", `{"summary": "a"} {}`, []string{"extra data after the JSON value"}},
		{"array", `["a"]`, []string{"must be a JSON object keyed by slot name"}},
		{"unknown slot", `{"summary": "a", "title": "x"}`, []string{`template "demo" has no slot "title"`, "use one of: summary, after, shots, page, cmp, clip, any"}},
		{"missing required", `{}`, []string{`slot "summary" is required`, `add "summary": "Some **markdown**"`}},
		{"null required", `{"summary": null}`, []string{`slot "summary" is required`}},
		{"too big", `{"summary": "` + strings.Repeat("a", MaxDataBytes) + `"}`, []string{"the limit is 256.0 KB", "move large content into a file"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := d.Validate(raw(tc.data), Local)
			mustContain(t, kerr(t, err).Error(), tc.want...)
		})
	}
}

func TestValidateNullOptionalIsAbsent(t *testing.T) {
	if _, err := testDef().Validate(raw(`{"summary": "a", "after": null}`), Local); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCollectsSeveralProblems(t *testing.T) {
	_, err := testDef().Validate(raw(`{"after": "a.png", "nope": 1}`), Local)
	e := kerr(t, err)
	mustContain(t, e.Msg, `data for template "demo" has 3 problems`, `(1) template "demo" has no slot "nope"`,
		`(2) slot "summary" is required`, `(3) slot "after" needs an image`)
	mustContain(t, e.Fix, "(1) use one of:", `(3) pass {"file": "<path>"}`)
}

func TestValidateCapsProblemsAtTen(t *testing.T) {
	_, err := oneSlot(BlockList).Validate(raw(`{"x": [1,2,3,4,5,6,7,8,9,10,11,12]}`), Local)
	e := kerr(t, err)
	mustContain(t, e.Msg, "has 12 problems", "(10) slot", "; and 2 more")
	if strings.Contains(e.Msg, "(11)") {
		t.Fatalf("listed more than ten: %s", e.Msg)
	}
}

func TestValidateSingleProblemKeepsExactFix(t *testing.T) {
	d := Definition{Name: "ui", Slots: []Slot{{Name: "after", Block: BlockImage, Required: true}}}
	_, err := d.Validate(raw(`{"after": 1}`), Local)
	e := kerr(t, err)
	if e.Msg != `slot "after" needs an image` || e.Fix != `pass {"file": "<path>"}` {
		t.Fatalf("got msg %q fix %q", e.Msg, e.Fix)
	}
}

func TestValidateRejectsUnknownMode(t *testing.T) {
	for _, m := range []Mode{0, modeAny, 9} {
		if _, err := testDef().Validate(raw(`{"summary": "a"}`), m); !kernel.Is(err, kernel.KindInternal) {
			t.Errorf("mode %d: want internal error, got %v", m, err)
		}
	}
}

func TestExampleOrShape(t *testing.T) {
	d := Definition{Name: "x", Slots: []Slot{{Name: "s", Block: BlockList}}}
	if got := d.exampleOrShape(); got != `{"s": ["First point", "Second point"]}` {
		t.Fatalf("shape = %s", got)
	}
	d.Example = raw(`{"s": ["a"]}`)
	if got := d.exampleOrShape(); got != `{"s": ["a"]}` {
		t.Fatalf("example = %s", got)
	}
	if got := (Definition{}).exampleOrShape(); got != "{}" {
		t.Fatalf("empty = %s", got)
	}
}
