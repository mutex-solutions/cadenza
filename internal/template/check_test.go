package template

import (
	"strings"
	"testing"
)

type blockCase struct {
	name  string
	block Block
	mode  Mode
	value string
	want  []string // parts of the error text; nil means the value is valid
}

func runBlockCases(t *testing.T, cases []blockCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(string(tc.block)+"/"+tc.name, func(t *testing.T) {
			mode := tc.mode
			if mode == 0 {
				mode = Local
			}
			_, err := oneSlot(tc.block).Validate(raw(`{"x": `+tc.value+`}`), mode)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			e := kerr(t, err)
			if e.Fix == "" {
				t.Errorf("error has no fix: %v", e)
			}
			mustContain(t, e.Error(), tc.want...)
		})
	}
}

func TestTextBlock(t *testing.T) {
	long := `"` + strings.Repeat("a", MaxTextChars+1) + `"`
	runBlockCases(t, []blockCase{
		{"ok", BlockText, 0, `"Some **md**"`, nil},
		{"unicode counts characters", BlockText, 0, `"` + strings.Repeat("é", MaxTextChars) + `"`, nil},
		{"number", BlockText, 0, `12`, []string{`slot "x" needs markdown text`, `fix: pass "Some **markdown**"`}},
		{"blank", BlockText, 0, `"  "`, []string{`slot "x" is empty`, "write some text"}},
		{"too long", BlockText, 0, long, []string{"20001 characters; the limit is 20000", "shorten it"}},
	})
}

func TestCalloutBlock(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"ok", BlockCallout, 0, `{"tone": "danger", "text": "Broke"}`, nil},
		{"tone defaults", BlockCallout, 0, `{"text": "Hi"}`, nil},
		{"string", BlockCallout, 0, `"Hi"`, []string{`slot "x" needs a callout`, `pass {"tone": "info", "text": "Some markdown"}`}},
		{"bad tone", BlockCallout, 0, `{"tone": "red", "text": "x"}`, []string{`has tone "red"`, "use one of: info, success, warning, danger"}},
		{"no text", BlockCallout, 0, `{"tone": "info"}`, []string{`slot "x" needs field "text"`}},
		{"unknown field", BlockCallout, 0, `{"text": "x", "title": "y"}`, []string{`unknown field "title"`, "use only: tone, text"}},
		{"text not string", BlockCallout, 0, `{"text": 3}`, []string{`slot "x" field "text" must be a string`}},
	})
}

func TestChecklistBlock(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"ok", BlockChecklist, 0, `[{"text": "a", "done": true}, {"text": "b"}]`, nil},
		{"not array", BlockChecklist, 0, `{"text": "a"}`, []string{"needs a checklist", `[{"text": "Write tests", "done": true}]`}},
		{"empty", BlockChecklist, 0, `[]`, []string{`slot "x" is empty`, "leave the slot out"}},
		{"item string", BlockChecklist, 0, `["a"]`, []string{`slot "x" item 1 needs a checklist item`}},
		{"done not bool", BlockChecklist, 0, `[{"text": "a", "done": "yes"}]`, []string{`item 1 field "done" must be true or false`}},
		{"missing text", BlockChecklist, 0, `[{"done": true}]`, []string{`item 1 needs field "text"`}},
		{"too many", BlockChecklist, 0, `[` + strings.TrimSuffix(strings.Repeat(`{"text":"a"},`, MaxListItems+1), ",") + `]`, []string{"101 items; the limit is 100", "keep the 100 most important"}},
	})
}

func TestListBlock(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"ok", BlockList, 0, `["a", "b"]`, nil},
		{"string", BlockList, 0, `"a"`, []string{`slot "x" needs a list`, `pass ["First point", "Second point"]`}},
		{"item number", BlockList, 0, `["a", 2]`, []string{`slot "x" item 2 must be a string`}},
		{"item blank", BlockList, 0, `[""]`, []string{`slot "x" item 1 is empty`}},
	})
}

func TestFileBlocksLocal(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"image ok", BlockImage, Local, `{"file": "./a.png"}`, nil},
		{"image as string", BlockImage, Local, `"./a.png"`, []string{`slot "x" needs an image`, `fix: pass {"file": "<path>"}`}},
		{"image ref in local", BlockImage, Local, `{"file": ` + webp + `}`, []string{"holds a stored file reference", `pass {"file": "<path>"}`}},
		{"image url", BlockImage, Local, `{"file": "https://x.dev/a.png"}`, []string{"has a URL", "download it and pass the local path"}},
		{"image empty path", BlockImage, Local, `{"file": " "}`, []string{"empty file path"}},
		{"image nul", BlockImage, Local, `{"file": "a\u0000b"}`, []string{"not allowed", "normal path"}},
		{"image no file", BlockImage, Local, `{}`, []string{`slot "x" needs field "file"`}},
		{"image extra field", BlockImage, Local, `{"file": "a", "alt": "b"}`, []string{`unknown field "alt"`, "use only: file"}},
		{"image number", BlockImage, Local, `{"file": 3}`, []string{`slot "x" needs an image`}},
		{"video ok", BlockVideo, Local, `{"file": "a.mov"}`, nil},
		{"webpage ok", BlockWebpage, Local, `{"file": "r.html"}`, nil},
		{"file ok", BlockFile, Local, `{"file": "r.zip"}`, nil},
	})
}

func TestFileBlocksRemote(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"image ok", BlockImage, Remote, `{"file": ` + webp + `}`, nil},
		{"image path", BlockImage, Remote, `{"file": "./a.png"}`, []string{"is a local path", "upload the file first", `"type": "image/webp"`}},
		{"image wrong type", BlockImage, Remote, `{"file": ` + pdf + `}`, []string{`needs an image (image/webp), but file "a.pdf" is application/pdf`, "convert it to WebP"}},
		{"image bad sha", BlockImage, Remote, `{"file": {"sha256": "x", "size": 1, "type": "image/webp", "name": "a.webp"}}`, []string{"bad sha256", "lowercase hex SHA-256"}},
		{"image empty file", BlockImage, Remote, `{"file": {"sha256": "` + sha + `", "size": 0, "type": "image/webp", "name": "a.webp"}}`, []string{"is empty", "upload the file again"}},
		{"image unknown ref field", BlockImage, Remote, `{"file": {"sha256": "` + sha + `", "size": 1, "type": "image/webp", "name": "a", "url": "x"}}`, []string{"bad file reference"}},
		{"image as string remote", BlockImage, Remote, `7`, []string{`needs an image`, `{"file": {"sha256": "<hex>"`}},
		{"video ok", BlockVideo, Remote, `{"file": ` + mp4 + `}`, nil},
		{"video wrong", BlockVideo, Remote, `{"file": ` + webp + `}`, []string{"needs a video (video/mp4)", "convert it to MP4"}},
		{"webpage ok", BlockWebpage, Remote, `{"file": ` + html + `}`, nil},
		{"webpage wrong", BlockWebpage, Remote, `{"file": ` + pdf + `}`, []string{"needs a webpage (text/html)", "self-contained HTML"}},
		{"file any", BlockFile, Remote, `{"file": ` + pdf + `}`, nil},
		{"file shape any type", BlockFile, Remote, `"x"`, []string{`"type": "<type>"`}},
	})
}

func TestGalleryAndCompareBlocks(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"gallery ok", BlockGallery, Local, `[{"file": "a.png", "caption": "A"}, {"file": "b.png"}]`, nil},
		{"gallery remote ok", BlockGallery, Remote, `[{"file": ` + webp + `}]`, nil},
		{"gallery remote wrong type", BlockGallery, Remote, `[{"file": ` + mp4 + `}]`, []string{`item 1 needs an image`}},
		{"gallery not array", BlockGallery, Local, `{"file": "a.png"}`, []string{"needs a gallery", `[{"file": "<path>", "caption": "Home screen"}]`}},
		{"gallery missing file", BlockGallery, Local, `[{"caption": "A"}]`, []string{`item 1 needs field "file"`}},
		{"gallery bad caption", BlockGallery, Local, `[{"file": "a", "caption": 1}]`, []string{`item 1 field "caption" must be a string`}},
		{"gallery item string", BlockGallery, Local, `["a.png"]`, []string{"item 1 needs an image with an optional caption"}},
		{"gallery too many", BlockGallery, Local, `[` + strings.TrimSuffix(strings.Repeat(`{"file":"a"},`, MaxGalleryItems+1), ",") + `]`, []string{"31 items; the limit is 30"}},
		{"compare ok", BlockCompare, Local, `{"before": {"file": "a"}, "after": {"file": "b"}, "caption": "c"}`, nil},
		{"compare remote ok", BlockCompare, Remote, `{"before": {"file": ` + webp + `}, "after": {"file": ` + webp + `}}`, nil},
		{"compare missing after", BlockCompare, Local, `{"before": {"file": "a"}}`, []string{`needs field "after"`, `"before": {"file": "<path>"}`}},
		{"compare bare path", BlockCompare, Local, `{"before": "a", "after": {"file": "b"}}`, []string{`slot "x" field "before" needs an image`}},
		{"compare string", BlockCompare, Local, `"a"`, []string{"needs a before/after comparison"}},
	})
}

func TestChartTableMetricsBlocks(t *testing.T) {
	rows := `[` + strings.TrimSuffix(strings.Repeat(`["a"],`, MaxTableRows+1), ",") + `]`
	runBlockCases(t, []blockCase{
		{"chart ok", BlockChart, 0, `{"mark": "bar"}`, nil},
		{"chart array", BlockChart, 0, `[1]`, []string{"needs a Vega-Lite chart", `"mark": "bar"`}},
		{"chart empty", BlockChart, 0, `{}`, []string{"empty chart"}},
		{"table ok", BlockTable, 0, `{"columns": ["a", "b"], "rows": [["x", 1], [true, null]]}`, nil},
		{"table no rows ok", BlockTable, 0, `{"columns": ["a"], "rows": []}`, nil},
		{"table no columns", BlockTable, 0, `{"rows": []}`, []string{`needs field "columns"`}},
		{"table no rows field", BlockTable, 0, `{"columns": ["a"]}`, []string{`needs field "rows"`}},
		{"table rows not array", BlockTable, 0, `{"columns": ["a"], "rows": {}}`, []string{`field "rows" needs a list of rows`}},
		{"table row not array", BlockTable, 0, `{"columns": ["a"], "rows": ["x"]}`, []string{"row 1 needs a list of cells"}},
		{"table row width", BlockTable, 0, `{"columns": ["a", "b"], "rows": [["x"]]}`, []string{"row 1 has 1 cells but there are 2 columns", "use null"}},
		{"table bad cell", BlockTable, 0, `{"columns": ["a"], "rows": [[{"x": 1}]]}`, []string{"row 1 cell 1 must be text", "turn the cell into a string"}},
		{"table long cell", BlockTable, 0, `{"columns": ["a"], "rows": [["` + strings.Repeat("a", MaxCellChars+1) + `"]]}`, []string{"cell 1 is 1001 characters"}},
		{"table too many rows", BlockTable, 0, `{"columns": ["a"], "rows": ` + rows + `}`, []string{"501 rows; the limit is 500", "show the full table as a file"}},
		{"table blank column", BlockTable, 0, `{"columns": [""], "rows": []}`, []string{`field "columns" item 1 is empty`}},
		{"table string", BlockTable, 0, `"a"`, []string{"needs a table"}},
		{"metrics ok", BlockMetrics, 0, `[{"label": "p95", "value": 120, "before": "340", "delta": -220, "unit": "ms", "good": "down"}]`, nil},
		{"metrics no value", BlockMetrics, 0, `[{"label": "p95"}]`, []string{`item 1 needs field "value"`}},
		{"metrics no label", BlockMetrics, 0, `[{"value": 1}]`, []string{`item 1 needs field "label"`}},
		{"metrics bool value", BlockMetrics, 0, `[{"label": "a", "value": true}]`, []string{`field "value" must be a number or a short string`, `120 or "1.2 s"`}},
		{"metrics bad before", BlockMetrics, 0, `[{"label": "a", "value": 1, "before": ""}]`, []string{`field "before" must be a number`}},
		{"metrics bad good", BlockMetrics, 0, `[{"label": "a", "value": 1, "good": "higher"}]`, []string{`field "good" is "higher"`, `use "up" when a higher number is better`}},
		{"metrics item string", BlockMetrics, 0, `["a"]`, []string{"item 1 needs a key number"}},
		{"metrics object", BlockMetrics, 0, `{}`, []string{"needs key numbers"}},
	})
}

func TestCodeDiffDiagramBlocks(t *testing.T) {
	diff := `"--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b"`
	runBlockCases(t, []blockCase{
		{"code ok", BlockCode, 0, `{"language": "go", "code": "x := 1", "filename": "a.go"}`, nil},
		{"code missing", BlockCode, 0, `{"language": "go"}`, []string{`needs field "code"`}},
		{"code string", BlockCode, 0, `"x"`, []string{"needs code", `"code": "fmt.Println(1)"`}},
		{"code long", BlockCode, 0, `{"code": "` + strings.Repeat("a", MaxCodeChars+1) + `"}`, []string{"100001 characters"}},
		{"code bad language", BlockCode, 0, `{"code": "x", "language": 1}`, []string{`field "language" must be a string`}},
		{"diff ok", BlockDiff, 0, `{"diff": ` + diff + `, "filename": "x"}`, nil},
		{"diff not unified", BlockDiff, 0, `{"diff": "a changed to b"}`, []string{"not a unified diff", "output of git diff"}},
		{"diff missing", BlockDiff, 0, `{}`, []string{`needs field "diff"`}},
		{"diff string", BlockDiff, 0, diff, []string{"needs a diff"}},
		{"diagram ok", BlockDiagram, 0, `{"mermaid": "graph TD; A-->B"}`, nil},
		{"diagram string", BlockDiagram, 0, `"graph TD"`, []string{"needs a Mermaid diagram", `{"mermaid": "graph TD; A-->B"}`}},
		{"diagram missing", BlockDiagram, 0, `{}`, []string{`needs field "mermaid"`}},
		{"diagram long", BlockDiagram, 0, `{"mermaid": "` + strings.Repeat("a", MaxMermaidChars+1) + `"}`, []string{"20001 characters"}},
	})
}

func TestLinksAndChoicesBlocks(t *testing.T) {
	runBlockCases(t, []blockCase{
		{"links ok", BlockLinks, 0, `[{"title": "Go", "url": "https://go.dev"}, {"title": "x", "url": "http://a.b/c"}]`, nil},
		{"links ftp", BlockLinks, 0, `[{"title": "x", "url": "ftp://a.b"}]`, []string{"not an http or https address", "starting with https://"}},
		{"links relative", BlockLinks, 0, `[{"title": "x", "url": "/docs"}]`, []string{"not an http or https address"}},
		{"links js", BlockLinks, 0, `[{"title": "x", "url": "javascript:alert(1)"}]`, []string{"not an http or https address"}},
		{"links no title", BlockLinks, 0, `[{"url": "https://a.b"}]`, []string{`item 1 needs field "title"`}},
		{"links no url", BlockLinks, 0, `[{"title": "a"}]`, []string{`item 1 needs field "url"`}},
		{"links string", BlockLinks, 0, `["https://a.b"]`, []string{"item 1 needs a link"}},
		{"choices ok", BlockChoices, 0, `[{"title": "A", "text": "why", "pros": ["p"], "cons": ["c"]}, {"title": "B"}]`, nil},
		{"choices one", BlockChoices, 0, `[{"title": "A"}]`, []string{"has 1 choice; at least 2 are needed", "add another choice"}},
		{"choices duplicate", BlockChoices, 0, `[{"title": "A"}, {"title": " a "}]`, []string{"same title as another choice", "different title"}},
		{"choices no title", BlockChoices, 0, `[{"title": "A"}, {"text": "x"}]`, []string{`item 2 needs field "title"`}},
		{"choices bad pros", BlockChoices, 0, `[{"title": "A", "pros": "fast"}, {"title": "B"}]`, []string{`field "pros" needs a list of short points`}},
		{"choices blank con", BlockChoices, 0, `[{"title": "A", "cons": [""]}, {"title": "B"}]`, []string{`field "cons" item 1 is empty`}},
		{"choices bad text", BlockChoices, 0, `[{"title": "A", "text": 1}, {"title": "B"}]`, []string{`field "text" must be a string`}},
		{"choices item string", BlockChoices, 0, `["A", "B"]`, []string{"item 1 needs a choice"}},
		{"choices too many", BlockChoices, 0, `[` + strings.TrimSuffix(strings.Repeat(`{"title":"a"},`, MaxChoices+1), ",") + `]`, []string{"11 items; the limit is 10"}},
	})
}

func TestSlotMaxLowersLimit(t *testing.T) {
	d := Definition{Name: "t", Slots: []Slot{
		{Name: "s", Block: BlockText, Required: true, Max: 5},
		{Name: "l", Block: BlockList, Max: 2},
	}}
	_, err := d.Validate(raw(`{"s": "123456", "l": ["a", "b", "c"]}`), Local)
	mustContain(t, kerr(t, err).Error(), "6 characters; the limit is 5", "3 items; the limit is 2")
	if _, err := d.Validate(raw(`{"s": "12345", "l": ["a", "b"]}`), Local); err != nil {
		t.Fatal(err)
	}
}

func TestEveryBlockHasSpecAndShape(t *testing.T) {
	for _, b := range Blocks() {
		s, ok := specs[b]
		if !ok || s.noun == "" || s.shape == "" {
			t.Errorf("block %q has no spec", b)
		}
		if !b.Valid() {
			t.Errorf("block %q not valid", b)
		}
	}
	if len(Blocks()) != len(specs) {
		t.Fatalf("Blocks() and specs disagree")
	}
	if Block("slider").Valid() {
		t.Fatal("unknown block reported valid")
	}
}

func TestCompactTruncates(t *testing.T) {
	if got := compact(strings.Repeat("a", 100)); len(got) != 60 || !strings.HasSuffix(got, "...") {
		t.Fatalf("compact = %q", got)
	}
	if got := compact("<b>"); got != `"<b>"` {
		t.Fatalf("compact escaped HTML: %s", got)
	}
}
