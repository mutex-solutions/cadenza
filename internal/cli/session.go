package cli

import (
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// agentSession is the agent session the CLI acts for.
type agentSession struct {
	Agent string
	ID    string
}

// knownSessionVars are environment variables agents set for the commands
// they run (verified: Claude Code documents CLAUDE_CODE_SESSION_ID; Codex
// sets CODEX_THREAD_ID; Gemini sets GEMINI_SESSION_ID for hooks).
var knownSessionVars = []struct{ env, agent string }{
	{"CLAUDE_CODE_SESSION_ID", "claude-code"},
	{"CODEX_THREAD_ID", "codex"},
	{"GEMINI_SESSION_ID", "gemini"},
	{"HERMES_SESSION_ID", "hermes"},
}

// resolveSession finds the agent session (plan gap G-1): an explicit
// --session or CADENZA_SESSION first, then a known agent variable. With
// neither it refuses rather than guess, because a guessed id would not
// survive the agent resuming.
func (a *App) resolveSession(f *flags) (agentSession, error) {
	agent := strings.ToLower(f.str("agent-name"))
	if agent == "" {
		agent = strings.ToLower(a.Getenv("CADENZA_AGENT"))
	}
	if id := f.str("session"); id != "" {
		return agentSession{Agent: orDefault(agent, detectAgent(a.Getenv)), ID: id}, nil
	}
	if id := a.Getenv("CADENZA_SESSION"); id != "" {
		return agentSession{Agent: orDefault(agent, detectAgent(a.Getenv)), ID: id}, nil
	}
	for _, v := range knownSessionVars {
		if id := a.Getenv(v.env); id != "" {
			return agentSession{Agent: orDefault(agent, v.agent), ID: id}, nil
		}
	}
	return agentSession{}, kernel.Invalid("no agent session id found").
		WithFix("pass --session <your session id>, or set CADENZA_SESSION; Claude Code, Codex and Gemini set one automatically")
}

func detectAgent(getenv func(string) string) string {
	for _, v := range knownSessionVars {
		if getenv(v.env) != "" {
			return v.agent
		}
	}
	return "other"
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
