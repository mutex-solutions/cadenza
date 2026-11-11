// Package cli is the `cadenza` command agents run on each machine (plan D2,
// D3): it pairs the machine, keeps one key per agent session, converts and
// uploads files straight to storage, and prints short JSON. Nothing runs in
// the background.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// App is one invocation of the CLI. Everything it touches outside the
// process is a field, so tests can run it in isolation.
type App struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Getenv    func(string) string
	HTTP      *http.Client
	ConfigDir string // ~/.config/cadenza unless set
	Cwd       string
	Home      string
	Version   string
	Hostname  string
	// Sleep is used between polls (pairing); tests make it instant.
	Sleep func(context.Context, time.Duration) error
}

// NewApp returns an App wired to the real process environment.
func NewApp(version string) *App {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	return &App{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv,
		HTTP: &http.Client{Timeout: 0}, Cwd: cwd, Home: home, Version: version, Hostname: host, Sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// command is one subcommand.
type command struct {
	usage string
	help  string
	run   func(ctx context.Context, a *App, args []string) error
}

func commands() map[string]command {
	return map[string]command{
		"login":         {"login <server-url>", "Pair this machine with a Cadenza server (once per machine).", cmdLogin},
		"logout":        {"logout", "Forget this machine's pairing and session keys.", cmdLogout},
		"show":          {"show [files…] --title T [--template NAME --data JSON|@file]", summary("show"), cmdShow},
		"ask":           {`ask "question" [--choice A]… [--waiting] [--post ID]`, summary("ask"), cmdAsk},
		"wait":          {"wait <question-id> [--timeout 10m]", summary("wait"), cmdWait},
		"responses":     {"responses", summary("responses"), cmdResponses},
		"reply":         {`reply <reply-id> "text"`, summary("reply"), cmdReply},
		"done":          {"done [--thread ID] [--summary S]", summary("done"), cmdDone},
		"threads":       {"threads", summary("threads"), cmdThreads},
		"templates":     {"templates [name]", summary("templates") + " With a name: " + summary("template"), cmdTemplates},
		"make-template": {"make-template <file.json|->", summary("make-template"), cmdMakeTemplate},
		"setup":         {"setup [--agent claude-code|codex|gemini|all] [--project]", "Install cadenza.md, the guide that teaches agents to use Cadenza.", cmdSetup},
		"mcp":           {"mcp", "Serve the Cadenza tools over MCP on stdin/stdout, for agents that prefer MCP.", cmdMCP},
		"status":        {"status", "Show the server, this machine and the current agent session.", cmdStatus},
		"version":       {"version", "Print the version.", cmdVersion},
	}
}

func summary(name string) string {
	e, ok := spec.Lookup(name)
	if !ok {
		panic("cli: unknown endpoint " + name)
	}
	return e.Summary
}

// Run runs the CLI and returns the process exit code.
func (a *App) Run(args []string) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		a.usage()
		return 0
	}
	cmd, ok := commands()[args[0]]
	if !ok {
		fmt.Fprintf(a.Stderr, "cadenza: unknown command %q\n\n", args[0])
		a.usage()
		return spec.ExitError
	}
	err := cmd.run(ctx, a, args[1:])
	if err == nil {
		return 0
	}
	var ex *exitError
	if errors.As(err, &ex) {
		return ex.code
	}
	return a.fail(err)
}

func (a *App) usage() {
	fmt.Fprintln(a.Stdout, "cadenza: show your work to your human and ask them questions.\n\nUsage:")
	names := make([]string, 0)
	for n := range commands() {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := commands()[n]
		fmt.Fprintf(a.Stdout, "  cadenza %-58s %s\n", c.usage, firstSentence(c.help))
	}
	fmt.Fprintln(a.Stdout, "\nGlobal: --session <id> (agent session, if your agent sets none), --server <url>.\nGuide: cadenza setup, or https://github.com/mutex-solutions/cadenza/blob/main/docs/cadenza.md")
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

// exitError ends the command with a specific exit code, having printed its output.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// fail prints an error as JSON on stderr and returns its exit code.
func (a *App) fail(err error) int {
	var ke *kernel.Error
	if !errors.As(err, &ke) {
		ke = &kernel.Error{Kind: kernel.KindInternal, Msg: err.Error()}
	}
	out := map[string]string{"error": ke.Msg, "kind": string(ke.Kind)}
	if ke.Fix != "" {
		out["fix"] = ke.Fix
	}
	if ke.Err != nil && ke.Kind != kernel.KindUnavailable {
		out["error"] = ke.Msg + ": " + ke.Err.Error()
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(a.Stderr, string(b))
	if ke.Kind == kernel.KindUnavailable {
		return spec.ExitUnreachable
	}
	return spec.ExitError
}

// print writes v as one line of JSON on stdout.
func (a *App) print(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.Stdout, string(b))
	return err
}
