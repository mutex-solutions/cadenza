// Package spec is Cadenza's contract (plan D4): every command and every API
// call is defined here once, as Go types plus a short description. The HTTP
// routes, the CLI, the MCP tools, the TypeScript types and cadenza.md are all
// built from this package, and tests check that none of them drift.
//
// Wire types only: no behaviour, no imports beyond media and the standard library.
package spec

// Auth is who may call an endpoint.
type Auth string

const (
	AuthNone    Auth = "none"    // anyone (pairing start, health)
	AuthMachine Auth = "machine" // a paired machine's key; may only create session keys
	AuthSession Auth = "session" // an agent session key
	AuthOwner   Auth = "owner"   // the logged-in owner (web or Android app)
	AuthAny     Auth = "any"     // a session key or the owner
)

// Endpoint is one HTTP call.
type Endpoint struct {
	Name    string // stable id, e.g. "show"; also the MCP tool name for agent calls
	Method  string
	Path    string // Go 1.22 ServeMux pattern without the method, e.g. "/v1/questions/{id}/wait"
	Auth    Auth
	Summary string // one line, shown in help, MCP and cadenza.md
	Input   any    // zero value of the request body type, or nil
	Output  any    // zero value of the response body type, or nil
	// Agent marks calls agents make through the CLI and MCP.
	Agent bool
}

// Error is the body of every non-2xx response.
type Error struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// Version is returned by GET /v1/version.
type Version struct {
	Server        string `json:"server"`
	API           int    `json:"api"`
	MinCLIVersion string `json:"min_cli_version"`
	// SetupNeeded is true until the owner password is set.
	SetupNeeded bool `json:"setup_needed"`
}

// APIVersion is bumped only for breaking changes; paths carry it as /v1.
const APIVersion = 1
