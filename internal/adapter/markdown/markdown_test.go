package markdown

import (
	"strings"
	"testing"
)

func render(t *testing.T, in string) string {
	t.Helper()
	return New().Render(in)
}

func TestXSSVectors(t *testing.T) {
	cases := map[string]string{
		"script tag":        "<script>alert(1)</script>",
		"img onerror":       `<img src=x onerror=alert(1)>`,
		"inline html":       `hi <b onmouseover=alert(1)>x</b>`,
		"javascript link":   `[click](javascript:alert(1))`,
		"JaVaScRiPt link":   `[click](JaVaScRiPt:alert(1))`,
		"entity javascript": `[click](&#106;avascript:alert(1))`,
		"hex entity":        `[click](&#x6A;avascript&#58;alert(1))`,
		"autolink js":       `<javascript:alert(1)>`,
		"vbscript":          `[x](vbscript:msgbox(1))`,
		"data link":         `[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)`,
		"data image":        `![x](data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)`,
		"js image":          `![x](javascript:alert(1))`,
		"file link":         `[x](file:///etc/passwd)`,
		"ref link js":       "[x][r]\n\n[r]: javascript:alert(1)",
		"tab in scheme":     "[x](java\tscript:alert(1))",
		"title break-out":   `[x](https://a.example "\" onclick=\"alert(1)")`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			out := strings.ToLower(render(t, in))
			for _, bad := range []string{
				"<script", `href="javascript`, `href="vbscript`, `href="data`, `href="file`,
				`src="javascript`, `src="data`, "onerror=", "onmouseover=", "<img src=x", `" onclick`,
			} {
				if strings.Contains(out, bad) {
					t.Fatalf("output contains %q:\n%s", bad, out)
				}
			}
		})
	}
}

func TestUnsafeLinkKeepsText(t *testing.T) {
	out := render(t, `[click me](javascript:alert(1))`)
	if !strings.Contains(out, "click me") || strings.Contains(out, "<a") {
		t.Fatalf("got %s", out)
	}
	out = render(t, `![the alt](data:image/png;base64,AAAA)`)
	if !strings.Contains(out, "the alt") || strings.Contains(out, "<img") {
		t.Fatalf("got %s", out)
	}
	out = render(t, `<javascript:alert(1)>`)
	if strings.Contains(out, "<a") {
		t.Fatalf("got %s", out)
	}
}

func TestLinkAttributes(t *testing.T) {
	cases := []struct {
		in, href string
		external bool
	}{
		{`[a](https://example.com/x)`, `href="https://example.com/x"`, true},
		{`[a](http://example.com)`, `href="http://example.com"`, true},
		{`[a](//example.com/p)`, `href="//example.com/p"`, true},
		{`<https://example.com>`, `href="https://example.com"`, true},
		{`see www.example.com now`, `href="http://www.example.com"`, true},
		{`[a](mailto:me@example.com)`, `href="mailto:me@example.com"`, false},
		{`mail me@example.com`, `href="mailto:me@example.com"`, false},
		{`[a](/t/t_81)`, `href="/t/t_81"`, false},
		{`[a](../x)`, `href="../x"`, false},
		{`[a](#section)`, `href="#section"`, false},
	}
	for _, c := range cases {
		out := render(t, c.in)
		if !strings.Contains(out, c.href) {
			t.Errorf("%s: want %s in %s", c.in, c.href, out)
		}
		hasAttrs := strings.Contains(out, `rel="noopener noreferrer nofollow"`) && strings.Contains(out, `target="_blank"`)
		if hasAttrs != c.external {
			t.Errorf("%s: external attrs = %v, want %v: %s", c.in, hasAttrs, c.external, out)
		}
	}
}

func TestSafeImage(t *testing.T) {
	out := render(t, `![cat](https://example.com/cat.webp)`)
	if !strings.Contains(out, `<img src="https://example.com/cat.webp" alt="cat"`) {
		t.Fatalf("got %s", out)
	}
}

func TestTable(t *testing.T) {
	out := render(t, "| a | b |\n|---|---|\n| 1 | 2 |\n")
	for _, want := range []string{"<table>", "<th>a</th>", "<td>2</td>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %s in %s", want, out)
		}
	}
}

func TestTaskListAndStrikethrough(t *testing.T) {
	out := render(t, "- [x] done\n- [ ] todo\n\n~~old~~")
	if !strings.Contains(out, `checked="" disabled="" type="checkbox"`) {
		t.Fatalf("no checked box: %s", out)
	}
	if !strings.Contains(out, `<input disabled="" type="checkbox"`) {
		t.Fatalf("no open box: %s", out)
	}
	if !strings.Contains(out, "<del>old</del>") {
		t.Fatalf("no strikethrough: %s", out)
	}
}

func TestEscapesText(t *testing.T) {
	out := render(t, "a < b & \"c\"")
	if !strings.Contains(out, "a &lt; b &amp; &quot;c&quot;") {
		t.Fatalf("got %s", out)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("é", MaxInput+100)
	got := truncate(long, MaxInput)
	if n := len([]rune(got)); n != MaxInput {
		t.Fatalf("runes = %d", n)
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short changed")
	}
	// Byte length over n but rune count under n.
	if s := strings.Repeat("é", 6); truncate(s, 8) != s {
		t.Fatal("multi-byte under limit changed")
	}
	out := render(t, strings.Repeat("a", MaxInput+500))
	if c := strings.Count(out, "a"); c != MaxInput {
		t.Fatalf("rendered %d chars", c)
	}
}

func TestClassify(t *testing.T) {
	if s, _ := classify("http://[::1"); s {
		t.Fatal("bad url accepted")
	}
	if s, _ := classify("a\x00b"); s {
		t.Fatal("control char accepted")
	}
}
