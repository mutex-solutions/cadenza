package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// markdown marks a normalized string that Render turns into {"md", "html"}.
type markdown string

// checkBlock validates one filled slot and returns its normalized value, or
// nil when it has problems.
func checkBlock(c *checker, s Slot, v any) any {
	spec := specs[s.Block]
	lim := spec.max
	if s.Max > 0 {
		lim = s.Max
	}
	top := fmt.Sprintf("slot %q", s.Name)
	n := len(c.problems) + c.extra
	var out any
	switch s.Block {
	case BlockText:
		out = checkText(c, top, v, lim)
	case BlockCallout:
		out = checkCallout(c, top, v, lim)
	case BlockChecklist:
		out = checkChecklist(c, top, v, lim)
	case BlockList:
		out = checkList(c, top, v, lim)
	case BlockImage, BlockVideo, BlockWebpage, BlockFile:
		out = c.fileValue(top, v, s.Block)
	case BlockGallery:
		out = checkGallery(c, top, v, lim)
	case BlockCompare:
		out = checkCompare(c, top, v)
	case BlockChart:
		out = checkChart(c, top, v)
	case BlockTable:
		out = checkTable(c, top, v, lim)
	case BlockMetrics:
		out = checkMetrics(c, top, v, lim)
	case BlockCode:
		out = checkCode(c, top, v)
	case BlockDiff:
		out = checkDiff(c, top, v)
	case BlockDiagram:
		out = checkDiagram(c, top, v)
	case BlockLinks:
		out = checkLinks(c, top, v, lim)
	case BlockChoices:
		out = checkChoices(c, top, v, lim)
	}
	if len(c.problems)+c.extra != n {
		return nil
	}
	return out
}

// need records that where holds the wrong kind of value.
func (c *checker) need(where, noun, shape string) {
	c.add(where+" needs "+noun, "pass "+shape)
}

// str checks a string field: type, emptiness and length in characters.
func (c *checker) str(where string, v any, max int, required bool) (string, bool) {
	s, ok := v.(string)
	if !ok {
		c.add(where+" must be a string", `pass a JSON string, e.g. "..."`)
		return "", false
	}
	if required && strings.TrimSpace(s) == "" {
		c.add(where+" is empty", "write some text there")
		return "", false
	}
	if n := utf8.RuneCountInString(s); n > max {
		c.add(fmt.Sprintf("%s is %d characters; the limit is %d", where, n, max),
			"shorten it, or put the full text in a file and show that")
		return "", false
	}
	return s, true
}

// obj checks that v is an object with no keys outside allowed.
func (c *checker) obj(where string, v any, noun, shape string, allowed ...string) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		c.need(where, noun, shape)
		return nil, false
	}
	good := true
	for _, k := range sortedKeys(m) {
		if !slices.Contains(allowed, k) {
			c.add(fmt.Sprintf("%s has unknown field %q", where, k), "use only: "+strings.Join(allowed, ", "))
			good = false
		}
	}
	return m, good
}

// arr checks that v is a non-empty array of at most max items.
func (c *checker) arr(where string, v any, noun, shape string, max int) ([]any, bool) {
	a, ok := v.([]any)
	if !ok {
		c.need(where, noun, shape)
		return nil, false
	}
	if len(a) == 0 {
		c.add(where+" is empty", "add at least one item, or leave the slot out")
		return nil, false
	}
	if len(a) > max {
		c.add(fmt.Sprintf("%s has %d items; the limit is %d", where, len(a), max),
			fmt.Sprintf("keep the %d most important", max))
		return nil, false
	}
	return a, true
}

// field returns a required field of m, recording a problem when it is missing.
func (c *checker) field(m map[string]any, where, key, shape string) (any, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		c.add(fmt.Sprintf("%s needs field %q", where, key), "pass "+shape)
		return nil, false
	}
	return v, true
}

// optStr copies an optional string field of m into out.
func (c *checker) optStr(m, out map[string]any, where, key string, max int) {
	v, ok := m[key]
	if !ok || v == nil {
		return
	}
	if s, ok := c.str(fmt.Sprintf("%s field %q", where, key), v, max, false); ok {
		out[key] = s
	}
}

func item(where string, i int) string { return fmt.Sprintf("%s item %d", where, i+1) }

func checkText(c *checker, where string, v any, lim int) any {
	if _, ok := v.(string); !ok {
		c.need(where, specs[BlockText].noun, specs[BlockText].shape)
		return nil
	}
	s, ok := c.str(where, v, lim, true)
	if !ok {
		return nil
	}
	return markdown(s)
}

func checkCallout(c *checker, where string, v any, lim int) any {
	shape := specs[BlockCallout].shape
	m, ok := c.obj(where, v, specs[BlockCallout].noun, shape, "tone", "text")
	if !ok {
		return nil
	}
	out := map[string]any{"tone": "info"}
	if t, present := m["tone"]; present && t != nil {
		if s, isStr := t.(string); !isStr || !slices.Contains(Tones, s) {
			c.add(fmt.Sprintf("%s has tone %s", where, compact(t)), "use one of: "+strings.Join(Tones, ", "))
		} else {
			out["tone"] = s
		}
	}
	if t, ok := c.field(m, where, "text", shape); ok {
		if s, ok := c.str(where+` field "text"`, t, lim, true); ok {
			out["text"] = markdown(s)
		}
	}
	return out
}

func checkChecklist(c *checker, where string, v any, lim int) any {
	shape := specs[BlockChecklist].shape
	a, ok := c.arr(where, v, specs[BlockChecklist].noun, shape, lim)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(a))
	for i, it := range a {
		w := item(where, i)
		m, ok := c.obj(w, it, "a checklist item", `{"text": "Write tests", "done": false}`, "text", "done")
		if !ok {
			continue
		}
		o := map[string]any{"done": false}
		if t, ok := c.field(m, w, "text", `{"text": "Write tests", "done": false}`); ok {
			if s, ok := c.str(w+` field "text"`, t, MaxTextChars, true); ok {
				o["text"] = markdown(s)
			}
		}
		if d, present := m["done"]; present && d != nil {
			if b, isBool := d.(bool); isBool {
				o["done"] = b
			} else {
				c.add(w+` field "done" must be true or false`, `pass "done": true or "done": false`)
			}
		}
		out = append(out, o)
	}
	return out
}

func checkList(c *checker, where string, v any, lim int) any {
	a, ok := c.arr(where, v, specs[BlockList].noun, specs[BlockList].shape, lim)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(a))
	for i, it := range a {
		if s, ok := c.str(item(where, i), it, MaxTextChars, true); ok {
			out = append(out, markdown(s))
		}
	}
	return out
}

func checkGallery(c *checker, where string, v any, lim int) any {
	shape := c.galleryShape()
	a, ok := c.arr(where, v, specs[BlockGallery].noun, shape, lim)
	if !ok {
		return nil
	}
	itemShape := strings.TrimSuffix(strings.TrimPrefix(shape, "["), "]")
	out := make([]any, 0, len(a))
	for i, it := range a {
		w := item(where, i)
		m, ok := c.obj(w, it, "an image with an optional caption", itemShape, "file", "caption")
		if !ok {
			continue
		}
		o := map[string]any{}
		if f, ok := c.field(m, w, "file", itemShape); ok {
			if t := c.fileTarget(w, f, BlockImage); t != nil {
				o["file"] = t
			}
		}
		c.optStr(m, o, w, "caption", MaxShortChars)
		out = append(out, o)
	}
	return out
}

func checkCompare(c *checker, where string, v any) any {
	shape := c.compareShape()
	m, ok := c.obj(where, v, specs[BlockCompare].noun, shape, "before", "after", "caption")
	if !ok {
		return nil
	}
	out := map[string]any{}
	for _, side := range []string{"before", "after"} {
		if f, ok := c.field(m, where, side, shape); ok {
			if fv := c.fileValue(fmt.Sprintf("%s field %q", where, side), f, BlockImage); fv != nil {
				out[side] = fv
			}
		}
	}
	c.optStr(m, out, where, "caption", MaxShortChars)
	return out
}

func checkChart(c *checker, where string, v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		c.need(where, specs[BlockChart].noun, specs[BlockChart].shape)
		return nil
	}
	if len(m) == 0 {
		c.add(where+" is an empty chart", "pass a Vega-Lite spec, e.g. "+specs[BlockChart].shape)
		return nil
	}
	return m
}

func checkTable(c *checker, where string, v any, lim int) any {
	shape := specs[BlockTable].shape
	m, ok := c.obj(where, v, specs[BlockTable].noun, shape, "columns", "rows")
	if !ok {
		return nil
	}
	out := map[string]any{}
	var ncols int
	if cv, ok := c.field(m, where, "columns", shape); ok {
		cw := where + ` field "columns"`
		if cols, ok := c.arr(cw, cv, "a list of column names", `["Name", "Value"]`, MaxTableColumns); ok {
			ncols = len(cols)
			names := make([]any, 0, len(cols))
			for i, col := range cols {
				if s, ok := c.str(item(cw, i), col, MaxShortChars, true); ok {
					names = append(names, s)
				}
			}
			out["columns"] = names
		}
	}
	rv, ok := c.field(m, where, "rows", shape)
	if !ok {
		return out
	}
	rw := where + ` field "rows"`
	rows, ok := rv.([]any)
	if !ok {
		c.need(rw, "a list of rows", `[["a", 1], ["b", 2]]`)
		return out
	}
	if len(rows) > lim {
		c.add(fmt.Sprintf("%s has %d rows; the limit is %d", where, len(rows), lim),
			fmt.Sprintf("keep the %d most important rows, or show the full table as a file", lim))
		return out
	}
	outRows := make([]any, 0, len(rows))
	for i, r := range rows {
		w := fmt.Sprintf("%s row %d", where, i+1)
		cells, ok := r.([]any)
		if !ok {
			c.need(w, "a list of cells", `["a", 1]`)
			continue
		}
		if ncols > 0 && len(cells) != ncols {
			c.add(fmt.Sprintf("%s has %d cells but there are %d columns", w, len(cells), ncols),
				"give every row one cell per column; use null for an empty cell")
			continue
		}
		for j, cell := range cells {
			switch x := cell.(type) {
			case nil, bool, json.Number:
			case string:
				c.str(fmt.Sprintf("%s cell %d", w, j+1), x, MaxCellChars, false)
			default:
				c.add(fmt.Sprintf("%s cell %d must be text, a number, true/false or null", w, j+1),
					"turn the cell into a string")
			}
		}
		outRows = append(outRows, cells)
	}
	out["rows"] = outRows
	return out
}

func checkMetrics(c *checker, where string, v any, lim int) any {
	shape := specs[BlockMetrics].shape
	a, ok := c.arr(where, v, specs[BlockMetrics].noun, shape, lim)
	if !ok {
		return nil
	}
	itemShape := `{"label": "p95 latency", "value": 120, "before": 340, "unit": "ms", "good": "down"}`
	out := make([]any, 0, len(a))
	for i, it := range a {
		w := item(where, i)
		m, ok := c.obj(w, it, "a key number", itemShape, "label", "value", "before", "unit", "delta", "good")
		if !ok {
			continue
		}
		o := map[string]any{}
		if l, ok := c.field(m, w, "label", itemShape); ok {
			if s, ok := c.str(w+` field "label"`, l, MaxShortChars, true); ok {
				o["label"] = s
			}
		}
		if val, ok := c.field(m, w, "value", itemShape); ok {
			if x, ok := c.numOrStr(w+` field "value"`, val); ok {
				o["value"] = x
			}
		}
		for _, k := range []string{"before", "delta"} {
			if val, present := m[k]; present && val != nil {
				if x, ok := c.numOrStr(fmt.Sprintf("%s field %q", w, k), val); ok {
					o[k] = x
				}
			}
		}
		c.optStr(m, o, w, "unit", MaxShortChars)
		if g, present := m["good"]; present && g != nil {
			if s, isStr := g.(string); isStr && (s == "up" || s == "down") {
				o["good"] = s
			} else {
				c.add(fmt.Sprintf(`%s field "good" is %s`, w, compact(g)),
					`use "up" when a higher number is better, "down" when lower is better`)
			}
		}
		out = append(out, o)
	}
	return out
}

// numOrStr accepts a JSON number or a short non-empty string such as "1.2 s".
func (c *checker) numOrStr(where string, v any) (any, bool) {
	switch x := v.(type) {
	case json.Number:
		return x, true
	case string:
		if strings.TrimSpace(x) != "" && utf8.RuneCountInString(x) <= MaxShortChars {
			return x, true
		}
	}
	c.add(where+" must be a number or a short string", `pass e.g. 120 or "1.2 s"`)
	return nil, false
}

func checkCode(c *checker, where string, v any) any {
	shape := specs[BlockCode].shape
	m, ok := c.obj(where, v, specs[BlockCode].noun, shape, "language", "code", "filename")
	if !ok {
		return nil
	}
	out := map[string]any{}
	if b, ok := c.field(m, where, "code", shape); ok {
		if s, ok := c.str(where+` field "code"`, b, MaxCodeChars, true); ok {
			out["code"] = s
		}
	}
	c.optStr(m, out, where, "language", MaxShortChars)
	c.optStr(m, out, where, "filename", MaxShortChars)
	return out
}

func checkDiff(c *checker, where string, v any) any {
	shape := specs[BlockDiff].shape
	m, ok := c.obj(where, v, specs[BlockDiff].noun, shape, "diff", "filename")
	if !ok {
		return nil
	}
	out := map[string]any{}
	if b, ok := c.field(m, where, "diff", shape); ok {
		if s, ok := c.str(where+` field "diff"`, b, MaxCodeChars, true); ok {
			if !looksLikeDiff(s) {
				c.add(where+` field "diff" is not a unified diff (no "@@" hunk line)`,
					"pass unified diff text, e.g. the output of git diff")
			} else {
				out["diff"] = s
			}
		}
	}
	c.optStr(m, out, where, "filename", MaxShortChars)
	return out
}

// looksLikeDiff reports whether s has at least one unified-diff hunk header.
func looksLikeDiff(s string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(line, "@@") {
			return true
		}
	}
	return false
}

func checkDiagram(c *checker, where string, v any) any {
	shape := specs[BlockDiagram].shape
	m, ok := c.obj(where, v, specs[BlockDiagram].noun, shape, "mermaid")
	if !ok {
		return nil
	}
	b, ok := c.field(m, where, "mermaid", shape)
	if !ok {
		return nil
	}
	s, ok := c.str(where+` field "mermaid"`, b, MaxMermaidChars, true)
	if !ok {
		return nil
	}
	return map[string]any{"mermaid": s}
}

func checkLinks(c *checker, where string, v any, lim int) any {
	shape := specs[BlockLinks].shape
	a, ok := c.arr(where, v, specs[BlockLinks].noun, shape, lim)
	if !ok {
		return nil
	}
	itemShape := `{"title": "Go spec", "url": "https://go.dev/ref/spec"}`
	out := make([]any, 0, len(a))
	for i, it := range a {
		w := item(where, i)
		m, ok := c.obj(w, it, "a link", itemShape, "title", "url")
		if !ok {
			continue
		}
		o := map[string]any{}
		if t, ok := c.field(m, w, "title", itemShape); ok {
			if s, ok := c.str(w+` field "title"`, t, MaxShortChars, true); ok {
				o["title"] = s
			}
		}
		if u, ok := c.field(m, w, "url", itemShape); ok {
			if s, ok := c.str(w+` field "url"`, u, MaxCellChars*2, true); ok {
				if p, err := url.Parse(s); err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
					c.add(fmt.Sprintf("%s has url %q, which is not an http or https address", w, s),
						`use a full address starting with https:// or http://`)
				} else {
					o["url"] = s
				}
			}
		}
		out = append(out, o)
	}
	return out
}

func checkChoices(c *checker, where string, v any, lim int) any {
	shape := specs[BlockChoices].shape
	a, ok := c.arr(where, v, specs[BlockChoices].noun, shape, lim)
	if !ok {
		return nil
	}
	if len(a) < MinChoices {
		c.add(fmt.Sprintf("%s has %d choice; at least %d are needed", where, len(a), MinChoices),
			"add another choice; a question needs something to choose between")
		return nil
	}
	itemShape := strings.TrimSuffix(strings.TrimPrefix(shape, "["), "]")
	seen := map[string]bool{}
	out := make([]any, 0, len(a))
	for i, it := range a {
		w := item(where, i)
		m, ok := c.obj(w, it, "a choice", itemShape, "title", "text", "pros", "cons")
		if !ok {
			continue
		}
		o := map[string]any{}
		if t, ok := c.field(m, w, "title", itemShape); ok {
			if s, ok := c.str(w+` field "title"`, t, MaxShortChars, true); ok {
				key := strings.ToLower(strings.TrimSpace(s))
				if seen[key] {
					c.add(fmt.Sprintf("%s has the same title as another choice: %q", w, s),
						"give each choice a different title")
				}
				seen[key] = true
				o["title"] = s
			}
		}
		if t, present := m["text"]; present && t != nil {
			if s, ok := c.str(w+` field "text"`, t, MaxTextChars, false); ok {
				o["text"] = markdown(s)
			}
		}
		for _, k := range []string{"pros", "cons"} {
			pv, present := m[k]
			if !present || pv == nil {
				continue
			}
			kw := fmt.Sprintf("%s field %q", w, k)
			if list, ok := c.arr(kw, pv, "a list of short points", `["fast", "simple"]`, 20); ok {
				pts := make([]any, 0, len(list))
				for j, p := range list {
					if s, ok := c.str(item(kw, j), p, MaxCellChars, true); ok {
						pts = append(pts, s)
					}
				}
				o[k] = pts
			}
		}
		out = append(out, o)
	}
	return out
}

// compact renders a JSON value briefly for error messages.
func compact(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // values here came from decodeJSON, so they always encode
	s := strings.TrimSpace(buf.String())
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
