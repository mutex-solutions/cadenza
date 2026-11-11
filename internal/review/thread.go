package review

import (
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Thread is one task. Its status follows plan §9:
//
//	active ──(owner requests changes)──► changes_requested ──(agent posts)──► active
//	active ──(agent: done)──► ready ──(owner approves)──► approved
//	any ──(owner archives)──► archived
type Thread struct {
	ID        string
	Owner     kernel.OwnerID
	SessionID string // the agent session that receives replies
	MachineID string
	Title     string
	Status    string
	NextSeq   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewThread starts a thread for an agent session.
func NewThread(a Actor, title string, now time.Time) *Thread {
	return &Thread{
		ID: kernel.NewID("t"), Owner: a.Owner, SessionID: a.SessionID, MachineID: a.MachineID,
		Title: title, Status: spec.ThreadActive, NextSeq: 1, CreatedAt: now, UpdatedAt: now,
	}
}

// Open reports whether agents may still post to the thread.
func (t *Thread) Open() bool { return t.Status != spec.ThreadArchived }

// VisibleTo reports whether an agent session may read and post to the thread:
// any session on the same machine may (plan gap G-3), nothing on other machines.
func (t *Thread) VisibleTo(a Actor) bool { return t.Owner == a.Owner && t.MachineID == a.MachineID }

// AddPost reserves the next post number. A new post answers a change request
// and reopens a ready or approved thread.
func (t *Thread) AddPost(now time.Time) (int, error) {
	if !t.Open() {
		return 0, kernel.Conflict("thread %s is archived", t.ID).WithFix("start a new thread with --new")
	}
	seq := t.NextSeq
	t.NextSeq++
	t.Status = spec.ThreadActive
	t.UpdatedAt = now
	return seq, nil
}

// Touch marks the thread changed, e.g. when a question is asked.
func (t *Thread) Touch(now time.Time) { t.UpdatedAt = now }

// Adopt moves the thread to another agent session on the same machine, e.g.
// after /clear or in tomorrow's session. Returns true if it changed hands.
func (t *Thread) Adopt(a Actor) bool {
	if t.SessionID == a.SessionID {
		return false
	}
	t.SessionID = a.SessionID
	return true
}

// MarkReady is the agent saying the work is finished.
func (t *Thread) MarkReady(now time.Time) error {
	if !t.Open() {
		return kernel.Conflict("thread %s is archived", t.ID)
	}
	t.Status = spec.ThreadReady
	t.UpdatedAt = now
	return nil
}

// ApplyVerdict applies the owner's approve or request-changes.
func (t *Thread) ApplyVerdict(verdict string, now time.Time) error {
	if !t.Open() {
		return kernel.Conflict("thread %s is archived", t.ID).WithFix("reopen it first")
	}
	switch verdict {
	case spec.VerdictApprove:
		t.Status = spec.ThreadApproved
	case spec.VerdictRequestChanges:
		t.Status = spec.ThreadChangesRequested
	default:
		return kernel.Invalid("unknown verdict %q", verdict).WithFix("use %q or %q", spec.VerdictApprove, spec.VerdictRequestChanges)
	}
	t.UpdatedAt = now
	return nil
}

// SetStatus is the owner approving, archiving or reopening a thread.
func (t *Thread) SetStatus(status string, now time.Time) error {
	switch status {
	case spec.ThreadApproved, spec.ThreadArchived, spec.ThreadActive:
	default:
		return kernel.Invalid("cannot set status %q", status).
			WithFix("use %q, %q or %q", spec.ThreadApproved, spec.ThreadArchived, spec.ThreadActive)
	}
	t.Status = status
	t.UpdatedAt = now
	return nil
}
