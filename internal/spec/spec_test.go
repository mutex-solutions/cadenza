package spec

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestEndpointsAreWellFormed(t *testing.T) {
	names := map[string]bool{}
	routes := map[string]bool{}
	methods := map[string]bool{http.MethodGet: true, http.MethodPost: true, http.MethodDelete: true}
	auths := map[Auth]bool{AuthNone: true, AuthMachine: true, AuthSession: true, AuthOwner: true, AuthAny: true}
	for _, e := range Endpoints {
		if e.Name == "" || strings.ToLower(e.Name) != e.Name || strings.ContainsAny(e.Name, " _") {
			t.Errorf("bad name %q (use lowercase-with-dashes)", e.Name)
		}
		if names[e.Name] {
			t.Errorf("duplicate name %q", e.Name)
		}
		names[e.Name] = true
		if !methods[e.Method] {
			t.Errorf("%s: bad method %q", e.Name, e.Method)
		}
		if !strings.HasPrefix(e.Path, "/v1/") {
			t.Errorf("%s: path %q must start with /v1/", e.Name, e.Path)
		}
		route := e.Method + " " + e.Path
		if routes[route] {
			t.Errorf("duplicate route %q", route)
		}
		routes[route] = true
		if !auths[e.Auth] {
			t.Errorf("%s: bad auth %q", e.Name, e.Auth)
		}
		if e.Summary == "" || len(e.Summary) > 200 {
			t.Errorf("%s: summary must be 1..200 chars", e.Name)
		}
		if e.Method == http.MethodGet && e.Input != nil {
			t.Errorf("%s: GET endpoints take no body", e.Name)
		}
		if e.Agent && e.Auth != AuthSession && e.Auth != AuthAny {
			t.Errorf("%s: agent endpoints need session auth", e.Name)
		}
	}
}

// Every field of every wire type must have a json tag, so the TypeScript and
// MCP schemas generated from these types match what goes over the wire.
func TestAllFieldsHaveJSONTags(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] || rt.PkgPath() == "encoding/json" {
			return
		}
		seen[rt] = true
		for i := range rt.NumField() {
			f := rt.Field(i)
			if f.Anonymous {
				walk(f.Type, path+"."+f.Name)
				continue
			}
			if tag := f.Tag.Get("json"); tag == "" {
				t.Errorf("%s.%s has no json tag", path, f.Name)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	for _, e := range Endpoints {
		for _, v := range []any{e.Input, e.Output} {
			if v != nil {
				walk(reflect.TypeOf(v), e.Name)
			}
		}
	}
}

func TestWireTypesRoundTrip(t *testing.T) {
	for _, e := range Endpoints {
		for _, v := range []any{e.Input, e.Output} {
			if v == nil {
				continue
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("%s: marshal %T: %v", e.Name, v, err)
			}
			p := reflect.New(reflect.TypeOf(v)).Interface()
			if err := json.Unmarshal(b, p); err != nil {
				t.Fatalf("%s: unmarshal %T: %v", e.Name, v, err)
			}
		}
	}
}

func TestLookup(t *testing.T) {
	if e, ok := Lookup("show"); !ok || e.Path != "/v1/posts" {
		t.Fatal("Lookup(show) failed")
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("Lookup(nope) should fail")
	}
}
