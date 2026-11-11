package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Every agent endpoint has a CLI command (template is `templates <name>`,
// responses-ack happens inside `responses`), so the CLI and the contract agree.
func TestEveryAgentEndpointHasACommand(t *testing.T) {
	cmds := commands()
	for _, e := range spec.Endpoints {
		if !e.Agent || e.Name == "template" || e.Name == "responses-ack" {
			continue
		}
		if _, ok := cmds[e.Name]; !ok {
			t.Errorf("no command for agent endpoint %s", e.Name)
		}
	}
}

func TestParseFlags(t *testing.T) {
	f, err := parseFlags([]string{"a.png", "--title", "T", "--choice=x", "--choice", "y", "--new", "b.png", "--", "--c"}, []string{"title", "choice"}, []string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	if f.str("title") != "T" || len(f.all("choice")) != 2 || !f.bool("new") || len(f.pos) != 3 || f.pos[2] != "--c" {
		t.Fatalf("%+v", f)
	}
	if _, err := parseFlags([]string{"--title"}, []string{"title"}, nil); err == nil {
		t.Fatal("missing value accepted")
	}
	if _, err := parseFlags([]string{"--nope"}, nil, nil); err == nil {
		t.Fatal("unknown flag accepted")
	}
	f, _ = parseFlags([]string{"--new=false", "-"}, nil, []string{"new"})
	if f.bool("new") || f.pos[0] != "-" {
		t.Fatal("bool=false and - positional")
	}
}

func TestParseTimeout(t *testing.T) {
	for in, want := range map[string]time.Duration{"30": 30 * time.Second, "10m": 10 * time.Minute, "0": 0} {
		d, forever, err := parseTimeout(in)
		if err != nil || forever || d != want {
			t.Errorf("%q: %v %v %v", in, d, forever, err)
		}
	}
	for _, in := range []string{"", "none", "never"} {
		if _, forever, _ := parseTimeout(in); !forever {
			t.Errorf("%q should wait forever", in)
		}
	}
	if _, _, err := parseTimeout("-5m"); err == nil {
		t.Fatal("negative timeout")
	}
}

func TestOlderThan(t *testing.T) {
	cases := map[[2]string]bool{{"0.1.0", "0.2.0"}: true, {"0.2.0", "0.1.9"}: false, {"v1.0.0", "1.0.0"}: false, {"dev", "9.9.9"}: false,
		{"1.2", "1.2.1"}: true, {"1.10.0", "1.9.0"}: false, {"1.0.0", ""}: false}
	for c, want := range cases {
		if got := olderThan(c[0], c[1]); got != want {
			t.Errorf("olderThan(%q, %q) = %v", c[0], c[1], got)
		}
	}
}

func TestGitRemote(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n\tbare = false\n[remote \"origin\"]\n\turl = https://user:tok@github.com/acme/app.git\n"), 0o644)
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0o755)
	if got := gitRemote(sub); got != "https://github.com/acme/app.git" {
		t.Fatalf("gitRemote = %q", got)
	}
	if gitRemote(t.TempDir()) != "" {
		t.Fatal("no repo")
	}
	if stripCredentials("git@github.com:a/b.git") != "git@github.com:a/b.git" {
		t.Fatal("ssh remotes are kept")
	}
}

func TestSessionKeyPathIsSafe(t *testing.T) {
	a := &App{ConfigDir: "/cfg", Getenv: func(string) string { return "" }}
	p := a.sessionKeyPath("claude-code", "../../etc/passwd")
	if filepath.Dir(p) != "/cfg/sessions" {
		t.Fatalf("path escaped: %s", p)
	}
	long := a.sessionKeyPath("x", string(make([]byte, 400)))
	if len(filepath.Base(long)) > 200 {
		t.Fatal("long ids must be shortened")
	}
}

func TestRedactJSONKeepsJSONValid(t *testing.T) {
	data := []byte(`{"summary":"key ` + "AKIA" + "IOSFODNN7EXAMPLE" + ` leaked"}`)
	out := redactJSON(data)
	if string(out) == string(data) {
		t.Fatal("secret not redacted")
	}
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal("redacted data must stay valid JSON")
	}
}
