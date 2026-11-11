package template

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// Mode says which form file values must take.
type Mode int

const (
	// Local is the CLI side, before upload: a file is a local path,
	// {"file": "./shots/after.png"}. Stored references are refused.
	Local Mode = iota + 1
	// Remote is the server side: a file is a stored reference,
	// {"file": {"sha256": "...", "size": 1234, "type": "image/webp", "name": "after.webp"}},
	// and its type must suit the block.
	Remote
	// modeAny accepts both forms. Question uses it because it only reads text.
	modeAny
)

// maxProblems caps how many problems one error lists.
const maxProblems = 10

// Result is what Validate found besides problems.
type Result struct {
	// Paths lists every local file path in the data, in slot order (Local mode).
	// A path used twice appears twice.
	Paths []LocalFile
	// Refs lists every stored file reference in the data, in slot order (Remote mode).
	Refs []media.FileRef
}

// LocalFile is one local path found in Local mode, with what it must become.
type LocalFile struct {
	Path  string
	Slot  string
	Block Block
	// Want is the kind the stored file must be: media.KindImage (convert to
	// WebP), media.KindVideo (MP4 H.264), media.KindWebpage (one HTML file),
	// or "" when any allowed type will do.
	Want media.Kind
}

// Validate checks filled data against d. It returns every file reference it
// found, or one kernel.Invalid error listing up to ten problems, each with
// its exact fix.
func (d Definition) Validate(data json.RawMessage, mode Mode) (Result, error) {
	if mode != Local && mode != Remote {
		return Result{}, kernel.Internal(errors.New("bad mode"), "template: unknown validation mode %d", mode)
	}
	c, _ := d.check(data, mode)
	if err := c.err(fmt.Sprintf("data for template %q", d.Name)); err != nil {
		return Result{}, err
	}
	return c.res, nil
}

// check validates data and returns the checker plus the normalized value of
// each filled slot, keyed by slot name. Normalized values have defaults
// filled in and markdown strings typed as markdown, ready for Render.
func (d Definition) check(data json.RawMessage, mode Mode) (*checker, map[string]any) {
	c := &checker{mode: mode}
	if len(data) > MaxDataBytes {
		c.add(fmt.Sprintf("data is %s; the limit is %s", media.HumanSize(int64(len(data))), media.HumanSize(MaxDataBytes)),
			"make text shorter or move large content into a file")
		return c, nil
	}
	v, err := decodeJSON(data)
	if err != nil {
		c.add("data is not valid JSON: "+err.Error(), "pass one JSON object, e.g. "+d.exampleOrShape())
		return c, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		c.add("data must be a JSON object keyed by slot name", "pass one JSON object, e.g. "+d.exampleOrShape())
		return c, nil
	}
	for _, key := range sortedKeys(obj) {
		if _, ok := d.slot(key); !ok {
			c.add(fmt.Sprintf("template %q has no slot %q", d.Name, key), "use one of: "+d.slotNames())
		}
	}
	out := map[string]any{}
	for _, s := range d.Slots {
		val, present := obj[s.Name]
		if !present || val == nil {
			if s.Required {
				c.add(fmt.Sprintf("slot %q is required", s.Name),
					fmt.Sprintf("add %q: %s", s.Name, specs[s.Block].shape))
			}
			continue
		}
		c.slot, c.block = s.Name, s.Block
		if n := checkBlock(c, s, val); n != nil {
			out[s.Name] = n
		}
	}
	return c, out
}

// exampleOrShape is a short hint of what valid data looks like.
func (d Definition) exampleOrShape() string {
	if len(d.Example) > 0 && len(d.Example) < 300 {
		return string(d.Example)
	}
	if len(d.Slots) > 0 {
		return fmt.Sprintf(`{"%s": %s}`, d.Slots[0].Name, specs[d.Slots[0].Block].shape)
	}
	return "{}"
}

// decodeJSON decodes exactly one JSON value, keeping numbers exact.
func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON value")
	}
	return v, nil
}

// problem is one thing wrong with the data, with its exact fix.
type problem struct{ msg, fix string }

// checker collects problems and file references while walking data.
type checker struct {
	mode     Mode
	problems []problem
	extra    int // problems beyond maxProblems, counted but not listed
	res      Result
	slot     string // the slot being checked
	block    Block  // its block
}

func (c *checker) add(msg, fix string) {
	if len(c.problems) >= maxProblems {
		c.extra++
		return
	}
	c.problems = append(c.problems, problem{msg, fix})
}

func (c *checker) ok() bool { return len(c.problems) == 0 }

// err turns the problems into one kernel.Invalid error, or nil. One problem
// keeps its own message and fix; several are numbered in both.
func (c *checker) err(subject string) error {
	switch len(c.problems) {
	case 0:
		return nil
	case 1:
		return kernel.Invalid("%s", c.problems[0].msg).WithFix("%s", c.problems[0].fix)
	}
	total := len(c.problems) + c.extra
	msgs := make([]string, len(c.problems))
	fixes := make([]string, len(c.problems))
	for i, p := range c.problems {
		msgs[i] = fmt.Sprintf("(%d) %s", i+1, p.msg)
		fixes[i] = fmt.Sprintf("(%d) %s", i+1, p.fix)
	}
	msg := fmt.Sprintf("%s has %d problems: %s", subject, total, strings.Join(msgs, "; "))
	if c.extra > 0 {
		msg += fmt.Sprintf("; and %d more", c.extra)
	}
	return kernel.Invalid("%s", msg).WithFix("%s", strings.Join(fixes, "; "))
}
