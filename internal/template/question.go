package template

import (
	"encoding/json"
	"strings"
)

// Question returns the question a post with this template asks: its text
// and the titles of the choices in the question's choices slot. ok is false
// when the template asks nothing (no Ask spec) or the data is not
// valid. File values may be paths or stored references.
func (d Definition) Question(data json.RawMessage) (text string, choices []string, ok bool) {
	q := d.Ask
	if q == nil {
		return "", nil, false
	}
	c, vals := d.check(data, modeAny)
	if !c.ok() {
		return "", nil, false
	}
	items, _ := vals[q.FromSlot].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if t, _ := m["title"].(string); t != "" {
			choices = append(choices, t)
		}
	}
	if len(choices) == 0 {
		return "", nil, false
	}
	if q.TextSlot != "" {
		if md, isMD := vals[q.TextSlot].(markdown); isMD {
			text = strings.TrimSpace(string(md))
		}
	}
	if text == "" {
		text = q.Text
	}
	if text == "" {
		text = DefaultQuestion
	}
	return text, choices, true
}
