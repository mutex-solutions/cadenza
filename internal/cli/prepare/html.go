package prepare

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// Cadenza shows a page in a sandbox with no network and no other files, so
// a page must be one self-contained file. These patterns find the places a
// page loads other files from: src/href attributes in markup and url(...)
// in CSS. Script bodies are skipped so JavaScript strings are not mistaken
// for attributes.
var (
	reComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reScript    = regexp.MustCompile(`(?is)(<script\b[^>]*>).*?</script\s*>`)
	reAttr      = regexp.MustCompile(`(?i)[\s"'](?:src|href|poster|data)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	reCSSURL    = regexp.MustCompile(`(?i)url\(\s*("[^"]*"|'[^']*'|[^)\s]+)\s*\)`)
	reCSSImport = regexp.MustCompile(`(?i)@import\s+("[^"]*"|'[^']*')`)
	reScheme    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// pageRefs are the resources a page refers to, split by what happens to them.
type pageRefs struct {
	local    []string // relative paths: refused
	external []string // http(s) and protocol-relative URLs: warned
}

// findRefs lists the local and external resources an HTML page loads.
func findRefs(html string) pageRefs {
	s := reComment.ReplaceAllString(html, "")
	s = reScript.ReplaceAllString(s, "$1</script>")
	var vals []string
	for _, re := range []*regexp.Regexp{reAttr, reCSSURL, reCSSImport} {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			vals = append(vals, strings.Trim(m[1], `"'`))
		}
	}
	var r pageRefs
	seen := map[string]bool{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		lower := strings.ToLower(v)
		switch {
		case strings.HasPrefix(v, "#"), strings.Contains(v, "{{"), strings.HasPrefix(v, "${"):
			// anchors and template placeholders
		case strings.HasPrefix(lower, "http:"), strings.HasPrefix(lower, "https:"), strings.HasPrefix(v, "//"):
			r.external = append(r.external, v)
		case strings.HasPrefix(lower, "file:"):
			r.local = append(r.local, v)
		case reScheme.MatchString(v):
			// data:, blob:, mailto:, javascript:, tel: and the like
		default:
			r.local = append(r.local, v)
		}
	}
	return r
}

// prepareHTML checks that a page is one self-contained file within the
// size limit. Local references are refused with the exact fix; external
// URLs are allowed but warned about. Secret-looking text is redacted.
func prepareHTML(path string, size, limit int64) (*Prepared, error) {
	name := filepath.Base(path)
	if size > limit {
		return nil, tooBig(path, size, limit, "remove large inlined images or data from the page, or show them as separate files")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, kernel.Internal(err, "cannot read %s", path)
	}
	text := string(b)
	refs := findRefs(text)
	if len(refs.local) > 0 {
		shown := refs.local
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return nil, kernel.Invalid("%s loads local files (%s) that will not exist in the viewer", name, strings.Join(shown, ", ")).
			WithFix("inline %s into the HTML file; cadenza shows one self-contained file", strings.Join(shown, ", "))
	}
	p := &Prepared{Path: path}
	for _, u := range refs.external {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%s loads %s; it will not load in the viewer (no network)", name, u))
	}
	if found := ScanText(text); len(found) > 0 {
		p.Data = []byte(Redact(text))
		p.Path = ""
		p.Warnings = append(p.Warnings, redactedWarning(name, found))
	}
	var sum string
	var n int64
	if p.Data != nil {
		sum, n = hashBytes(p.Data), int64(len(p.Data))
	} else if sum, n, err = HashFile(path); err != nil {
		return nil, err
	}
	p.File = media.FileRef{SHA256: sum, Size: n, Type: media.TypeHTML, Name: name}
	if err := p.File.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}
