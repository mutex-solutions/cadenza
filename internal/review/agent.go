package review

import (
	"context"
	"fmt"
	"time"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Show saves a post. Files must already be uploaded. Repeating a call with
// the same retry id returns the first result without a duplicate.
func (s *Service) Show(ctx context.Context, a Actor, in spec.ShowInput) (spec.ShowOutput, error) {
	if err := checkText("title", in.Title, true, MaxTitle); err != nil {
		return spec.ShowOutput{}, err
	}
	if err := checkText("summary", in.Summary, false, MaxSummary); err != nil {
		return spec.ShowOutput{}, err
	}
	if err := checkText("retry_id", in.RetryID, true, MaxRetryID); err != nil {
		return spec.ShowOutput{}, err
	}
	if len(in.Attachments) == 0 || len(in.Attachments) > MaxAttachments {
		return spec.ShowOutput{}, kernel.Invalid("a post needs 1 to %d attachments, got %d", MaxAttachments, len(in.Attachments)).
			WithFix("pass --template with --data, or one or more files")
	}
	if prev, err := s.store.GetPostByRetry(ctx, a.Owner, a.SessionID, in.RetryID); err == nil {
		return s.showOutput(ctx, a, prev)
	} else if !kernel.Is(err, kernel.KindNotFound) {
		return spec.ShowOutput{}, err
	}

	// Check every attachment and collect the files before opening a transaction.
	atts := make([]Attachment, len(in.Attachments))
	var refs []media.FileRef
	var question *TemplateQuestion
	for i, in := range in.Attachments {
		switch {
		case in.Template != "" && in.File != nil:
			return spec.ShowOutput{}, kernel.Invalid("attachment %d has both a template and a file", i+1).WithFix("send one or the other")
		case in.Template != "":
			c, err := s.templates.Check(ctx, a.Owner, in.Template, in.Version, in.Data)
			if err != nil {
				return spec.ShowOutput{}, err
			}
			atts[i] = Attachment{Index: i, Template: in.Template, Version: c.Version, Data: in.Data, View: c.View}
			for _, f := range c.Files {
				refs = append(refs, f)
				if f.PreviewSHA256 != "" {
					refs = append(refs, previewRef(f))
				}
			}
			if c.Question != nil {
				if question != nil {
					return spec.ShowOutput{}, kernel.Invalid("a post can ask only one question").WithFix("use one options attachment per post")
				}
				question = c.Question
			}
		case in.File != nil:
			if err := in.File.Validate(); err != nil {
				return spec.ShowOutput{}, err
			}
			f := *in.File
			atts[i] = Attachment{Index: i, File: &f}
			refs = append(refs, f)
			if f.PreviewSHA256 != "" {
				refs = append(refs, previewRef(f))
			}
		default:
			return spec.ShowOutput{}, kernel.Invalid("attachment %d is empty", i+1).WithFix("give it a template or a file")
		}
	}
	if err := s.files.Confirm(ctx, a.Owner, refs); err != nil {
		return spec.ShowOutput{}, err
	}

	now := s.clock.Now()
	post := &Post{ID: kernel.NewID("p"), Owner: a.Owner, Title: in.Title, Summary: in.Summary, SessionID: a.SessionID,
		RetryID: in.RetryID, CreatedAt: now, Attachments: atts}
	if in.Summary != "" {
		post.SummaryHTML = s.render.Render(in.Summary)
	}
	var out spec.ShowOutput
	err := s.tx(ctx, func(r Repo, rec *recorder) error {
		t, created, err := s.resolveThread(ctx, r, a, in.ThreadID, in.NewThread, in.Title)
		if err != nil {
			return err
		}
		if !created {
			if err := s.adopt(ctx, r, rec, a, t); err != nil {
				return err
			}
		}
		if post.Seq, err = t.AddPost(now); err != nil {
			return err
		}
		post.ThreadID = t.ID
		if created {
			if err := r.CreateThread(ctx, t); err != nil {
				return err
			}
			if _, err := rec.add(ctx, s.event(a, spec.EventThreadCreated, t.ID, t.ID, now)); err != nil {
				return err
			}
		} else if err := r.UpdateThread(ctx, t); err != nil {
			return err
		}
		if err := r.CreatePost(ctx, post); err != nil {
			return err
		}
		e := s.event(a, spec.EventPostAdded, t.ID, post.ID, now)
		if _, err := rec.add(ctx, e); err != nil {
			return err
		}
		out = spec.ShowOutput{ThreadID: t.ID, PostID: post.ID, URL: s.threadURL(t.ID)}
		if question != nil {
			q, err := s.newQuestion(ctx, r, rec, a, t, post.ID, question.Text, question.Choices, true, in.Notify, "post:"+in.RetryID, now)
			if err != nil {
				return err
			}
			out.QuestionID = q.ID
		}
		return nil
	})
	return out, err
}

func (s *Service) showOutput(ctx context.Context, a Actor, p *Post) (spec.ShowOutput, error) {
	out := spec.ShowOutput{ThreadID: p.ThreadID, PostID: p.ID, URL: s.threadURL(p.ThreadID)}
	q, err := s.store.GetQuestionByRetry(ctx, a.Owner, a.SessionID, "post:"+p.RetryID)
	if err == nil {
		out.QuestionID = q.ID
	} else if !kernel.Is(err, kernel.KindNotFound) {
		return out, err
	}
	return out, nil
}

// previewRef is the stored preview of an image. Its size is not in the post,
// so it is confirmed with size 0 (any size).
func previewRef(f media.FileRef) media.FileRef {
	return media.FileRef{SHA256: f.PreviewSHA256, Type: media.TypeWebP, Name: "preview-" + f.Name}
}

// resolveThread picks the thread for a post or question (plan gap G-5):
// an explicit thread id, a forced new thread, or the session's latest open
// thread, else a new one.
func (s *Service) resolveThread(ctx context.Context, r Repo, a Actor, threadID string, forceNew bool, title string) (*Thread, bool, error) {
	now := s.clock.Now()
	switch {
	case threadID != "" && forceNew:
		return nil, false, kernel.Invalid("thread_id and new_thread cannot be used together")
	case threadID != "":
		t, err := r.GetThread(ctx, a.Owner, threadID)
		if err != nil || !t.VisibleTo(a) {
			return nil, false, kernel.NotFound("thread %s not found on this machine", threadID).WithFix("run `cadenza threads` to list threads you can continue")
		}
		if !t.Open() {
			return nil, false, kernel.Conflict("thread %s is archived", threadID).WithFix("start a new thread with --new")
		}
		return t, false, nil
	case forceNew:
	default:
		t, err := r.LatestOpenThread(ctx, a.Owner, a.SessionID)
		if err == nil {
			return t, false, nil
		}
		if !kernel.Is(err, kernel.KindNotFound) {
			return nil, false, err
		}
	}
	if title == "" {
		return nil, false, kernel.Invalid("a new thread needs a title").WithFix("pass --title")
	}
	return NewThread(a, title, now), true, nil
}

// adopt hands a thread, and its undelivered replies, to the acting session.
func (s *Service) adopt(ctx context.Context, r Repo, rec *recorder, a Actor, t *Thread) error {
	if !t.Adopt(a) {
		return nil
	}
	return r.MoveUndelivered(ctx, a.Owner, t.ID, a.SessionID)
}

func (s *Service) event(a Actor, kind, threadID, ref string, at time.Time) eventlog.Event {
	return eventlog.Event{Owner: a.Owner, Kind: kind, ThreadID: threadID, RefID: ref, SessionID: a.SessionID, At: at}
}

// Ask creates a question and returns at once (plan D12).
func (s *Service) Ask(ctx context.Context, a Actor, in spec.AskInput) (spec.AskOutput, error) {
	if err := checkText("text", in.Text, true, MaxQuestion); err != nil {
		return spec.AskOutput{}, err
	}
	if err := checkText("retry_id", in.RetryID, true, MaxRetryID); err != nil {
		return spec.AskOutput{}, err
	}
	if len(in.Choices) > MaxChoices {
		return spec.AskOutput{}, kernel.Invalid("at most %d choices, got %d", MaxChoices, len(in.Choices))
	}
	seen := map[string]bool{}
	for _, c := range in.Choices {
		if err := checkText("choice", c, true, MaxChoice); err != nil {
			return spec.AskOutput{}, err
		}
		if seen[c] {
			return spec.AskOutput{}, kernel.Invalid("choice %q is listed twice", c)
		}
		seen[c] = true
	}
	if prev, err := s.store.GetQuestionByRetry(ctx, a.Owner, a.SessionID, in.RetryID); err == nil {
		return spec.AskOutput{QuestionID: prev.ID, ThreadID: prev.ThreadID, URL: s.threadURL(prev.ThreadID)}, nil
	} else if !kernel.Is(err, kernel.KindNotFound) {
		return spec.AskOutput{}, err
	}
	now := s.clock.Now()
	var out spec.AskOutput
	err := s.tx(ctx, func(r Repo, rec *recorder) error {
		threadID := in.ThreadID
		var replaced *Question
		if in.Replaces != "" {
			q, err := r.GetQuestion(ctx, a.Owner, in.Replaces)
			if err != nil {
				return kernel.NotFound("question %s not found", in.Replaces)
			}
			if threadID == "" {
				threadID = q.ThreadID
			} else if threadID != q.ThreadID {
				return kernel.Invalid("question %s is in another thread", in.Replaces)
			}
			replaced = q
		}
		if in.PostID != "" {
			p, err := r.GetPost(ctx, a.Owner, in.PostID)
			if err != nil {
				return kernel.NotFound("post %s not found", in.PostID)
			}
			if threadID == "" {
				threadID = p.ThreadID
			} else if threadID != p.ThreadID {
				return kernel.Invalid("post %s is in another thread", in.PostID)
			}
		}
		title := in.Title
		if title == "" {
			title = firstLine(in.Text, MaxTitle)
		}
		t, created, err := s.resolveThread(ctx, r, a, threadID, in.NewThread, title)
		if err != nil {
			return err
		}
		if created {
			if err := r.CreateThread(ctx, t); err != nil {
				return err
			}
			if _, err := rec.add(ctx, s.event(a, spec.EventThreadCreated, t.ID, t.ID, now)); err != nil {
				return err
			}
		} else if err := s.adopt(ctx, r, rec, a, t); err != nil {
			return err
		}
		t.Touch(now)
		if !created {
			if err := r.UpdateThread(ctx, t); err != nil {
				return err
			}
		}
		q, err := s.newQuestion(ctx, r, rec, a, t, in.PostID, in.Text, in.Choices, in.Waiting, in.Notify, in.RetryID, now)
		if err != nil {
			return err
		}
		if replaced != nil {
			if err := replaced.ReplaceWith(q.ID, now); err != nil {
				return err
			}
			if err := r.UpdateQuestion(ctx, replaced); err != nil {
				return err
			}
			if _, err := rec.add(ctx, s.event(a, spec.EventQuestionChanged, t.ID, replaced.ID, now)); err != nil {
				return err
			}
		}
		out = spec.AskOutput{QuestionID: q.ID, ThreadID: t.ID, URL: s.threadURL(t.ID)}
		return nil
	})
	return out, err
}

func (s *Service) newQuestion(ctx context.Context, r Repo, rec *recorder, a Actor, t *Thread, postID, text string, choices []string, waiting bool, notify, retryID string, at time.Time) (*Question, error) {
	q := &Question{ID: kernel.NewID("q"), Owner: a.Owner, ThreadID: t.ID, PostID: postID, SessionID: a.SessionID,
		Text: text, TextHTML: s.render.Render(text), Choices: choices, Waiting: waiting, NotifyTitle: notify,
		Status: spec.QuestionOpen, RetryID: retryID, CreatedAt: at}
	if err := r.CreateQuestion(ctx, q); err != nil {
		return nil, err
	}
	if _, err := rec.add(ctx, s.event(a, spec.EventQuestionAsked, t.ID, q.ID, at)); err != nil {
		return nil, err
	}
	return q, nil
}

// QuestionState is what `cadenza wait` reports. It does not block; the HTTP
// layer waits for events and calls it again.
func (s *Service) QuestionState(ctx context.Context, a Actor, questionID string) (spec.WaitOutput, *Question, error) {
	q, err := s.store.GetQuestion(ctx, a.Owner, questionID)
	if err != nil {
		return spec.WaitOutput{}, nil, err
	}
	t, err := s.store.GetThread(ctx, a.Owner, q.ThreadID)
	if err != nil {
		return spec.WaitOutput{}, nil, err
	}
	if !t.VisibleTo(a) {
		return spec.WaitOutput{}, nil, kernel.NotFound("question %s not found on this machine", questionID)
	}
	out := spec.WaitOutput{QuestionID: q.ID, Status: q.Status, ReplacedBy: q.ReplacedBy}
	if q.Status == spec.QuestionAnswered {
		rep, err := s.store.AnswerFor(ctx, a.Owner, q.ID)
		if err != nil {
			return out, q, err
		}
		resp, err := s.toResponse(ctx, a.Owner, rep, q)
		if err != nil {
			return out, q, err
		}
		out.Answer = &resp
	}
	return out, q, nil
}

// Responses lists what the owner sent since the session's last
// acknowledgement. Nothing is marked delivered until Ack (plan gap G-6).
func (s *Service) Responses(ctx context.Context, a Actor) (spec.ResponsesOutput, error) {
	cursor, err := s.store.GetCursor(ctx, a.SessionID)
	if err != nil {
		return spec.ResponsesOutput{}, err
	}
	reps, err := s.store.Undelivered(ctx, a.Owner, a.SessionID, cursor, 200)
	if err != nil {
		return spec.ResponsesOutput{}, err
	}
	out := spec.ResponsesOutput{Items: []spec.Response{}, Cursor: cursor}
	for i := range reps {
		var q *Question
		if reps[i].QuestionID != "" {
			if q, err = s.store.GetQuestion(ctx, a.Owner, reps[i].QuestionID); err != nil {
				return out, err
			}
		}
		resp, err := s.toResponse(ctx, a.Owner, &reps[i], q)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, resp)
		out.Cursor = reps[i].Seq
	}
	out.NextStep = NextStep(out.Items)
	return out, nil
}

// Ack marks responses up to cursor delivered. Acknowledging an older cursor
// than the current one is a no-op.
func (s *Service) Ack(ctx context.Context, a Actor, cursor int64) error {
	now := s.clock.Now()
	return s.tx(ctx, func(r Repo, rec *recorder) error {
		cur, err := r.GetCursor(ctx, a.SessionID)
		if err != nil {
			return err
		}
		if cursor <= cur {
			return nil
		}
		n, err := r.MarkDelivered(ctx, a.Owner, a.SessionID, cursor, now)
		if err != nil {
			return err
		}
		if err := r.SetCursor(ctx, a.SessionID, cursor); err != nil {
			return err
		}
		if n > 0 {
			_, err = rec.add(ctx, s.event(a, spec.EventReplyDelivered, "", fmt.Sprint(cursor), now))
		}
		return err
	})
}

// AgentReply lets the agent answer one of the owner's comments.
func (s *Service) AgentReply(ctx context.Context, a Actor, replyID, text string) error {
	if err := checkText("text", text, true, MaxReplyText); err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(r Repo, rec *recorder) error {
		rep, err := r.GetReply(ctx, a.Owner, replyID)
		if err != nil {
			return err
		}
		t, err := r.GetThread(ctx, a.Owner, rep.ThreadID)
		if err != nil {
			return err
		}
		if !t.VisibleTo(a) {
			return kernel.NotFound("reply %s not found on this machine", replyID)
		}
		if rep.Kind == spec.ReplyAnswer {
			return kernel.Invalid("reply %s is an answer to your question", replyID).WithFix("answer comments and notes only")
		}
		rep.AgentReply, rep.AgentReplyAt = text, &now
		if err := r.UpdateReply(ctx, rep); err != nil {
			return err
		}
		_, err = rec.add(ctx, s.event(a, spec.EventAgentReplied, t.ID, rep.ID, now))
		return err
	})
}

// LatestThread stands for the session's most recent open thread.
const LatestThread = "latest"

// Done marks a thread ready for review. An empty id or LatestThread means
// the session's most recent open thread.
func (s *Service) Done(ctx context.Context, a Actor, threadID, summary string) error {
	if err := checkText("summary", summary, false, MaxSummary); err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(r Repo, rec *recorder) error {
		var t *Thread
		var err error
		if threadID == "" || threadID == LatestThread {
			t, err = r.LatestOpenThread(ctx, a.Owner, a.SessionID)
			if kernel.Is(err, kernel.KindNotFound) {
				return kernel.NotFound("this session has no open thread").WithFix("pass --thread <id>; see `cadenza threads`")
			}
		} else {
			t, err = r.GetThread(ctx, a.Owner, threadID)
			if err == nil && !t.VisibleTo(a) {
				err = kernel.NotFound("thread %s not found on this machine", threadID)
			}
		}
		if err != nil {
			return err
		}
		if err := t.MarkReady(now); err != nil {
			return err
		}
		if err := r.UpdateThread(ctx, t); err != nil {
			return err
		}
		_, err = rec.add(ctx, s.event(a, spec.EventThreadReady, t.ID, t.ID, now))
		return err
	})
}

// AgentThreads lists open threads on the agent's machine.
func (s *Service) AgentThreads(ctx context.Context, a Actor) (spec.ThreadList, error) {
	rows, err := s.store.ListThreads(ctx, a.Owner, a.MachineID, false, 100)
	if err != nil {
		return spec.ThreadList{}, err
	}
	return toList(rows), nil
}

func toList(rows []ThreadRow) spec.ThreadList {
	out := spec.ThreadList{Threads: make([]spec.ThreadSummary, 0, len(rows))}
	for _, r := range rows {
		out.Threads = append(out.Threads, r.Summary())
	}
	return out
}

// toResponse converts a reply to what the agent receives, with signed links
// for its files.
func (s *Service) toResponse(ctx context.Context, owner kernel.OwnerID, r *Reply, q *Question) (spec.Response, error) {
	out := spec.Response{ID: r.ID, Kind: r.Kind, ThreadID: r.ThreadID, PostID: r.PostID, QuestionID: r.QuestionID,
		Choice: r.Choice, Text: r.Text, Verdict: r.Verdict, Spot: r.Spot, Extra: r.Extra, CreatedAt: kernel.Millis(r.CreatedAt)}
	if q != nil {
		out.Question = q.Text
	}
	for _, f := range r.Files {
		u, err := s.files.DownloadURL(ctx, owner, f.SHA256)
		if err != nil {
			return out, err
		}
		out.Files = append(out.Files, spec.ResponseFile{FileRef: f, URL: u.URL})
	}
	return out, nil
}

func firstLine(s string, max int) string {
	for i, c := range s {
		if c == '\n' {
			s = s[:i]
			break
		}
	}
	return clipRunes(s, max)
}

func clipRunes(s string, max int) string {
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
