package template

import (
	"strings"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// Block is the kind of content a slot holds. The set is fixed (plan D7):
// every viewer knows how to draw each one.
type Block string

const (
	BlockText      Block = "text"      // markdown string
	BlockCallout   Block = "callout"   // {"tone": "info", "text": "markdown"}
	BlockChecklist Block = "checklist" // [{"text": "markdown", "done": true}]
	BlockList      Block = "list"      // ["markdown", ...], a bullet list
	BlockImage     Block = "image"     // {"file": ...}, WebP
	BlockGallery   Block = "gallery"   // [{"file": ..., "caption": "..."}]
	BlockCompare   Block = "compare"   // {"before": {"file": ...}, "after": {"file": ...}, "caption": "..."}
	BlockVideo     Block = "video"     // {"file": ...}, MP4
	BlockChart     Block = "chart"     // a Vega-Lite spec object
	BlockTable     Block = "table"     // {"columns": [...], "rows": [[...]]}
	BlockMetrics   Block = "metrics"   // [{"label": "p95", "value": 120, "before": 340, "unit": "ms", "good": "down"}]
	BlockCode      Block = "code"      // {"language": "go", "code": "...", "filename": "..."}
	BlockDiff      Block = "diff"      // {"diff": "unified diff", "filename": "..."}
	BlockDiagram   Block = "diagram"   // {"mermaid": "graph TD; A-->B"}
	BlockFile      Block = "file"      // {"file": ...}, any stored type
	BlockWebpage   Block = "webpage"   // {"file": ...}, one self-contained HTML file
	BlockLinks     Block = "links"     // [{"title": "...", "url": "https://..."}]
	BlockChoices   Block = "choices"   // [{"title": "...", "text": "markdown", "pros": [...], "cons": [...]}]
)

// Limits on filled data. They keep posts small and fast to load.
const (
	MaxDataBytes    = 256 << 10 // whole data object, as JSON
	MaxTextChars    = 20_000    // any markdown or plain text field
	MaxCodeChars    = 100_000   // code and diff bodies
	MaxMermaidChars = 20_000
	MaxShortChars   = 300 // labels, titles, captions, units, file names
	MaxCellChars    = 1_000
	MaxListItems    = 100 // list, checklist, links
	MaxGalleryItems = 30
	MaxMetrics      = 30
	MaxTableRows    = 500
	MaxTableColumns = 30
	MinChoices      = 2
	MaxChoices      = 10
	MaxSlots        = 30
	MaxPathChars    = 4096
)

// Tones a callout may use.
var Tones = []string{"info", "success", "warning", "danger"}

// blockSpec is what the validator and error messages need to know about a block.
type blockSpec struct {
	noun  string     // "an image": used as `slot "x" needs an image`
	shape string     // a JSON example of a valid value, Local mode
	want  media.Kind // for file blocks: the kind the file must be; "" = any
	max   int        // the default limit Slot.Max may lower; 0 = Max does not apply
}

var specs = map[Block]blockSpec{
	BlockText:      {noun: "markdown text", shape: `"Some **markdown**"`, max: MaxTextChars},
	BlockCallout:   {noun: "a callout", shape: `{"tone": "info", "text": "Some markdown"}`, max: MaxTextChars},
	BlockChecklist: {noun: "a checklist", shape: `[{"text": "Write tests", "done": true}]`, max: MaxListItems},
	BlockList:      {noun: "a list", shape: `["First point", "Second point"]`, max: MaxListItems},
	BlockImage:     {noun: "an image", shape: `{"file": "<path>"}`, want: media.KindImage},
	BlockGallery:   {noun: "a gallery", shape: `[{"file": "<path>", "caption": "Home screen"}]`, max: MaxGalleryItems},
	BlockCompare:   {noun: "a before/after comparison", shape: `{"before": {"file": "<path>"}, "after": {"file": "<path>"}, "caption": "..."}`},
	BlockVideo:     {noun: "a video", shape: `{"file": "<path>"}`, want: media.KindVideo},
	BlockChart:     {noun: "a Vega-Lite chart", shape: `{"mark": "bar", "data": {"values": [{"x": "a", "y": 1}]}, "encoding": {"x": {"field": "x"}, "y": {"field": "y", "type": "quantitative"}}}`},
	BlockTable:     {noun: "a table", shape: `{"columns": ["Name", "Value"], "rows": [["a", 1]]}`, max: MaxTableRows},
	BlockMetrics:   {noun: "key numbers", shape: `[{"label": "p95 latency", "value": 120, "before": 340, "unit": "ms", "good": "down"}]`, max: MaxMetrics},
	BlockCode:      {noun: "code", shape: `{"language": "go", "code": "fmt.Println(1)", "filename": "main.go"}`},
	BlockDiff:      {noun: "a diff", shape: `{"diff": "--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-old\n+new", "filename": "x.go"}`},
	BlockDiagram:   {noun: "a Mermaid diagram", shape: `{"mermaid": "graph TD; A-->B"}`},
	BlockFile:      {noun: "a file", shape: `{"file": "<path>"}`},
	BlockWebpage:   {noun: "a webpage", shape: `{"file": "<path>"}`, want: media.KindWebpage},
	BlockLinks:     {noun: "links", shape: `[{"title": "Go spec", "url": "https://go.dev/ref/spec"}]`, max: MaxListItems},
	BlockChoices:   {noun: "choices", shape: `[{"title": "Option A", "text": "Why", "pros": ["fast"], "cons": ["new dependency"]}]`, max: MaxChoices},
}

// Blocks lists every block in a stable order.
func Blocks() []Block {
	return []Block{
		BlockText, BlockCallout, BlockChecklist, BlockList, BlockImage, BlockGallery, BlockCompare,
		BlockVideo, BlockChart, BlockTable, BlockMetrics, BlockCode, BlockDiff, BlockDiagram,
		BlockFile, BlockWebpage, BlockLinks, BlockChoices,
	}
}

// Valid reports whether b is one of the fixed blocks.
func (b Block) Valid() bool {
	_, ok := specs[b]
	return ok
}

// blockNames is the comma-separated block list used in fix texts.
func blockNames() string {
	names := make([]string, 0, len(specs))
	for _, b := range Blocks() {
		names = append(names, string(b))
	}
	return strings.Join(names, ", ")
}
