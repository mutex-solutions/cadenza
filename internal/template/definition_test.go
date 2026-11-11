package template

import (
	"encoding/json"
	"strings"
	"testing"
)

// defJSON builds a definition with fields overridden by over.
func defJSON(t *testing.T, over map[string]any) json.RawMessage {
	t.Helper()
	d := map[string]any{
		"name":    "api-change",
		"use_for": "Any change to the public API",
		"guide":   "Fill **summary**.",
		"slots": []any{
			map[string]any{"name": "summary", "block": "text", "required": true},
			map[string]any{"name": "files_changed", "block": "list", "max": 5},
		},
		"example": map[string]any{"summary": "Added /v2/users", "files_changed": []any{"api.go"}},
	}
	for k, v := range over {
		if v == nil {
			delete(d, k)
		} else {
			d[k] = v
		}
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseDefinitionOK(t *testing.T) {
	d, err := ParseDefinition(defJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "api-change" || len(d.Slots) != 2 || d.Slots[1].Label != "Files changed" || d.Slots[0].Label != "Summary" {
		t.Fatalf("unexpected %+v", d)
	}
}

func TestParseDefinitionKeepsGivenLabel(t *testing.T) {
	d, err := ParseDefinition(defJSON(t, map[string]any{"slots": []any{
		map[string]any{"name": "summary", "block": "text", "required": true, "label": "TL;DR", "hint": "one line"},
		map[string]any{"name": "files_changed", "block": "list"},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Slots[0].Label != "TL;DR" || d.Slots[0].Hint != "one line" {
		t.Fatalf("label lost: %+v", d.Slots[0])
	}
}

func slots(s ...map[string]any) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

func TestParseDefinitionProblems(t *testing.T) {
	summary := map[string]any{"name": "summary", "block": "text", "required": true}
	ex := map[string]any{"summary": "x"}
	cases := []struct {
		name string
		over map[string]any
		want []string
	}{
		{"bad name", map[string]any{"name": "API_change"}, []string{`template name "API_change" is not allowed`, "lowercase letters, digits and dashes"}},
		{"short name", map[string]any{"name": "a"}, []string{`template name "a" is not allowed`}},
		{"long name", map[string]any{"name": strings.Repeat("a", 41)}, []string{"is not allowed"}},
		{"trailing dash", map[string]any{"name": "ab-"}, []string{"is not allowed"}},
		{"no use_for", map[string]any{"use_for": ""}, []string{"use_for must be one line", "say when to pick"}},
		{"no guide", map[string]any{"guide": " "}, []string{"guide must be markdown", "explain how to fill"}},
		{"no slots", map[string]any{"slots": []any{}}, []string{"has 0 slots; it needs 1 to 30"}},
		{"unknown block", map[string]any{"slots": slots(summary, map[string]any{"name": "s", "block": "slider"})},
			[]string{`slot 2 ("s") has unknown block "slider"`, "use one of: text, callout, checklist, list, image, gallery, compare, video, chart, table, metrics, code, diff, diagram, file, webpage, links, choices"}},
		{"duplicate slot", map[string]any{"slots": slots(summary, map[string]any{"name": "summary", "block": "list"})},
			[]string{`slot name "summary" is used twice`, "give each slot its own name"}},
		{"bad slot name", map[string]any{"slots": slots(summary, map[string]any{"name": "Bad-Name", "block": "list"})},
			[]string{`slot 2 ("Bad-Name") has a name that is not allowed`}},
		{"no required", map[string]any{"slots": slots(map[string]any{"name": "summary", "block": "text"})},
			[]string{"no required slot", `"required": true`}},
		{"negative max", map[string]any{"slots": slots(summary, map[string]any{"name": "l", "block": "list", "max": -1})}, []string{"negative max"}},
		{"max on image", map[string]any{"slots": slots(summary, map[string]any{"name": "i", "block": "image", "max": 2})},
			[]string{`max does not apply to block "image"`, "remove max"}},
		{"max too big", map[string]any{"slots": slots(summary, map[string]any{"name": "l", "block": "list", "max": 1000})},
			[]string{`max 1000; block "list" allows at most 100`, "set max to 100 or less"}},
		{"long label", map[string]any{"slots": slots(summary, map[string]any{"name": "l", "block": "list", "label": strings.Repeat("a", 301)})}, []string{"label that is too long"}},
		{"long hint", map[string]any{"slots": slots(summary, map[string]any{"name": "l", "block": "list", "hint": strings.Repeat("a", 301)})}, []string{"hint that is too long"}},
		{"no example", map[string]any{"example": nil}, []string{"has no example", `add "example"`}},
		{"null example", map[string]any{"example": json.RawMessage("null")}, []string{"has no example"}},
		{"bad example", map[string]any{"example": map[string]any{"summary": 1}}, []string{`example: slot "summary" needs markdown text`}},
		{"example unknown slot", map[string]any{"example": map[string]any{"summary": "a", "x": 1}}, []string{`example: template "api-change" has no slot "x"`}},
		{"unknown field", map[string]any{"color": "red"}, []string{"not valid", `unknown field "color"`, "pass a JSON object with name"}},
		{"question not choices", map[string]any{"slots": slots(summary), "example": ex, "question": map[string]any{"from_slot": "summary"}},
			[]string{`question.from_slot "summary" is not a choices slot`}},
		{"question bad text slot", map[string]any{
			"slots":    slots(summary, map[string]any{"name": "opts", "block": "choices"}),
			"example":  ex,
			"question": map[string]any{"from_slot": "opts", "text_slot": "opts", "text": strings.Repeat("a", 301)}},
			[]string{`question.text_slot "opts" is not a text slot`, "question.text is too long"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDefinition(defJSON(t, tc.over))
			e := kerr(t, err)
			if e.Fix == "" {
				t.Errorf("no fix: %v", e)
			}
			mustContain(t, e.Error(), tc.want...)
		})
	}
}

func TestParseDefinitionRawProblems(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
	}{
		"not json":  {`{`, "not valid"},
		"trailing":  {string(defJSON(t, nil)) + `{}`, "extra data"},
		"too large": {`{"guide": "` + strings.Repeat("a", MaxDataBytes) + `"}`, "larger than 256 KB"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDefinition(raw(tc.raw))
			mustContain(t, kerr(t, err).Error(), tc.want)
		})
	}
}

func TestParseDefinitionTooManySlots(t *testing.T) {
	var s []any
	for i := range MaxSlots + 1 {
		s = append(s, map[string]any{"name": "s" + strings.Repeat("a", i), "block": "text", "required": true})
	}
	_, err := ParseDefinition(defJSON(t, map[string]any{"slots": s}))
	mustContain(t, kerr(t, err).Error(), "has 31 slots")
}

func TestParseDefinitionQuestionOK(t *testing.T) {
	d, err := ParseDefinition(defJSON(t, map[string]any{
		"slots": slots(
			map[string]any{"name": "q", "block": "text", "required": true},
			map[string]any{"name": "opts", "block": "choices", "required": true}),
		"example":  map[string]any{"q": "Which?", "opts": []any{map[string]any{"title": "A"}, map[string]any{"title": "B"}}},
		"question": map[string]any{"from_slot": "opts", "text_slot": "q"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Ask == nil || d.Ask.FromSlot != "opts" {
		t.Fatalf("question lost: %+v", d.Ask)
	}
}

func TestLabelFor(t *testing.T) {
	for in, want := range map[string]string{"files_changed": "Files changed", "a": "A", "": ""} {
		if got := labelFor(in); got != want {
			t.Errorf("labelFor(%q) = %q, want %q", in, got, want)
		}
	}
}
