package access

import (
	"context"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Repo is the storage access needs. Get* methods return a kernel NotFound
// error when nothing matches.
type Repo interface {
	GetOwner(ctx context.Context) (*Owner, error)  // the single owner
	SaveOwner(ctx context.Context, o *Owner) error // insert or update

	CreateWebSession(ctx context.Context, s *WebSession) error
	GetWebSession(ctx context.Context, tokenHash string) (*WebSession, error)
	DeleteWebSession(ctx context.Context, tokenHash string) error

	CreateMachine(ctx context.Context, m *Machine) error
	GetMachine(ctx context.Context, owner kernel.OwnerID, id string) (*Machine, error)
	GetMachineByKeyHash(ctx context.Context, keyHash string) (*Machine, error)
	UpdateMachine(ctx context.Context, m *Machine) error
	ListMachines(ctx context.Context, owner kernel.OwnerID) ([]MachineRow, error)

	CreatePairing(ctx context.Context, p *Pairing) error
	GetPairing(ctx context.Context, id string) (*Pairing, error)
	GetPendingPairingByCode(ctx context.Context, code string) (*Pairing, error)
	UpdatePairing(ctx context.Context, p *Pairing) error

	GetAgentSessionByExternal(ctx context.Context, machineID, agent, externalID string) (*AgentSession, error)
	GetAgentSessionByKeyHash(ctx context.Context, keyHash string) (*AgentSession, error)
	GetAgentSession(ctx context.Context, owner kernel.OwnerID, id string) (*AgentSession, error)
	CreateAgentSession(ctx context.Context, s *AgentSession) error
	UpdateAgentSession(ctx context.Context, s *AgentSession) error
	// ListAgentSessions lists unrevoked sessions on machines that are not removed.
	ListAgentSessions(ctx context.Context, owner kernel.OwnerID) ([]SessionRow, error)

	AppendEvent(ctx context.Context, e *eventlog.Event) error
}

// SessionRow is a live agent session with its machine's name.
type SessionRow struct {
	AgentSession
	MachineName string
}

// MachineRow is a machine with its live session count, for listings.
type MachineRow struct {
	Machine
	Sessions int
}

// Store runs fn in one transaction. Repo methods on the Store itself run
// outside a transaction.
type Store interface {
	Repo
	InTx(ctx context.Context, fn func(Repo) error) error
}

// Publisher is told about events after their transaction commits.
type Publisher interface {
	Publish(events ...eventlog.Event)
}
