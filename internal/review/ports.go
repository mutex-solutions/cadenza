package review

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Repo is the storage review needs. Get* methods return a kernel NotFound
// error when nothing matches.
type Repo interface {
	CreateThread(ctx context.Context, t *Thread) error
	GetThread(ctx context.Context, owner kernel.OwnerID, id string) (*Thread, error)
	UpdateThread(ctx context.Context, t *Thread) error
	// LatestOpenThread is the session's most recently updated open thread.
	LatestOpenThread(ctx context.Context, owner kernel.OwnerID, sessionID string) (*Thread, error)
	GetThreadRow(ctx context.Context, owner kernel.OwnerID, id string) (*ThreadRow, error)
	// ListThreads lists threads, newest change first. An empty machineID means
	// all machines; archived threads are included only when asked.
	ListThreads(ctx context.Context, owner kernel.OwnerID, machineID string, includeArchived bool, limit int) ([]ThreadRow, error)

	CreatePost(ctx context.Context, p *Post) error // with its attachments
	GetPostByRetry(ctx context.Context, owner kernel.OwnerID, sessionID, retryID string) (*Post, error)
	GetPost(ctx context.Context, owner kernel.OwnerID, id string) (*Post, error)
	ListPosts(ctx context.Context, owner kernel.OwnerID, threadID string) ([]Post, error)

	CreateQuestion(ctx context.Context, q *Question) error
	GetQuestion(ctx context.Context, owner kernel.OwnerID, id string) (*Question, error)
	GetQuestionByRetry(ctx context.Context, owner kernel.OwnerID, sessionID, retryID string) (*Question, error)
	UpdateQuestion(ctx context.Context, q *Question) error
	ListQuestions(ctx context.Context, owner kernel.OwnerID, threadID string) ([]Question, error)

	CreateReply(ctx context.Context, r *Reply) error
	GetReply(ctx context.Context, owner kernel.OwnerID, id string) (*Reply, error)
	UpdateReply(ctx context.Context, r *Reply) error
	ListReplies(ctx context.Context, owner kernel.OwnerID, threadID string) ([]Reply, error)
	// AnswerFor is the answer reply to a question.
	AnswerFor(ctx context.Context, owner kernel.OwnerID, questionID string) (*Reply, error)
	// Undelivered lists the session's replies with Seq > after, oldest first.
	Undelivered(ctx context.Context, owner kernel.OwnerID, sessionID string, after int64, limit int) ([]Reply, error)
	// MarkDelivered sets DeliveredAt on the session's replies with Seq <= upTo.
	MarkDelivered(ctx context.Context, owner kernel.OwnerID, sessionID string, upTo int64, at time.Time) (int, error)
	// MoveUndelivered hands a thread's undelivered replies to another session.
	MoveUndelivered(ctx context.Context, owner kernel.OwnerID, threadID, toSessionID string) error

	GetCursor(ctx context.Context, sessionID string) (int64, error) // 0 when unset
	SetCursor(ctx context.Context, sessionID string, seq int64) error

	AppendEvent(ctx context.Context, e *eventlog.Event) error
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

// Files is what review needs from file storage.
type Files interface {
	// Confirm checks that every file's bytes are stored, recording them for
	// the owner. A post is saved only after this passes.
	Confirm(ctx context.Context, owner kernel.OwnerID, refs []media.FileRef) error
	// DownloadURL signs a link to a stored file.
	DownloadURL(ctx context.Context, owner kernel.OwnerID, sha256 string) (spec.FileURL, error)
}

// Templates checks filled templates.
type Templates interface {
	Check(ctx context.Context, owner kernel.OwnerID, name string, version int, data json.RawMessage) (Checked, error)
}

// Checked is a validated, rendered template attachment.
type Checked struct {
	Version int
	Files   []media.FileRef
	View    json.RawMessage
	// Question is set when the template asks something (the options template).
	Question *TemplateQuestion
}

type TemplateQuestion struct {
	Text    string
	Choices []string
}
