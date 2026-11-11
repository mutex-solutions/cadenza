package markdown

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// External links open in a new tab and pass no referrer or ranking.
const (
	externalRel    = "noopener noreferrer nofollow"
	externalTarget = "_blank"
)

// linkPolicy removes links and images whose target is not allowed and marks
// external links. A removed link keeps its text; a removed image becomes its
// alt text.
type linkPolicy struct{}

func (linkPolicy) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var links []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Link, *ast.AutoLink, *ast.Image:
			links = append(links, n)
		}
		return ast.WalkContinue, nil
	})
	for _, n := range links {
		switch n := n.(type) {
		case *ast.Link:
			apply(n, string(n.Destination), func() { unwrap(n) })
		case *ast.Image:
			if safe, _ := classify(string(n.Destination)); !safe {
				replace(n, ast.NewString(textOf(n, src)))
			}
		case *ast.AutoLink:
			apply(n, string(n.URL(src)), func() { replace(n, ast.NewString(n.Label(src))) })
		}
	}
}

// apply removes n if dest is unsafe, or marks it external.
func apply(n ast.Node, dest string, remove func()) {
	safe, external := classify(dest)
	switch {
	case !safe:
		remove()
	case external:
		n.SetAttributeString("rel", []byte(externalRel))
		n.SetAttributeString("target", []byte(externalTarget))
	}
}

// classify reports whether dest is an allowed link target and whether it
// leaves the site. Allowed: http, https, mailto, relative paths (including
// protocol-relative //host, treated as external) and #anchors.
func classify(dest string) (safe, external bool) {
	if strings.ContainsFunc(dest, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false, false
	}
	u, err := url.Parse(strings.TrimSpace(dest))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true, true
	case "mailto":
		return true, false
	case "":
		return true, u.Host != ""
	default:
		return false, false
	}
}

// unwrap replaces n with its children.
func unwrap(n ast.Node) {
	parent := n.Parent()
	for c := n.FirstChild(); c != nil; {
		next := c.NextSibling()
		parent.InsertBefore(parent, n, c)
		c = next
	}
	parent.RemoveChild(parent, n)
}

// replace swaps n for r in n's parent.
func replace(n, r ast.Node) {
	n.Parent().ReplaceChild(n.Parent(), n, r)
}

// textOf is the plain text inside n, e.g. an image's alt text.
func textOf(n ast.Node, src []byte) []byte {
	var b []byte
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b = append(b, c.Segment.Value(src)...)
		case *ast.String:
			b = append(b, c.Value...)
		}
		return ast.WalkContinue, nil
	})
	return b
}
