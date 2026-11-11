package prepare

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/media"
)

const selfContained = `<!doctype html>
<html><head><style>body{background:url("data:image/png;base64,AAAA")}</style></head>
<body>
<!-- <img src="old.png"> -->
<a href="#top">top</a> <a href="mailto:a@b.c">mail</a> <a href="javascript:void(0)">x</a>
<img src="data:image/png;base64,AAAA" alt="">
<a href="{{ link }}">tpl</a>
<script>const img = new Image(); img.src = "frame.png"; el.href = url;</script>
</body></html>`

func TestHTMLSelfContained(t *testing.T) {
	path := writeFile(t, "report.html", []byte(selfContained))
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != media.SourceHTML || p.File.Type != media.TypeHTML || p.File.Name != "report.html" || p.Path != path || len(p.Warnings) != 0 {
		t.Fatalf("got %+v", p)
	}
	checkRef(t, p)
}

func TestHTMLLocalRefsRefused(t *testing.T) {
	page := `<html><script src="./app.js"></script><body><img src='img/a.png'></body></html>`
	_, err := PrepareFile(context.Background(), writeFile(t, "index.html", []byte(page)), Options{})
	e := wantInvalid(t, err)
	if e.Fix != "inline ./app.js, img/a.png into the HTML file; cadenza shows one self-contained file" {
		t.Fatalf("fix = %q", e.Fix)
	}
	_, err = PrepareFile(context.Background(), writeFile(t, "index.html", []byte(`<script src="app.js"></script>`)), Options{})
	if e := wantInvalid(t, err); e.Fix != "inline app.js into the HTML file; cadenza shows one self-contained file" {
		t.Fatalf("fix = %q", e.Fix)
	}
}

func TestHTMLManyRefsShortened(t *testing.T) {
	var b strings.Builder
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		b.WriteString(`<img src="` + n + `.png">`)
	}
	_, err := PrepareFile(context.Background(), writeFile(t, "p.html", []byte(b.String())), Options{})
	if e := wantInvalid(t, err); strings.Contains(e.Fix, "f.png") {
		t.Fatalf("fix not shortened: %q", e.Fix)
	}
}

func TestHTMLExternalWarned(t *testing.T) {
	page := `<script src="https://cdn.example.com/x.js"></script><link href=//fonts.example.com/f.css rel=stylesheet>`
	p, err := PrepareFile(context.Background(), writeFile(t, "p.html", []byte(page)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 2 || !strings.Contains(p.Warnings[0], "no network") {
		t.Fatalf("warnings %q", p.Warnings)
	}
}

func TestFindRefs(t *testing.T) {
	page := `<style>@import "base.css"; .a{background:url(img/bg.png)} .b{background:url('https://x.test/b.png')}</style>
<object data="chart.svg"></object><video poster="file:///tmp/p.png"></video><div data-src="ignored.png"></div>
<img src=plain.png><a href="blob:abc">b</a>`
	got := findRefs(page)
	wantLocal := []string{"chart.svg", "file:///tmp/p.png", "plain.png", "img/bg.png", "base.css"}
	if !reflect.DeepEqual(got.local, wantLocal) || !reflect.DeepEqual(got.external, []string{"https://x.test/b.png"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestHTMLLimit(t *testing.T) {
	_, err := PrepareFile(context.Background(), writeFile(t, "p.html", []byte(selfContained)), Options{Limits: Limits{PageBytes: 100}})
	if e := wantInvalid(t, err); e.Fix == "" {
		t.Fatal("missing fix")
	}
}

func TestHTMLSecretsRedacted(t *testing.T) {
	token := "ghp_" + strings.Repeat("a1B2", 9)
	p, err := PrepareFile(context.Background(), writeFile(t, "p.html", []byte("<p>"+token+"</p>")), Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(checkRef(t, p))
	if p.Path != "" || strings.Contains(body, token) || body != "<p>[redacted github-token]</p>" || len(p.Warnings) != 1 {
		t.Fatalf("got %q %+v", body, p)
	}
}
