package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeScriptCoversTheSpec(t *testing.T) {
	ts := TypeScript()
	for _, want := range []string{
		"export interface ShowInput {",
		"export interface ResponseFile extends FileRef {",
		"thread_id?: string;",
		"data?: unknown;",
		"headers: Record<string, string>;",
		`"show": { method: "POST", path: "/v1/posts", auth: "session"`,
		`"show": { input: ShowInput; output: ShowOutput };`,
		`"logout": { input: void; output: void };`,
		`export const QuestionOpen = "open";`,
		"export const APIVersion = 1;",
	} {
		if !strings.Contains(ts, want) {
			t.Errorf("api.gen.ts lacks %q", want)
		}
	}
}

func TestGuideMentionsEveryAgentCommandAndExitCode(t *testing.T) {
	g := Guide()
	for _, want := range []string{"cadenza show", "cadenza ask", "cadenza wait", "cadenza responses", "cadenza reply",
		"cadenza done", "cadenza threads", "cadenza templates", "cadenza make-template", "0 answered", "2 still open",
		"3 dismissed", "4 replaced", "5 Cadenza unreachable", "self-contained HTML"} {
		if !strings.Contains(g, want) {
			t.Errorf("guide lacks %q", want)
		}
	}
	// Claude Code truncates long tool descriptions; the guide itself should stay short too.
	if len(g) > 6000 {
		t.Errorf("guide is %d bytes; keep it under 6000", len(g))
	}
}

// The committed generated files must match the generator (CI runs make gen too).
func TestGeneratedFilesAreUpToDate(t *testing.T) {
	root := filepath.Join("..", "..")
	for path, want := range map[string]string{
		"web/packages/core/src/api.gen.ts": TypeScript(),
		"internal/guide/cadenza.md":        Guide(),
		"docs/cadenza.md":                  Guide(),
	} {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("%s: %v (run make gen)", path, err)
		}
		if string(got) != want {
			t.Errorf("%s is stale; run make gen", path)
		}
	}
}
