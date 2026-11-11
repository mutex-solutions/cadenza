package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Definition is a template: what it is for, how to fill it, and its slots.
// Built-in templates and custom ones (made by an agent or the owner) share
// this shape.
type Definition struct {
	Name    string `json:"name"`              // lowercase letters, digits and dashes, 2..40 chars
	Version int    `json:"version,omitempty"` // set by the store; built-ins are 1
	UseFor  string `json:"use_for"`           // one line: when to pick this template
	Guide   string `json:"guide"`             // markdown: how to fill it, with tips
	Slots   []Slot `json:"slots"`
	// Example is filled data that validates in Local mode.
	Example json.RawMessage `json:"example"`
	// Ask (JSON "question"), when set, makes a post with this template also ask the
	// owner to pick one of the choices in a choices slot.
	Ask *QuestionSpec `json:"question,omitempty"`
}

// Slot is one named place in a template, filled with one block.
type Slot struct {
	Name     string `json:"name"` // lowercase letters, digits and underscores
	Block    Block  `json:"block"`
	Required bool   `json:"required,omitempty"`
	Label    string `json:"label,omitempty"` // heading shown to the owner; defaults from Name
	Hint     string `json:"hint,omitempty"`  // one line for the agent: what goes here
	// Max lowers the block's limit: characters for text and callout, items
	// for list-like blocks, rows for table. 0 keeps the block's default.
	Max int `json:"max,omitempty"`
}

// QuestionSpec turns a choices slot into a question.
type QuestionSpec struct {
	FromSlot string `json:"from_slot"`           // a choices slot; its titles become the choices
	TextSlot string `json:"text_slot,omitempty"` // a text slot holding the question; optional
	Text     string `json:"text,omitempty"`      // question used when TextSlot is unset or empty
}

// DefaultQuestion is asked when a template names no question text.
const DefaultQuestion = "Which option should I go with?"

const (
	maxUseForChars = 200
	maxGuideChars  = MaxTextChars
	maxHintChars   = MaxShortChars
)

var (
	nameRE     = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	slotNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ParseDefinition decodes and checks a custom template. Every problem comes
// back in one kernel.Invalid error with exact fixes. Labels left empty are
// filled from slot names.
func ParseDefinition(raw json.RawMessage) (Definition, error) {
	c := &checker{mode: Local}
	var d Definition
	if len(raw) > MaxDataBytes {
		c.add(fmt.Sprintf("definition is larger than %d KB", MaxDataBytes>>10), "shorten the guide and the example")
		return Definition{}, c.err("template definition")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		c.add("template definition is not valid: "+err.Error(),
			"pass a JSON object with name, use_for, guide, slots (each with name and block) and example")
		return Definition{}, c.err("template definition")
	}
	if _, err := dec.Token(); err != io.EOF {
		c.add("template definition has extra data after the JSON object", "pass exactly one JSON object")
		return Definition{}, c.err("template definition")
	}
	d.fillLabels()
	d.checkFields(c)
	if !c.ok() {
		return Definition{}, c.err(fmt.Sprintf("template %q", d.Name))
	}
	if len(d.Example) == 0 || string(d.Example) == "null" {
		c.add("template has no example", "add \"example\": filled data that shows every slot, e.g. "+d.exampleOrShape())
	} else {
		ec, _ := d.check(d.Example, Local)
		for _, p := range ec.problems {
			c.add("example: "+p.msg, p.fix)
		}
		c.extra += ec.extra
	}
	if err := c.err(fmt.Sprintf("template %q", d.Name)); err != nil {
		return Definition{}, err
	}
	return d, nil
}

func (d *Definition) fillLabels() {
	for i := range d.Slots {
		if strings.TrimSpace(d.Slots[i].Label) == "" {
			d.Slots[i].Label = labelFor(d.Slots[i].Name)
		}
	}
}

// labelFor turns "files_changed" into "Files changed".
func labelFor(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// checkFields checks everything except the example.
func (d Definition) checkFields(c *checker) {
	if n := len(d.Name); n < 2 || n > 40 || !nameRE.MatchString(d.Name) {
		c.add(fmt.Sprintf("template name %q is not allowed", d.Name),
			`use 2 to 40 lowercase letters, digits and dashes, starting with a letter, e.g. "api-change"`)
	}
	if strings.TrimSpace(d.UseFor) == "" || utf8.RuneCountInString(d.UseFor) > maxUseForChars {
		c.add("use_for must be one line of 1 to 200 characters", `say when to pick this template, e.g. "Any change to the public API"`)
	}
	if strings.TrimSpace(d.Guide) == "" || utf8.RuneCountInString(d.Guide) > maxGuideChars {
		c.add(fmt.Sprintf("guide must be markdown of 1 to %d characters", maxGuideChars),
			"explain how to fill each slot, with a tip or two")
	}
	if len(d.Slots) == 0 || len(d.Slots) > MaxSlots {
		c.add(fmt.Sprintf("template has %d slots; it needs 1 to %d", len(d.Slots), MaxSlots),
			`add slots like {"name": "summary", "block": "text", "required": true}`)
	}
	seen := map[string]bool{}
	required := 0
	for i, s := range d.Slots {
		where := fmt.Sprintf("slot %d (%q)", i+1, s.Name)
		if len(s.Name) > 40 || !slotNameRE.MatchString(s.Name) {
			c.add(where+" has a name that is not allowed",
				`use up to 40 lowercase letters, digits and underscores, starting with a letter, e.g. "files_changed"`)
		}
		if seen[s.Name] {
			c.add(fmt.Sprintf("slot name %q is used twice", s.Name), "give each slot its own name")
		}
		seen[s.Name] = true
		if s.Required {
			required++
		}
		spec, ok := specs[s.Block]
		if !ok {
			c.add(fmt.Sprintf("%s has unknown block %q", where, s.Block), "use one of: "+blockNames())
		} else {
			switch {
			case s.Max < 0:
				c.add(where+" has a negative max", "remove max or set it to a positive number")
			case s.Max > 0 && spec.max == 0:
				c.add(fmt.Sprintf("%s: max does not apply to block %q", where, s.Block), "remove max")
			case s.Max > spec.max:
				c.add(fmt.Sprintf("%s has max %d; block %q allows at most %d", where, s.Max, s.Block, spec.max),
					fmt.Sprintf("set max to %d or less", spec.max))
			}
		}
		if utf8.RuneCountInString(s.Label) > MaxShortChars {
			c.add(where+" has a label that is too long", fmt.Sprintf("keep labels under %d characters", MaxShortChars))
		}
		if utf8.RuneCountInString(s.Hint) > maxHintChars {
			c.add(where+" has a hint that is too long", fmt.Sprintf("keep hints under %d characters", maxHintChars))
		}
	}
	if len(d.Slots) > 0 && required == 0 {
		c.add("template has no required slot", `mark the main slot "required": true so posts are never empty`)
	}
	if q := d.Ask; q != nil {
		if s, ok := d.slot(q.FromSlot); !ok || s.Block != BlockChoices {
			c.add(fmt.Sprintf("question.from_slot %q is not a choices slot", q.FromSlot),
				"set from_slot to the name of a slot with block \"choices\"")
		}
		if q.TextSlot != "" {
			if s, ok := d.slot(q.TextSlot); !ok || s.Block != BlockText {
				c.add(fmt.Sprintf("question.text_slot %q is not a text slot", q.TextSlot),
					"set text_slot to the name of a slot with block \"text\", or remove it")
			}
		}
		if utf8.RuneCountInString(q.Text) > MaxShortChars {
			c.add("question.text is too long", fmt.Sprintf("keep it under %d characters", MaxShortChars))
		}
	}
}

// slot finds a slot by name.
func (d Definition) slot(name string) (Slot, bool) {
	for _, s := range d.Slots {
		if s.Name == name {
			return s, true
		}
	}
	return Slot{}, false
}

// slotNames lists the slot names for fix texts.
func (d Definition) slotNames() string {
	names := make([]string, len(d.Slots))
	for i, s := range d.Slots {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}
