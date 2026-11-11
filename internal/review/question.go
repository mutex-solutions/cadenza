package review

import (
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Question is an agent asking the owner. Lifecycle (plan §9):
//
//	open ──(owner answers)──► answered
//	open ──(agent asks a newer one)──► replaced
//	open ──(owner dismisses)──► dismissed
type Question struct {
	ID          string
	Owner       kernel.OwnerID
	ThreadID    string
	PostID      string
	SessionID   string
	Text        string
	TextHTML    string
	Choices     []string
	Waiting     bool
	NotifyTitle string
	Status      string
	ReplacedBy  string
	RetryID     string
	CreatedAt   time.Time
	ClosedAt    *time.Time
}

func (q *Question) Open() bool { return q.Status == spec.QuestionOpen }

func (q *Question) close(status string, now time.Time) error {
	if !q.Open() {
		return kernel.Conflict("question %s is already %s", q.ID, q.Status)
	}
	q.Status = status
	q.ClosedAt = &now
	return nil
}

// Answer closes the question with an answer. A choice, when given, must be
// one of the question's choices.
func (q *Question) Answer(choice, text string, now time.Time) error {
	if choice != "" {
		found := false
		for _, c := range q.Choices {
			if c == choice {
				found = true
				break
			}
		}
		if !found {
			return kernel.Invalid("%q is not one of the choices", choice)
		}
	}
	if choice == "" && text == "" {
		return kernel.Invalid("an answer needs a choice or some text")
	}
	return q.close(spec.QuestionAnswered, now)
}

// Dismiss closes the question without an answer.
func (q *Question) Dismiss(now time.Time) error { return q.close(spec.QuestionDismissed, now) }

// ReplaceWith closes the question because the agent asked a newer one.
func (q *Question) ReplaceWith(newID string, now time.Time) error {
	if err := q.close(spec.QuestionReplaced, now); err != nil {
		return err
	}
	q.ReplacedBy = newID
	return nil
}
