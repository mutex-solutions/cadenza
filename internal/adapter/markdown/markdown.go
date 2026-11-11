// Package markdown renders short markdown from agents and owners into HTML
// that is safe to put in a page. It implements review.Renderer with goldmark.
//
// Safety comes from three things: raw HTML in the input is never rendered
// (goldmark's unsafe mode stays off), links and images keep only http,
// https, mailto, relative and #anchor targets, and input is capped at
// MaxInput characters.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"

	"github.com/mutex-solutions/cadenza/internal/review"
)

// MaxInput is the most characters (runes) rendered; the rest is cut off.
const MaxInput = 50_000

var _ review.Renderer = (*Renderer)(nil)

// Renderer turns markdown into sanitized HTML. It is safe for concurrent use.
type Renderer struct {
	md goldmark.Markdown
}

// New returns a Renderer with GitHub-flavored tables, strikethrough,
// autolinks and task lists.
func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(linkPolicy{}, 1000)),
		),
		// Unsafe is deliberately not enabled: raw HTML is dropped.
	)
	return &Renderer{md: md}
}

// Render returns sanitized HTML for markdown. Input longer than MaxInput
// characters is truncated first. It never fails; on an internal error it
// returns an empty string.
func (r *Renderer) Render(markdown string) string {
	src := []byte(truncate(markdown, MaxInput))
	var buf bytes.Buffer
	if err := r.md.Convert(src, &buf); err != nil {
		return ""
	}
	return buf.String()
}

// truncate cuts s to at most n runes, never splitting a rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}
