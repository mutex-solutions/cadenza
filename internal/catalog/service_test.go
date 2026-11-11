package catalog_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/sqlite"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/template"
)

type upper struct{}

func (upper) Render(md string) string { return "<p>" + strings.ToUpper(md) + "</p>" }

func newService(t *testing.T) *catalog.Service {
	t.Helper()
	st, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewService(st.Catalog(), upper{}, kernel.NewFakeClock(time.Now()))
}

var ctx = context.Background()

const custom = `{
  "name": "db-migration",
  "use_for": "A database schema change",
  "guide": "Say what changes and how to roll back.",
  "slots": [{"name": "summary", "block": "text", "required": true}, {"name": "sql", "block": "code"}],
  "example": {"summary": "Add index", "sql": {"language": "sql", "code": "CREATE INDEX i ON t (c);"}}
}`

func TestListIncludesBuiltinsAndCustom(t *testing.T) {
	s := newService(t)
	list, err := s.List(ctx, "o_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Templates) != len(template.Builtins()) || len(list.Templates) != 15 {
		t.Fatalf("want 15 built-ins, got %d", len(list.Templates))
	}
	if _, err := s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(custom)); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List(ctx, "o_1")
	last := list.Templates[len(list.Templates)-1]
	if last.Name != "db-migration" || last.Source != catalog.SourceAgent || last.UseFor == "" {
		t.Fatalf("custom template missing from list: %+v", last)
	}
	other, _ := s.List(ctx, "o_2")
	if len(other.Templates) != 15 {
		t.Fatal("custom templates belong to their owner")
	}
}

func TestMakeVersionsAndReservedNames(t *testing.T) {
	s := newService(t)
	out, err := s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(custom))
	if err != nil || out.Version != 1 || !out.Created {
		t.Fatalf("first make: %+v %v", out, err)
	}
	out, err = s.Make(ctx, "o_1", catalog.SourceOwner, json.RawMessage(custom))
	if err != nil || out.Version != 2 || out.Created {
		t.Fatalf("second make must add a version: %+v %v", out, err)
	}
	reserved := strings.Replace(custom, "db-migration", "ui-change", 1)
	if _, err := s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(reserved)); !kernel.Is(err, kernel.KindConflict) {
		t.Fatalf("built-in names are reserved: %v", err)
	}
	if _, err := s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(`{"name":"x"}`)); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("invalid definition accepted: %v", err)
	}
}

func TestGetAndLookup(t *testing.T) {
	s := newService(t)
	info, err := s.Get(ctx, "o_1", "ui-change")
	if err != nil || info.Source != catalog.SourceBuiltin || info.Guide == "" || len(info.Example) == 0 || info.Version != 1 {
		t.Fatalf("Get builtin: %+v %v", info, err)
	}
	if _, err := s.Get(ctx, "o_1", "nope"); !kernel.Is(err, kernel.KindNotFound) || !strings.Contains(err.Error(), "cadenza templates") {
		t.Fatalf("unknown template: %v", err)
	}
	if _, _, err := s.Lookup(ctx, "o_1", "ui-change", 2); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("built-ins only have version 1")
	}
	s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(custom))
	s.Make(ctx, "o_1", catalog.SourceAgent, json.RawMessage(strings.Replace(custom, "A database schema change", "Schema change v2", 1)))
	d, _, err := s.Lookup(ctx, "o_1", "db-migration", 1)
	if err != nil || d.Version != 1 || d.UseFor != "A database schema change" {
		t.Fatalf("old versions stay readable: %+v %v", d, err)
	}
	info, _ = s.Get(ctx, "o_1", "db-migration")
	if info.Version != 2 || info.Source != catalog.SourceAgent {
		t.Fatalf("Get returns the latest: %+v", info)
	}
}

func TestCheck(t *testing.T) {
	s := newService(t)
	sha := strings.Repeat("a", 64)
	data := `{"summary":"Promo field moved","after":{"file":{"sha256":"` + sha + `","size":10,"type":"image/webp","name":"after.webp"}}}`
	c, err := s.Check(ctx, "o_1", "ui-change", 0, json.RawMessage(data))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 || len(c.Files) != 1 || c.Files[0].SHA256 != sha || c.Question != nil {
		t.Fatalf("checked: %+v", c)
	}
	if !strings.Contains(string(c.View), "PROMO FIELD MOVED") {
		t.Fatalf("markdown not rendered: %s", c.View)
	}
	// Local paths are not allowed on the server.
	if _, err := s.Check(ctx, "o_1", "ui-change", 0, json.RawMessage(`{"summary":"x","after":{"file":"./a.png"}}`)); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("local path accepted on the server: %v", err)
	}
	if _, err := s.Check(ctx, "o_1", "nope", 0, json.RawMessage(`{}`)); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("unknown template")
	}
}

func TestCheckOptionsAsksAQuestion(t *testing.T) {
	s := newService(t)
	d, _ := template.Builtin("options")
	c, err := s.Check(ctx, "o_1", "options", 0, d.Example)
	if err != nil {
		t.Fatal(err)
	}
	if c.Question == nil || len(c.Question.Choices) < 2 || c.Question.Text == "" {
		t.Fatalf("options must ask a question: %+v", c.Question)
	}
}
