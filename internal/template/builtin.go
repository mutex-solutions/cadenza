package template

import (
	"embed"
	"fmt"
)

//go:embed builtin/*.json
var builtinFS embed.FS

// BuiltinVersion is the version of every built-in template.
const BuiltinVersion = 1

// builtinNames is the order built-ins are listed in (plan §4).
var builtinNames = []string{
	"ui-change", "feature-demo", "bug-fix", "code-change", "test-report",
	"perf-compare", "data-report", "experiment", "options", "plan",
	"research", "design-review", "progress", "incident", "release-notes",
}

var builtins = loadBuiltins()

// loadBuiltins parses the embedded templates. A bad one is a bug caught by
// the tests, so it panics.
func loadBuiltins() []Definition {
	defs := make([]Definition, 0, len(builtinNames))
	for _, name := range builtinNames {
		raw, err := builtinFS.ReadFile("builtin/" + name + ".json")
		if err != nil {
			panic(fmt.Sprintf("template: built-in %q is missing: %v", name, err))
		}
		d, err := ParseDefinition(raw)
		if err != nil {
			panic(fmt.Sprintf("template: built-in %q is invalid: %v", name, err))
		}
		if d.Name != name {
			panic(fmt.Sprintf("template: built-in file %q names itself %q", name, d.Name))
		}
		d.Version = BuiltinVersion
		defs = append(defs, d)
	}
	return defs
}

// Builtins returns the built-in templates in plan order. The caller owns the
// returned slice and may change it.
func Builtins() []Definition {
	out := make([]Definition, len(builtins))
	for i, d := range builtins {
		out[i] = d.clone()
	}
	return out
}

// Builtin returns the built-in template with this name.
func Builtin(name string) (Definition, bool) {
	for _, d := range builtins {
		if d.Name == name {
			return d.clone(), true
		}
	}
	return Definition{}, false
}

// clone copies d so callers cannot change the shared built-ins.
func (d Definition) clone() Definition {
	d.Slots = append([]Slot(nil), d.Slots...)
	d.Example = append([]byte(nil), d.Example...)
	if d.Ask != nil {
		q := *d.Ask
		d.Ask = &q
	}
	return d
}
