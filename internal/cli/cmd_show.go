package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/cli/prepare"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/template"
)

func cmdShow(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"title", "summary", "template", "data", "thread", "notify-title", "retry-id"}, []string{"new"})
	if err != nil {
		return err
	}
	c, err := a.agentClient(ctx, f)
	if err != nil {
		return err
	}
	in, warnings, cleanup, err := a.buildShow(ctx, c, f)
	defer cleanup()
	if err != nil {
		return err
	}
	var out spec.ShowOutput
	if err := c.call(ctx, "show", nil, nil, in, &out); err != nil {
		return err
	}
	return a.print(withWarnings(out, warnings))
}

// buildShow turns flags and files into a ShowInput, converting and uploading
// every file first.
func (a *App) buildShow(ctx context.Context, c *client, f *flags) (spec.ShowInput, []string, func(), error) {
	noop := func() {}
	in := spec.ShowInput{Title: prepare.Redact(f.str("title")), Summary: prepare.Redact(f.str("summary")), ThreadID: f.str("thread"),
		NewThread: f.bool("new"), Notify: f.str("notify-title")}
	if in.Title == "" {
		return in, nil, noop, kernel.Invalid("--title is required").WithFix(`pass --title "what this is"`)
	}
	var paths []string
	var tmplData json.RawMessage
	var def template.Definition
	name := f.str("template")
	if name != "" {
		raw, err := a.readData(f.str("data"))
		if err != nil {
			return in, nil, noop, err
		}
		if def, err = c.definition(ctx, name); err != nil {
			return in, nil, noop, err
		}
		res, err := def.Validate(raw, template.Local)
		if err != nil {
			return in, nil, noop, err
		}
		for _, lf := range res.Paths {
			paths = append(paths, lf.Path)
		}
		tmplData = raw
	} else if f.str("data") != "" {
		return in, nil, noop, kernel.Invalid("--data needs --template").WithFix("pass --template <name>; see `cadenza templates`")
	}
	filePaths := f.pos
	if name == "" && len(filePaths) == 0 {
		return in, nil, noop, kernel.Invalid("nothing to show").WithFix("pass --template with --data, or one or more files")
	}
	files, warnings, cleanup, err := a.prepareFiles(ctx, append(append([]string{}, paths...), filePaths...))
	if err != nil {
		return in, nil, noop, err
	}
	byPath := map[string]prepared{}
	for _, p := range files {
		byPath[p.path] = p
	}
	if err := c.upload(ctx, files); err != nil {
		return in, nil, cleanup, err
	}
	var hashes []string
	if name != "" {
		refs := map[string]media.FileRef{}
		for _, p := range paths {
			pf := byPath[p]
			if pf.Inline {
				return in, nil, cleanup, kernel.Invalid("%s is text, but a template slot needs a file", p).
					WithFix("put text straight into the slot's JSON instead of a file path")
			}
			ref := pf.File
			if pf.Preview != nil {
				ref.PreviewSHA256 = pf.Preview.File.SHA256
			}
			refs[p] = ref
			hashes = append(hashes, ref.SHA256)
		}
		data, err := template.ReplacePaths(tmplData, refs)
		if err != nil {
			return in, nil, cleanup, err
		}
		data = redactJSON(data)
		in.Attachments = append(in.Attachments, spec.Attachment{Template: name, Version: def.Version, Data: data})
	}
	for _, p := range filePaths {
		pf := byPath[p]
		switch {
		case pf.Inline && pf.Source == media.SourceText:
			if in.Summary != "" {
				in.Summary += "\n\n"
			}
			in.Summary += pf.Text
		case pf.Inline:
			return in, nil, cleanup, kernel.Invalid("%s is a %s source, which needs a template", p, pf.Source).
				WithFix("put it in a template slot: diagrams use {\"mermaid\": \"…\"}, charts take the Vega-Lite spec; e.g. the plan or data-report template")
		default:
			ref := pf.File
			if pf.Preview != nil {
				ref.PreviewSHA256 = pf.Preview.File.SHA256
			}
			in.Attachments = append(in.Attachments, spec.Attachment{File: &ref})
			hashes = append(hashes, ref.SHA256)
		}
	}
	if len([]rune(in.Summary)) > review.MaxSummary {
		return in, nil, cleanup, kernel.Invalid("the summary (with text files) is longer than %d characters", review.MaxSummary).
			WithFix("shorten it, or show the text as a file such as a PDF or HTML page")
	}
	if len(in.Attachments) == 0 {
		return in, nil, cleanup, kernel.Invalid("nothing to show besides text").WithFix("add a template or a file; text files become the summary")
	}
	in.RetryID = f.str("retry-id")
	if in.RetryID == "" {
		in.RetryID = retryID(c.sess.ID, in.Title, in.Summary, in.ThreadID, in.NewThread, name, string(tmplData), hashes)
	}
	return in, warnings, cleanup, nil
}

// definition finds a template: built-ins locally, custom ones from the server.
func (c *client) definition(ctx context.Context, name string) (template.Definition, error) {
	if d, ok := template.Builtin(name); ok {
		return d, nil
	}
	var info spec.TemplateInfo
	if err := c.call(ctx, "template", map[string]string{"name": name}, nil, nil, &info); err != nil {
		return template.Definition{}, err
	}
	d, err := template.ParseDefinition(info.Definition)
	if err != nil {
		return template.Definition{}, err
	}
	d.Version = info.Version
	return d, nil
}

// readData reads --data: inline JSON, @file, or - for stdin.
func (a *App) readData(v string) (json.RawMessage, error) {
	var b []byte
	var err error
	switch {
	case v == "":
		return nil, kernel.Invalid("--template needs --data").WithFix(`pass --data '{"slot": …}' or --data @file.json; see cadenza templates <name>`)
	case v == "-":
		b, err = io.ReadAll(io.LimitReader(a.Stdin, 1<<20))
	case strings.HasPrefix(v, "@"):
		b, err = os.ReadFile(a.abs(v[1:]))
	default:
		b = []byte(v)
	}
	if err != nil {
		return nil, kernel.Invalid("reading --data: %v", err)
	}
	if !json.Valid(b) {
		return nil, kernel.Invalid("--data is not valid JSON").WithFix("check quotes and commas; pass a file with --data @file.json")
	}
	return json.RawMessage(b), nil
}

// redactJSON hides secret-looking text inside template data, keeping it valid JSON.
func redactJSON(data json.RawMessage) json.RawMessage {
	r := json.RawMessage(prepare.Redact(string(data)))
	if json.Valid(r) {
		return r
	}
	return data
}

// retryID makes the same call produce the same id, so retrying is safe.
func retryID(parts ...any) string {
	b, _ := json.Marshal(parts)
	h := sha256.Sum256(b)
	return "cli-" + hex.EncodeToString(h[:12])
}

func withWarnings(v any, warnings []string) any {
	if len(warnings) == 0 {
		return v
	}
	b, _ := json.Marshal(v)
	var m map[string]any
	json.Unmarshal(b, &m)
	m["warnings"] = warnings
	return m
}
