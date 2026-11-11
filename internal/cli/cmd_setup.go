package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/guide"
	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Markers around the block cadenza setup manages in an agent's instruction
// file, so running setup again updates it in place.
const (
	markStart = "<!-- cadenza:start -->"
	markEnd   = "<!-- cadenza:end -->"
)

// agentFiles are where each agent reads standing instructions.
func (a *App) agentFiles(project bool) map[string]string {
	if project {
		return map[string]string{
			"claude-code": filepath.Join(a.Cwd, "CLAUDE.md"),
			"codex":       filepath.Join(a.Cwd, "AGENTS.md"),
			"gemini":      filepath.Join(a.Cwd, "GEMINI.md"),
		}
	}
	return map[string]string{
		"claude-code": filepath.Join(a.Home, ".claude", "CLAUDE.md"),
		"codex":       filepath.Join(a.Home, ".codex", "AGENTS.md"),
		"gemini":      filepath.Join(a.Home, ".gemini", "GEMINI.md"),
	}
}

func cmdSetup(_ context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"agent"}, []string{"project", "print"})
	if err != nil {
		return err
	}
	if f.bool("print") {
		_, err := a.Stdout.Write([]byte(guide.Markdown))
		return err
	}
	// The guide is written once; agent files point at it (Claude Code imports
	// it with @path) or carry a copy (agents without imports).
	guidePath := filepath.Join(a.configDir(), "cadenza.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(guidePath, []byte(guide.Markdown), 0o644); err != nil {
		return err
	}
	files := a.agentFiles(f.bool("project"))
	which := orDefault(f.str("agent"), "all")
	var targets []string
	if which == "all" {
		targets = []string{"claude-code", "codex", "gemini"}
	} else if _, ok := files[which]; ok {
		targets = []string{which}
	} else {
		return kernel.Invalid("unknown agent %q", which).
			WithFix("use claude-code, codex, gemini or all; for other agents add %s to their instructions", guidePath)
	}
	installed := map[string]string{}
	for _, agent := range targets {
		path := files[agent]
		block := "Use Cadenza to show your work and ask your human questions; the guide is in " + guidePath + "."
		if agent == "claude-code" {
			block += "\n@" + guidePath
		} else {
			block += "\n\n" + guide.Markdown
		}
		if err := upsertBlock(path, block); err != nil {
			return err
		}
		installed[agent] = path
	}
	return a.print(map[string]any{"guide": guidePath, "installed": installed,
		"hermes": "add the guide to Hermes' instructions and pass --session (Hermes has no session id variable)"})
}

// upsertBlock writes block between the markers in path, replacing an
// existing block or appending a new one.
func upsertBlock(path, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(old)
	wrapped := markStart + "\n" + strings.TrimSpace(block) + "\n" + markEnd
	if i := strings.Index(text, markStart); i >= 0 {
		if j := strings.Index(text[i:], markEnd); j >= 0 {
			text = text[:i] + wrapped + text[i+j+len(markEnd):]
			return writeFileAtomic(path, []byte(text), 0o644)
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return writeFileAtomic(path, []byte(text+wrapped+"\n"), 0o644)
}
