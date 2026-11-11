// Package eventlog is the numbered change log (plan D15). Every change to a
// thread, post, question, reply or machine is appended in the same database
// transaction as the change itself. After commit, the change is published on
// an in-process Bus, which wakes live streams, waiting agents and
// notifications. Apps that were offline catch up by reading the log after the
// last number they saw.
package eventlog

import (
	"context"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Event is one numbered change. Seq is assigned by storage on append.
type Event struct {
	Seq      int64
	Owner    kernel.OwnerID
	Kind     string // one of the spec.Event* kinds
	ThreadID string
	RefID    string // the post, question, reply or machine that changed
	// SessionID is the agent session the change concerns, when there is one.
	// It lets waiting agents ignore changes that are not theirs.
	SessionID string
	At        time.Time
}

// Retention is how long events are kept. Apps further behind get a reset and
// reload everything.
const Retention = 30 * 24 * time.Hour

// Reader reads the stored log, for catching up and pruning.
type Reader interface {
	ListEvents(ctx context.Context, owner kernel.OwnerID, after int64, limit int) ([]Event, error)
	OldestSeq(ctx context.Context, owner kernel.OwnerID) (int64, error)
	LatestSeq(ctx context.Context, owner kernel.OwnerID) (int64, error)
	DeleteEventsBefore(ctx context.Context, before time.Time) (int64, error)
}
