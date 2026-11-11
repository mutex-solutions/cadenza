// Package access is who may do what: the owner's login, paired machines,
// pairing codes and per-agent-session keys (plan §7, D13, D16, D17).
//
// Keys:
//   - a web session token (cz_ws_…) is the owner, in the app;
//   - a machine key (cz_mk_…) can only mint session keys for its machine;
//   - a session key (cz_sk_…) can show, ask and read replies for threads on
//     its machine.
//
// Keys are random and shown once; only their SHA-256 is stored.
package access

import (
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Key prefixes.
const (
	PrefixWebSession = "cz_ws_"
	PrefixMachine    = "cz_mk_"
	PrefixSession    = "cz_sk_"
)

// Lifetimes.
const (
	WebSessionTTL     = 30 * 24 * time.Hour
	SessionKeyIdleTTL = 7 * 24 * time.Hour // plan §7: until 7 days unused
	PairingTTL        = 10 * time.Minute
	touchEvery        = time.Minute // how often last-used times are written
)

// Owner is the one person who owns this server.
type Owner struct {
	ID           kernel.OwnerID
	PasswordHash string
	CreatedAt    time.Time
}

// WebSession is a logged-in browser or app.
type WebSession struct {
	TokenHash string
	Owner     kernel.OwnerID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Machine is a paired computer or VM.
type Machine struct {
	ID         string
	Owner      kernel.OwnerID
	Name       string
	KeyHash    string
	CreatedAt  time.Time
	LastSeenAt time.Time
	RemovedAt  *time.Time
}

func (m *Machine) Removed() bool { return m.RemovedAt != nil }

// Pairing statuses. Claimed means the machine has collected its key.
const (
	PairingPending  = "pending"
	PairingApproved = "approved"
	PairingClaimed  = "claimed"
)

// Pairing is one attempt to pair a machine with a short code.
type Pairing struct {
	ID          string
	Code        string
	SecretHash  string
	Hostname    string
	Status      string
	Owner       kernel.OwnerID // set on approval
	MachineID   string         // set on approval
	MachineName string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// Expired reports whether a pairing can no longer be approved or claimed.
func (p *Pairing) Expired(now time.Time) bool { return !now.Before(p.ExpiresAt) }

// AgentSession is one run of an agent on a machine, identified by the agent's
// own session id so that resuming the agent keeps the same session.
type AgentSession struct {
	ID         string
	Owner      kernel.OwnerID
	MachineID  string
	Agent      string
	ExternalID string
	Cwd        string
	Repo       string
	KeyHash    string
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  *time.Time
}

// Usable reports whether the session's key still works at now.
func (s *AgentSession) Usable(now time.Time) bool {
	return s.RevokedAt == nil && now.Sub(s.LastUsedAt) < SessionKeyIdleTTL
}

// Role is what kind of caller a Principal is.
type Role string

const (
	RoleOwner   Role = "owner"
	RoleMachine Role = "machine"
	RoleSession Role = "session"
)

// Principal is an authenticated caller.
type Principal struct {
	Role        Role
	Owner       kernel.OwnerID
	MachineID   string // machine and session
	MachineName string // machine and session
	SessionID   string // session
	Agent       string // session
}

// Known agent names. Anything else is stored as given, lowercased.
var KnownAgents = map[string]string{
	"claude-code": "Claude Code",
	"codex":       "Codex",
	"gemini":      "Gemini",
	"hermes":      "Hermes",
	"cursor":      "Cursor",
}

// AgentLabel is the display name of an agent, e.g. "Claude Code".
func AgentLabel(agent string) string {
	if l, ok := KnownAgents[agent]; ok {
		return l
	}
	if agent == "" {
		return "Agent"
	}
	return agent
}
