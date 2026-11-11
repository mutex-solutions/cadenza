package template

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestBuiltinsAllPresentAndValid(t *testing.T) {
	defs := Builtins()
	if len(defs) != 15 {
		t.Fatalf("got %d built-ins, want 15", len(defs))
	}
	for i, d := range defs {
		if d.Name != builtinNames[i] || d.Version != BuiltinVersion {
			t.Errorf("built-in %d is %q v%d", i, d.Name, d.Version)
		}
		res, err := d.Validate(d.Example, Local)
		if err != nil {
			t.Errorf("%s: example does not validate: %v", d.Name, err)
		}
		// Every slot should be shown in the example so agents see each block used.
		var ex map[string]json.RawMessage
		if err := json.Unmarshal(d.Example, &ex); err != nil {
			t.Fatal(err)
		}
		for _, s := range d.Slots {
			if s.Hint == "" || s.Label == "" {
				t.Errorf("%s: slot %q needs a label and hint", d.Name, s.Name)
			}
			if _, ok := ex[s.Name]; !ok && s.Block != BlockGallery && s.Block != BlockImage {
				t.Errorf("%s: example does not fill slot %q", d.Name, s.Name)
			}
		}
		if len(d.Guide) < 100 || d.UseFor == "" {
			t.Errorf("%s: guide or use_for too thin", d.Name)
		}
		_ = res
	}
}

func TestBuiltinUIChangeSlots(t *testing.T) {
	d, ok := Builtin("ui-change")
	if !ok {
		t.Fatal("ui-change missing")
	}
	var got []string
	for _, s := range d.Slots {
		got = append(got, s.Name+":"+string(s.Block))
	}
	want := []string{"summary:text", "before:image", "after:image", "demo:video", "changes:list", "risks:list"}
	if !slices.Equal(got, want) || !d.Slots[0].Required {
		t.Fatalf("slots = %v", got)
	}
	res, _ := d.Validate(d.Example, Local)
	if len(res.Paths) != 3 {
		t.Fatalf("example should reference 3 files, got %+v", res.Paths)
	}
}

func TestBuiltinUnknown(t *testing.T) {
	if _, ok := Builtin("nope"); ok {
		t.Fatal("found a built-in that does not exist")
	}
}

func TestBuiltinsAreCopies(t *testing.T) {
	d, _ := Builtin("options")
	d.Slots[0].Name = "changed"
	d.Example[0] = 'X'
	d.Ask.FromSlot = "changed"
	again, _ := Builtin("options")
	if again.Slots[0].Name == "changed" || again.Example[0] == 'X' || again.Ask.FromSlot == "changed" {
		t.Fatal("Builtin returned shared state")
	}
	all := Builtins()
	all[0].Slots[0].Name = "changed"
	if Builtins()[0].Slots[0].Name == "changed" {
		t.Fatal("Builtins returned shared state")
	}
}

func TestBuiltinMatchesFile(t *testing.T) {
	// Built-in JSON files carry no version; the loader sets it.
	raw, err := builtinFS.ReadFile("builtin/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["version"]; has {
		t.Fatal("built-in files must not set version")
	}
}
