package template

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Renderer turns short markdown into sanitized HTML. It matches
// review.Renderer; the server passes its markdown adapter.
type Renderer interface {
	Render(markdown string) string
}

// View is the data the web and Android viewers draw. It is self-describing,
// so a viewer needs no template lookup:
//
//	{
//	  "template": "ui-change",
//	  "version": 1,
//	  "slots": [
//	    {"name": "summary", "block": "text", "label": "Summary",
//	     "value": {"md": "Promo field **below** total", "html": "<p>Promo field <strong>below</strong> total</p>"}},
//	    {"name": "after", "block": "image", "label": "After",
//	     "value": {"file": {"sha256": "…", "size": 1234, "type": "image/webp", "name": "after.webp"}}},
//	    {"name": "changes", "block": "list", "label": "Changes",
//	     "value": [{"md": "Promo field moved", "html": "<p>Promo field moved</p>"}]}
//	  ]
//	}
//
// Slots come in template order; empty slots are left out. Each value has the
// block's data shape (see the Block constants) with defaults filled in
// (callout "tone" is "info", checklist "done" is false), and every markdown
// field replaced by {"md": source, "html": sanitized HTML}. The markdown
// fields are: the text block value, callout "text", each list item,
// checklist item "text" and choices item "text". Everything else (captions,
// titles, labels, table cells, code) is plain text.
type View struct {
	Template string     `json:"template"`
	Version  int        `json:"version,omitempty"`
	Slots    []ViewSlot `json:"slots"`
}

// ViewSlot is one filled slot in a View.
type ViewSlot struct {
	Name  string `json:"name"`
	Block Block  `json:"block"`
	Label string `json:"label"`
	Value any    `json:"value"`
}

// Markdown is how a markdown field appears in a View.
type Markdown struct {
	MD   string `json:"md"`
	HTML string `json:"html"`
}

// Render validates stored data (Remote mode) and returns it as View JSON,
// with markdown turned into HTML by r.
func (d Definition) Render(data json.RawMessage, r Renderer) (json.RawMessage, error) {
	c, vals := d.check(data, Remote)
	if err := c.err(fmt.Sprintf("data for template %q", d.Name)); err != nil {
		return nil, err
	}
	v := View{Template: d.Name, Version: d.Version, Slots: []ViewSlot{}}
	for _, s := range d.Slots {
		val, ok := vals[s.Name]
		if !ok {
			continue
		}
		v.Slots = append(v.Slots, ViewSlot{Name: s.Name, Block: s.Block, Label: s.Label, Value: renderValue(val, r)})
	}
	out, err := encode(v)
	if err != nil {
		return nil, kernel.Internal(err, "template: encoding view")
	}
	return out, nil
}

// renderValue replaces markdown strings with Markdown, recursively.
func renderValue(v any, r Renderer) any {
	switch x := v.(type) {
	case markdown:
		return Markdown{MD: string(x), HTML: r.Render(string(x))}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = renderValue(val, r)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = renderValue(val, r)
		}
		return out
	}
	return v
}

// encode marshals v without escaping <, > and &, which code and diffs use a lot.
func encode(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
