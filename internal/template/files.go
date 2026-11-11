package template

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// A file value is an object with a "file" key. In Local mode the key holds a
// path: {"file": "./shots/after.png"}. In Remote mode it holds a stored
// reference: {"file": {"sha256": "...", "size": 1234, "type": "image/webp", "name": "after.webp"}}.

// wantType is the stored content type a file block needs, or "" for any.
func wantType(k media.Kind) string {
	switch k {
	case media.KindImage:
		return media.TypeWebP
	case media.KindVideo:
		return media.TypeMP4
	case media.KindWebpage:
		return media.TypeHTML
	}
	return ""
}

// target is the "file" part of a file value in the current mode.
func (c *checker) target(want media.Kind) string {
	if c.mode == Remote {
		typ := wantType(want)
		if typ == "" {
			typ = "<type>"
		}
		return fmt.Sprintf(`{"sha256": "<hex>", "size": <bytes>, "type": "%s", "name": "<name>"}`, typ)
	}
	return `"<path>"`
}

func (c *checker) fileShape(want media.Kind) string {
	return fmt.Sprintf(`{"file": %s}`, c.target(want))
}

func (c *checker) galleryShape() string {
	return fmt.Sprintf(`[{"file": %s, "caption": "Home screen"}]`, c.target(media.KindImage))
}

func (c *checker) compareShape() string {
	f := c.fileShape(media.KindImage)
	return fmt.Sprintf(`{"before": %s, "after": %s, "caption": "..."}`, f, f)
}

// fileValue checks {"file": ...} for block b (image, video, webpage, file)
// and returns the normalized {"file": target}.
func (c *checker) fileValue(where string, v any, b Block) any {
	want := specs[b].want
	shape := c.fileShape(want)
	m, ok := c.obj(where, v, specs[b].noun, shape, "file")
	if !ok {
		return nil
	}
	f, ok := c.field(m, where, "file", shape)
	if !ok {
		return nil
	}
	t := c.fileTarget(where, f, b)
	if t == nil {
		return nil
	}
	return map[string]any{"file": t}
}

// fileTarget checks the inside of a file value: a path or a stored reference,
// depending on the mode. It records what it finds in c.res.
func (c *checker) fileTarget(where string, v any, b Block) any {
	want := specs[b].want
	switch x := v.(type) {
	case string:
		if c.mode == Remote {
			c.add(where+" is a local path; the server needs a stored file",
				"upload the file first (the cadenza CLI does this) and pass "+c.fileShape(want))
			return nil
		}
		return c.localPath(where, x, b)
	case map[string]any:
		if c.mode == Local {
			c.add(where+" holds a stored file reference; here a local path is expected",
				`pass {"file": "<path>"}`)
			return nil
		}
		return c.storedRef(where, x, b)
	}
	c.need(where, specs[b].noun, c.fileShape(want))
	return nil
}

func (c *checker) localPath(where, p string, b Block) any {
	switch {
	case strings.TrimSpace(p) == "":
		c.add(where+" has an empty file path", `pass {"file": "<path>"}`)
	case strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://"):
		c.add(fmt.Sprintf("%s has a URL, not a file: %q", where, p), "download it and pass the local path")
	case strings.ContainsRune(p, 0) || len(p) > MaxPathChars:
		c.add(where+" has a file path that is not allowed", "pass a normal path to a local file")
	default:
		c.res.Paths = append(c.res.Paths, LocalFile{Path: p, Slot: c.slot, Block: b, Want: specs[b].want})
		return p
	}
	return nil
}

func (c *checker) storedRef(where string, m map[string]any, b Block) any {
	want := specs[b].want
	raw, _ := json.Marshal(m) // m came from decodeJSON, so it always encodes
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var ref media.FileRef
	if err := dec.Decode(&ref); err != nil {
		c.add(where+" has a bad file reference", "pass "+c.fileShape(want))
		return nil
	}
	if err := ref.Validate(); err != nil {
		var ke *kernel.Error
		fix := "upload the file again and pass the reference the server returned"
		msg := err.Error()
		if errors.As(err, &ke) {
			msg = ke.Msg
			if ke.Fix != "" {
				fix = ke.Fix
			}
		}
		c.add(where+": "+msg, fix)
		return nil
	}
	if typ := wantType(want); typ != "" && ref.Type != typ {
		c.add(fmt.Sprintf("%s needs %s (%s), but file %q is %s", where, specs[b].noun, typ, ref.Name, ref.Type),
			convertFix(want))
		return nil
	}
	c.res.Refs = append(c.res.Refs, ref)
	return ref
}

func convertFix(want media.Kind) string {
	switch want {
	case media.KindImage:
		return "convert it to WebP (the cadenza CLI does this for PNG, JPEG and GIF) and upload again"
	case media.KindVideo:
		return "convert it to MP4 (H.264) and upload again; the cadenza CLI does this when ffmpeg is installed"
	default:
		return "pass one self-contained HTML file"
	}
}

// ReplacePaths swaps local paths for stored references after upload: every
// {"file": "<path>"} whose path is a key of refs becomes
// {"file": {<ref>}}. Paths not in refs are left alone, so a later Remote
// validation reports them. Numbers keep their exact text; object keys come
// out sorted.
func ReplacePaths(data json.RawMessage, refs map[string]media.FileRef) (json.RawMessage, error) {
	v, err := decodeJSON(data)
	if err != nil {
		return nil, kernel.Invalid("data is not valid JSON: %s", err.Error()).WithFix("pass one JSON object keyed by slot name")
	}
	out, err := encode(replace(v, refs))
	if err != nil {
		return nil, kernel.Internal(err, "template: encoding data")
	}
	return out, nil
}

func replace(v any, refs map[string]media.FileRef) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if p, ok := val.(string); ok && k == "file" {
				if ref, ok := refs[p]; ok {
					x[k] = ref
				}
				continue
			}
			x[k] = replace(val, refs)
		}
	case []any:
		for i, val := range x {
			x[i] = replace(val, refs)
		}
	}
	return v
}
