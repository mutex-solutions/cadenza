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

// Inbox lists the owner's threads across all machines, newest change first.
func (s *Service) Inbox(ctx context.Context, owner kernel.OwnerID, includeArchived bool) (spec.ThreadList, error) {
	rows, err := s.store.ListThreads(ctx, owner, "", includeArchived, 500)
	if err != nil {
		return spec.ThreadList{}, err
	}
	return toList(rows), nil
}

// ThreadView is one thread with everything in it.
func (s *Service) ThreadView(ctx context.Context, owner kernel.OwnerID, id string) (spec.ThreadView, error) {
	row, err := s.store.GetThreadRow(ctx, owner, id)
	if err != nil {
		return spec.ThreadView{}, err
	}
	posts, err := s.store.ListPosts(ctx, owner, id)
	if err != nil {
		return spec.ThreadView{}, err
	}
	qs, err := s.store.ListQuestions(ctx, owner, id)
	if err != nil {
		return spec.ThreadView{}, err
	}
	reps, err := s.store.ListReplies(ctx, owner, id)
	if err != nil {
		return spec.ThreadView{}, err
	}
	v := spec.ThreadView{Thread: row.Summary(), Posts: []spec.PostView{}, Questions: []spec.QuestionView{}, Replies: []spec.ReplyView{}}
	for _, p := range posts {
		pv := spec.PostView{ID: p.ID, Seq: p.Seq, Title: p.Title, SummaryHTML: p.SummaryHTML, CreatedAt: kernel.Millis(p.CreatedAt),
			Attachments: make([]spec.AttachmentView, 0, len(p.Attachments))}
		for _, a := range p.Attachments {
			pv.Attachments = append(pv.Attachments, spec.AttachmentView{Index: a.Index, Template: a.Template, Version: a.Version, Data: a.View, File: a.File})
		}
		v.Posts = append(v.Posts, pv)
	}
	for _, q := range qs {
		v.Questions = append(v.Questions, spec.QuestionView{ID: q.ID, PostID: q.PostID, TextHTML: q.TextHTML, Choices: q.Choices,
			Waiting: q.Waiting, Status: q.Status, ReplacedBy: q.ReplacedBy, CreatedAt: kernel.Millis(q.CreatedAt)})
	}
	for i := range reps {
		r := &reps[i]
		rv := spec.ReplyView{Response: spec.Response{ID: r.ID, Kind: r.Kind, ThreadID: r.ThreadID, PostID: r.PostID,
			QuestionID: r.QuestionID, Choice: r.Choice, Text: r.Text, Verdict: r.Verdict, Spot: r.Spot, Extra: r.Extra,
			CreatedAt: kernel.Millis(r.CreatedAt)}, AgentReply: r.AgentReply, Delivered: r.DeliveredAt != nil}
		for _, f := range r.Files {
			rv.Files = append(rv.Files, spec.ResponseFile{FileRef: f})
		}
		if r.AgentReplyAt != nil {
			rv.AgentReplyAt = kernel.Millis(*r.AgentReplyAt)
		}
		v.Replies = append(v.Replies, rv)
	}
	return v, nil
}

// Answer answers an open question.
func (s *Service) Answer(ctx context.Context, owner kernel.OwnerID, questionID string, in spec.AnswerInput) (spec.ReplyOutput, error) {
	if err := s.checkReplyBody(ctx, owner, in.Text, in.Files); err != nil {
		return spec.ReplyOutput{}, err
	}
	now := s.clock.Now()
	var out spec.ReplyOutput
	err := s.tx(ctx, func(r Repo, rec *recorder) error {
		q, err := r.GetQuestion(ctx, owner, questionID)
		if err != nil {
			return err
		}
		if err := q.Answer(in.Choice, in.Text, now); err != nil {
			return err
		}
		if err := r.UpdateQuestion(ctx, q); err != nil {
			return err
		}
		t, err := r.GetThread(ctx, owner, q.ThreadID)
		if err != nil {
			return err
		}
		rep := &Reply{ID: kernel.NewID("r"), Owner: owner, ThreadID: q.ThreadID, PostID: q.PostID, QuestionID: q.ID,
			SessionID: t.SessionID, Kind: spec.ReplyAnswer, Choice: in.Choice, Text: in.Text, Files: in.Files, CreatedAt: now}
		if err := s.saveReply(ctx, r, rec, t, rep, now); err != nil {
			return err
		}
		if _, err := rec.add(ctx, ownerEvent(owner, spec.EventQuestionChanged, t, q.ID, now)); err != nil {
			return err
		}
		out.ReplyID = rep.ID
		return nil
	})
	return out, err
}

// Dismiss closes a question without answering it.
func (s *Service) Dismiss(ctx context.Context, owner kernel.OwnerID, questionID string) error {
	now := s.clock.Now()
	return s.tx(ctx, func(r Repo, rec *recorder) error {
		q, err := r.GetQuestion(ctx, owner, questionID)
		if err != nil {
			return err
		}
		if err := q.Dismiss(now); err != nil {
			return err
		}
		if err := r.UpdateQuestion(ctx, q); err != nil {
			return err
		}
		t, err := r.GetThread(ctx, owner, q.ThreadID)
		if err != nil {
			return err
		}
		_, err = rec.add(ctx, ownerEvent(owner, spec.EventQuestionChanged, t, q.ID, now))
		return err
	})
}

// AddReply is a note, verdict, pinned comment or sketch on a post.
func (s *Service) AddReply(ctx context.Context, owner kernel.OwnerID, in spec.ReplyInput) (spec.ReplyOutput, error) {
	switch in.Kind {
	case spec.ReplyNote, spec.ReplySketch:
		if in.Text == "" && len(in.Files) == 0 {
			return spec.ReplyOutput{}, kernel.Invalid("a %s needs text or files", in.Kind)
		}
	case spec.ReplyComment:
		if in.Text == "" {
			return spec.ReplyOutput{}, kernel.Invalid("a comment needs text")
		}
		if in.Spot == nil {
			return spec.ReplyOutput{}, kernel.Invalid("a comment needs a spot").WithFix("send a note for a comment on the whole post")
		}
	case spec.ReplyVerdict:
		if in.Verdict != spec.VerdictApprove && in.Verdict != spec.VerdictRequestChanges {
			return spec.ReplyOutput{}, kernel.Invalid("unknown verdict %q", in.Verdict)
		}
	default:
		return spec.ReplyOutput{}, kernel.Invalid("unknown reply kind %q", in.Kind).
			WithFix("use note, comment, verdict or sketch; answer questions with the answer call")
	}
	if len(in.Extra) > 0 && !json.Valid(in.Extra) {
		return spec.ReplyOutput{}, kernel.Invalid("extra is not valid JSON")
	}
	if len(in.Extra) > 64<<10 {
		return spec.ReplyOutput{}, kernel.Invalid("extra is larger than 64 KB")
	}
	if err := s.checkReplyBody(ctx, owner, in.Text, in.Files); err != nil {
		return spec.ReplyOutput{}, err
	}
	now := s.clock.Now()
	var out spec.ReplyOutput
	err := s.tx(ctx, func(r Repo, rec *recorder) error {
		p, err := r.GetPost(ctx, owner, in.PostID)
		if err != nil {
			return err
		}
		if in.Spot != nil && (in.Spot.Attachment < 0 || in.Spot.Attachment >= len(p.Attachments)) {
			return kernel.Invalid("spot points at attachment %d; the post has %d", in.Spot.Attachment, len(p.Attachments))
		}
		t, err := r.GetThread(ctx, owner, p.ThreadID)
		if err != nil {
			return err
		}
		if in.Kind == spec.ReplyVerdict {
			if err := t.ApplyVerdict(in.Verdict, now); err != nil {
				return err
			}
			if err := r.UpdateThread(ctx, t); err != nil {
				return err
			}
			if _, err := rec.add(ctx, ownerEvent(owner, spec.EventThreadStatus, t, t.ID, now)); err != nil {
				return err
			}
		}
		rep := &Reply{ID: kernel.NewID("r"), Owner: owner, ThreadID: t.ID, PostID: p.ID, SessionID: t.SessionID,
			Kind: in.Kind, Text: in.Text, Verdict: in.Verdict, Spot: in.Spot, Files: in.Files, Extra: in.Extra, CreatedAt: now}
		if err := s.saveReply(ctx, r, rec, t, rep, now); err != nil {
			return err
		}
		out.ReplyID = rep.ID
		return nil
	})
	return out, err
}

// MarkRead marks a thread read by the owner. It records no event: other
// devices learn of it on their next reload.
func (s *Service) MarkRead(ctx context.Context, owner kernel.OwnerID, id string) error {
	return s.store.InTx(ctx, func(r Repo) error {
		t, err := r.GetThread(ctx, owner, id)
		if err != nil {
			return err
		}
		t.MarkRead(s.clock.Now())
		return r.UpdateThread(ctx, t)
	})
}

// SetThreadStatus approves, archives or reopens a thread.
func (s *Service) SetThreadStatus(ctx context.Context, owner kernel.OwnerID, id, status string) error {
	now := s.clock.Now()
	return s.tx(ctx, func(r Repo, rec *recorder) error {
		t, err := r.GetThread(ctx, owner, id)
		if err != nil {
			return err
		}
		if err := t.SetStatus(status, now); err != nil {
			return err
		}
		if err := r.UpdateThread(ctx, t); err != nil {
			return err
		}
		_, err = rec.add(ctx, ownerEvent(owner, spec.EventThreadStatus, t, t.ID, now))
		return err
	})
}

func (s *Service) checkReplyBody(ctx context.Context, owner kernel.OwnerID, text string, files []media.FileRef) error {
	if err := checkText("text", text, false, MaxReplyText); err != nil {
		return err
	}
	if len(files) > MaxReplyFiles {
		return kernel.Invalid("at most %d files per reply", MaxReplyFiles)
	}
	for _, f := range files {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	return s.files.Confirm(ctx, owner, files)
}

// saveReply numbers a reply with its event and stores it.
func (s *Service) saveReply(ctx context.Context, r Repo, rec *recorder, t *Thread, rep *Reply, now time.Time) error {
	seq, err := rec.add(ctx, ownerEvent(rep.Owner, spec.EventReplyAdded, t, rep.ID, now))
	if err != nil {
		return err
	}
	rep.Seq = seq
	return r.CreateReply(ctx, rep)
}

func ownerEvent(owner kernel.OwnerID, kind string, t *Thread, ref string, at time.Time) eventlog.Event {
	return eventlog.Event{Owner: owner, Kind: kind, ThreadID: t.ID, RefID: ref, SessionID: t.SessionID, At: at}
}
