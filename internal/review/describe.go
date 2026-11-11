package review

import (
	"context"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// EventInfo is what a notification about an event needs: who and where, and
// the short title the agent chose. Never the work itself (plan §11).
type EventInfo struct {
	ThreadID     string
	ThreadStatus string
	Agent        string // display name, e.g. "Claude Code"
	Machine      string
	Title        string // the agent's notify title, post title or thread title
	Waiting      bool   // a question the agent is blocked on
	FirstPost    bool
}

// Describe looks up what an event is about.
func (s *Service) Describe(ctx context.Context, e eventlog.Event) (EventInfo, error) {
	row, err := s.store.GetThreadRow(ctx, e.Owner, e.ThreadID)
	if err != nil {
		return EventInfo{}, err
	}
	info := EventInfo{ThreadID: row.ID, ThreadStatus: row.Status, Agent: access.AgentLabel(row.Agent), Machine: row.MachineName, Title: row.Title}
	switch e.Kind {
	case spec.EventQuestionAsked:
		q, err := s.store.GetQuestion(ctx, e.Owner, e.RefID)
		if err != nil {
			return info, err
		}
		info.Waiting = q.Waiting
		if q.NotifyTitle != "" {
			info.Title = q.NotifyTitle
		}
	case spec.EventPostAdded:
		p, err := s.store.GetPost(ctx, e.Owner, e.RefID)
		if err != nil {
			return info, err
		}
		info.FirstPost = p.Seq == 1
		if p.Title != "" {
			info.Title = p.Title
		}
	}
	return info, nil
}
