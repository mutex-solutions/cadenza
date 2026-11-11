package spec

// PairStartInput begins pairing a machine. No credentials needed.
type PairStartInput struct {
	Hostname string `json:"hostname"` // suggested name, the owner can change it
}

type PairStartOutput struct {
	PairID  string `json:"pair_id"`
	Code    string `json:"code"`   // shown to the user, typed into the app, e.g. "KQ7-4XM"
	Secret  string `json:"secret"` // proves this caller started the pairing; never shown
	Expires int64  `json:"expires"`
}

// PairPollInput is sent by the machine until the owner approves.
type PairPollInput struct {
	Secret string `json:"secret"`
}

// Pairing statuses.
const (
	PairPending  = "pending"
	PairApproved = "approved"
	PairExpired  = "expired"
)

type PairPollOutput struct {
	Status      string `json:"status"`
	MachineID   string `json:"machine_id,omitempty"`
	MachineName string `json:"machine_name,omitempty"`
	// MachineKey is returned exactly once, on the first poll after approval.
	MachineKey string `json:"machine_key,omitempty"`
}

// PairApproveInput is sent by the owner from the app.
type PairApproveInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type PairApproveOutput struct {
	MachineID string `json:"machine_id"`
	Name      string `json:"name"`
}

// SessionKeyInput asks for a key for one agent session. Calling it again for
// the same agent session returns a new key for the same session, so a lost
// local key file does not lose the session's threads.
type SessionKeyInput struct {
	Agent          string `json:"agent"`            // claude-code, codex, gemini, hermes, other
	AgentSessionID string `json:"agent_session_id"` // the agent's own session id
	Cwd            string `json:"cwd,omitempty"`
	Repo           string `json:"repo,omitempty"`
}

type SessionKeyOutput struct {
	SessionID  string `json:"session_id"`
	SessionKey string `json:"session_key"`
}

// LoginInput logs the owner in with a password.
type LoginInput struct {
	Password string `json:"password"`
}

// LoginOutput carries the web session token, for apps that cannot use the
// cookie (the Android app sends it as a Bearer token).
type LoginOutput struct {
	Token   string `json:"token"`
	Expires int64  `json:"expires"`
}

// SetupInput sets the owner's password the first time the server runs.
type SetupInput struct {
	Password string `json:"password"`
}

type Me struct {
	OwnerID string `json:"owner_id"`
}

type Machine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastSeenAt int64  `json:"last_seen_at"`
	Sessions   int    `json:"sessions"`
}

type MachineList struct {
	Machines []Machine `json:"machines"`
}

type RenameMachineInput struct {
	Name string `json:"name"`
}
