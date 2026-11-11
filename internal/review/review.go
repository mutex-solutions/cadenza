// Package review is Cadenza's core: threads (one per task), posts (final once
// made), questions and the owner's replies (plan §4, §5, §9, D5, D12).
//
// Rules live on the aggregates (Thread, Question). The Service runs use cases
// in one transaction each, appends events to the event log in that same
// transaction, and publishes them after commit.
package review

import (
	"encoding/json"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Limits on what agents send.
const (
	MaxTitle       = 200
	MaxSummary     = 4000
	MaxRetryID     = 128
	MaxAttachments = 20
	MaxQuestion    = 4000
	MaxChoices     = 10
	MaxChoice      = 200
	MaxReplyText   = 20000
	MaxReplyFiles  = 20
)

// Actor is an agent session acting through its key.
type Actor struct {
	Owner       kernel.OwnerID
	SessionID   string
	MachineID   string
	MachineName string
	Agent       string
}

// Post is something an agent showed. It never changes after it is made.
type Post struct {
	ID          string
	Owner       kernel.OwnerID
	ThreadID    string
	Seq         int
	Title       string
	Summary     string
	SummaryHTML string
	SessionID   string
	RetryID     string
	CreatedAt   time.Time
	Attachments []Attachment
}

// Attachment is a checked, filled template or a single file.
type Attachment struct {
	Index    int
	Template string
	Version  int
	Data     json.RawMessage // template data as sent (with file refs)
	View     json.RawMessage // template data with markdown rendered, for the apps
	File     *media.FileRef
}

// Reply is something the owner sent, delivered to the thread's agent session.
type Reply struct {
	ID         string
	Owner      kernel.OwnerID
	ThreadID   string
	PostID     string
	QuestionID string
	SessionID  string // the session it is delivered to
	Kind       string
	Choice     string
	Text       string
	Verdict    string
	Spot       *spec.Spot
	Files      []media.FileRef
	Extra      json.RawMessage
	// Seq orders delivery; it is the number of the reply.added event.
	Seq          int64
	CreatedAt    time.Time
	DeliveredAt  *time.Time
	AgentReply   string
	AgentReplyAt *time.Time
}

// ThreadRow is a thread with what lists show next to it.
type ThreadRow struct {
	Thread
	MachineName   string
	Agent         string
	Posts         int
	OpenQuestions int
}

// Summary converts a row to its wire form.
func (r ThreadRow) Summary() spec.ThreadSummary {
	return spec.ThreadSummary{
		ID: r.ID, Title: r.Title, Status: r.Status, Machine: r.MachineName, Agent: r.Agent,
		Posts: r.Posts, Open: r.OpenQuestions, UpdatedAt: kernel.Millis(r.UpdatedAt), Unread: r.Unread(),
	}
}
