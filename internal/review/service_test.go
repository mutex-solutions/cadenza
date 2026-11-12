package review_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

var ctx = context.Background()

type fixture struct {
	*testkit.Env
	owner kernel.OwnerID
	mk    string
	a     review.Actor
}

func setup(t *testing.T) *fixture {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	mk := e.Machine(t, "vm-03")
	_, a := e.Session(t, mk, "session-1")
	return &fixture{Env: e, owner: owner, mk: mk, a: a}
}

func (f *fixture) uiChange(t *testing.T) spec.Attachment {
	before := f.Upload("before.webp", media.TypeWebP, []byte("before-bytes"))
	after := f.Upload("after.webp", media.TypeWebP, []byte("after-bytes"))
	data, _ := json.Marshal(map[string]any{
		"summary": "Promo field below total; one step instead of two.",
		"before":  map[string]any{"file": before},
		"after":   map[string]any{"file": after},
		"changes": []string{"Promo field moved", "Step 2 removed"},
	})
	return spec.Attachment{Template: "ui-change", Data: data}
}

func TestShowCreatesThreadAndIsIdempotent(t *testing.T) {
	f := setup(t)
	sub := f.Bus.Subscribe(10)
	defer sub.Close()
	in := spec.ShowInput{Title: "Checkout redesign", Summary: "**Done**", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}}
	out, err := f.Review.Show(ctx, f.a, in)
	if err != nil {
		t.Fatal(err)
	}
	if !kernel.HasPrefix(out.ThreadID, "t") || !kernel.HasPrefix(out.PostID, "p") || out.URL != "https://cadenza.test/t/"+out.ThreadID {
		t.Fatalf("output %+v", out)
	}
	kinds := []string{(<-sub.C).Kind, (<-sub.C).Kind}
	if kinds[0] != spec.EventThreadCreated || kinds[1] != spec.EventPostAdded {
		t.Fatalf("events %v", kinds)
	}
	again, err := f.Review.Show(ctx, f.a, in)
	if err != nil || again != out {
		t.Fatalf("retry must return the same result: %+v %v", again, err)
	}
	v, err := f.Review.ThreadView(ctx, f.owner, out.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Posts) != 1 || v.Posts[0].SummaryHTML != "<p>**Done**</p>" || v.Thread.Machine != "vm-03" || v.Thread.Agent != "claude-code" {
		t.Fatalf("view %+v", v)
	}
	att := v.Posts[0].Attachments[0]
	if att.Template != "ui-change" || att.Version != 1 || !strings.Contains(string(att.Data), `"html"`) {
		t.Fatalf("attachment view %+v", att)
	}
}

func TestShowContinuesLatestThreadAndHonoursNew(t *testing.T) {
	f := setup(t)
	first, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Task A", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	second, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Round 2", RetryID: "r2", Attachments: []spec.Attachment{f.uiChange(t)}})
	if err != nil || second.ThreadID != first.ThreadID {
		t.Fatalf("second post should continue the thread: %+v %v", second, err)
	}
	third, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Task B", NewThread: true, RetryID: "r3", Attachments: []spec.Attachment{f.uiChange(t)}})
	if third.ThreadID == first.ThreadID {
		t.Fatal("--new must start a thread")
	}
	fourth, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "back", ThreadID: first.ThreadID, RetryID: "r4", Attachments: []spec.Attachment{f.uiChange(t)}})
	if fourth.ThreadID != first.ThreadID {
		t.Fatal("explicit thread id ignored")
	}
	v, _ := f.Review.ThreadView(ctx, f.owner, first.ThreadID)
	if len(v.Posts) != 3 || v.Posts[2].Seq != 3 || v.Thread.Title != "Task A" {
		t.Fatalf("posts %+v", v.Posts)
	}
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", ThreadID: first.ThreadID, NewThread: true, RetryID: "r5", Attachments: []spec.Attachment{f.uiChange(t)}}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("thread id with --new accepted")
	}
}

func TestShowValidation(t *testing.T) {
	f := setup(t)
	file := f.Upload("report.html", media.TypeHTML, []byte("<!doctype html><p>hi"))
	missing := media.FileRef{SHA256: strings.Repeat("c", 64), Size: 5, Type: media.TypePDF, Name: "x.pdf"}
	cases := map[string]spec.ShowInput{
		"no title":       {RetryID: "r", Attachments: []spec.Attachment{{File: &file}}},
		"no retry":       {Title: "x", Attachments: []spec.Attachment{{File: &file}}},
		"no attachments": {Title: "x", RetryID: "r"},
		"both kinds":     {Title: "x", RetryID: "r", Attachments: []spec.Attachment{{Template: "ui-change", File: &file}}},
		"empty":          {Title: "x", RetryID: "r", Attachments: []spec.Attachment{{}}},
		"bad template":   {Title: "x", RetryID: "r", Attachments: []spec.Attachment{{Template: "ui-change", Data: json.RawMessage(`{}`)}}},
		"not uploaded":   {Title: "x", RetryID: "r", Attachments: []spec.Attachment{{File: &missing}}},
		"long title":     {Title: strings.Repeat("x", review.MaxTitle+1), RetryID: "r", Attachments: []spec.Attachment{{File: &file}}},
	}
	for name, in := range cases {
		if _, err := f.Review.Show(ctx, f.a, in); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r", Attachments: []spec.Attachment{{Template: "nope", Data: json.RawMessage(`{}`)}}}); !kernel.Is(err, kernel.KindNotFound) {
		t.Errorf("unknown template: %v", err)
	}
	out, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Perf report", RetryID: "ok", Attachments: []spec.Attachment{{File: &file}}})
	if err != nil {
		t.Fatalf("plain file post: %v", err)
	}
	v, _ := f.Review.ThreadView(ctx, f.owner, out.ThreadID)
	if v.Posts[0].Attachments[0].File == nil || v.Posts[0].Attachments[0].File.Name != "report.html" {
		t.Fatal("file attachment lost")
	}
}

func TestOptionsTemplateAsksAQuestion(t *testing.T) {
	f := setup(t)
	data := json.RawMessage(`{"question":"Which layout?","options":[{"title":"Single column","text":"simpler"},{"title":"Two columns","text":"denser"}],"recommendation":{"tone":"info","text":"Single column"}}`)
	out, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Layout", RetryID: "r1", Attachments: []spec.Attachment{{Template: "options", Data: data}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.QuestionID == "" {
		t.Fatal("options post must create a question")
	}
	again, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Layout", RetryID: "r1", Attachments: []spec.Attachment{{Template: "options", Data: data}}})
	if again.QuestionID != out.QuestionID {
		t.Fatal("retry must return the same question")
	}
	st, _, _ := f.Review.QuestionState(ctx, f.a, out.QuestionID)
	if st.Status != spec.QuestionOpen {
		t.Fatal("question not open")
	}
	v, _ := f.Review.ThreadView(ctx, f.owner, out.ThreadID)
	if len(v.Questions) != 1 || len(v.Questions[0].Choices) != 2 || v.Questions[0].PostID != out.PostID {
		t.Fatalf("question %+v", v.Questions)
	}
}

func TestAskAnswerWaitResponsesAck(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Checkout", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	ask, err := f.Review.Ask(ctx, f.a, spec.AskInput{PostID: show.PostID, Text: "Keep promo visible?", Choices: []string{"yes", "no"}, Waiting: true, RetryID: "q1"})
	if err != nil || ask.ThreadID != show.ThreadID {
		t.Fatalf("ask: %+v %v", ask, err)
	}
	if again, _ := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "Keep promo visible?", RetryID: "q1"}); again.QuestionID != ask.QuestionID {
		t.Fatal("ask retry made a duplicate")
	}
	st, _, err := f.Review.QuestionState(ctx, f.a, ask.QuestionID)
	if err != nil || st.Status != spec.QuestionOpen || st.Answer != nil {
		t.Fatalf("open state %+v %v", st, err)
	}
	photo := f.Upload("photo.webp", media.TypeWebP, []byte("photo"))
	if _, err := f.Review.Answer(ctx, f.owner, ask.QuestionID, spec.AnswerInput{Choice: "maybe"}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("answer outside choices accepted")
	}
	if _, err := f.Review.Answer(ctx, f.owner, ask.QuestionID, spec.AnswerInput{Choice: "yes", Text: "and make it bold", Files: []media.FileRef{photo}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Review.Answer(ctx, f.owner, ask.QuestionID, spec.AnswerInput{Choice: "no"}); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("answering twice")
	}
	st, _, _ = f.Review.QuestionState(ctx, f.a, ask.QuestionID)
	if st.Status != spec.QuestionAnswered || st.Answer == nil || st.Answer.Choice != "yes" || st.Answer.Question != "Keep promo visible?" ||
		len(st.Answer.Files) != 1 || !strings.HasPrefix(st.Answer.Files[0].URL, "https://blobs.test/get/") {
		t.Fatalf("answered state %+v", st.Answer)
	}
	resp, err := f.Review.Responses(ctx, f.a)
	if err != nil || len(resp.Items) != 1 || resp.Items[0].Kind != spec.ReplyAnswer || !strings.HasPrefix(resp.NextStep, "Use the answers") {
		t.Fatalf("responses %+v %v", resp, err)
	}
	// Not acknowledged yet: still there.
	if again, _ := f.Review.Responses(ctx, f.a); len(again.Items) != 1 {
		t.Fatal("responses must stay until acknowledged")
	}
	if err := f.Review.Ack(ctx, f.a, resp.Cursor); err != nil {
		t.Fatal(err)
	}
	if err := f.Review.Ack(ctx, f.a, resp.Cursor-1); err != nil {
		t.Fatal("acknowledging an older cursor is a no-op")
	}
	after, _ := f.Review.Responses(ctx, f.a)
	if len(after.Items) != 0 || !strings.HasPrefix(after.NextStep, "Nothing new") {
		t.Fatalf("after ack %+v", after)
	}
	v, _ := f.Review.ThreadView(ctx, f.owner, show.ThreadID)
	if !v.Replies[0].Delivered {
		t.Fatal("reply not marked delivered")
	}
}

func TestReplaceAndDismiss(t *testing.T) {
	f := setup(t)
	q1, _ := f.Review.Ask(ctx, f.a, spec.AskInput{Title: "Task", Text: "Use Postgres?", RetryID: "q1"})
	q2, err := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "Use Postgres 17?", Replaces: q1.QuestionID, RetryID: "q2"})
	if err != nil || q2.ThreadID != q1.ThreadID {
		t.Fatalf("replace: %+v %v", q2, err)
	}
	st, _, _ := f.Review.QuestionState(ctx, f.a, q1.QuestionID)
	if st.Status != spec.QuestionReplaced || st.ReplacedBy != q2.QuestionID {
		t.Fatalf("replaced state %+v", st)
	}
	if err := f.Review.Dismiss(ctx, f.owner, q2.QuestionID); err != nil {
		t.Fatal(err)
	}
	st, _, _ = f.Review.QuestionState(ctx, f.a, q2.QuestionID)
	if st.Status != spec.QuestionDismissed {
		t.Fatal("dismiss lost")
	}
	if _, err := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "x", Replaces: q2.QuestionID, RetryID: "q3"}); !kernel.Is(err, kernel.KindConflict) {
		t.Fatalf("replacing a closed question: %v", err)
	}
	if _, err := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "x", Choices: []string{"a", "a"}, RetryID: "q4"}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("duplicate choices accepted")
	}
}

func TestOtherMachinesCannotSeeThreads(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	ask, _ := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "q?", RetryID: "q1"})
	otherMK := f.Machine(t, "vm-04")
	_, other := f.Session(t, otherMK, "session-x")
	if _, err := f.Review.Show(ctx, other, spec.ShowInput{Title: "x", ThreadID: show.ThreadID, RetryID: "r9", Attachments: []spec.Attachment{f.uiChange(t)}}); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("another machine posted to the thread")
	}
	if _, _, err := f.Review.QuestionState(ctx, other, ask.QuestionID); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("another machine read the question")
	}
	if err := f.Review.Done(ctx, other, show.ThreadID, ""); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("another machine finished the thread")
	}
	list, _ := f.Review.AgentThreads(ctx, other)
	if len(list.Threads) != 0 {
		t.Fatal("another machine listed the thread")
	}
}

func TestAdoptionMovesUndeliveredReplies(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	if _, err := f.Review.AddReply(ctx, f.owner, spec.ReplyInput{PostID: show.PostID, Kind: spec.ReplyNote, Text: "looks good"}); err != nil {
		t.Fatal(err)
	}
	// After /clear the agent has a new session id on the same machine.
	_, next := f.Session(t, f.mk, "session-2")
	list, _ := f.Review.AgentThreads(ctx, next)
	if len(list.Threads) != 1 {
		t.Fatal("a new session on the same machine must see open threads")
	}
	if r, _ := f.Review.Responses(ctx, next); len(r.Items) != 0 {
		t.Fatal("not adopted yet")
	}
	if _, err := f.Review.Show(ctx, next, spec.ShowInput{Title: "round 2", ThreadID: show.ThreadID, RetryID: "r2", Attachments: []spec.Attachment{f.uiChange(t)}}); err != nil {
		t.Fatal(err)
	}
	r, _ := f.Review.Responses(ctx, next)
	if len(r.Items) != 1 || r.Items[0].Text != "looks good" {
		t.Fatalf("adopting session must receive the undelivered note: %+v", r)
	}
	if old, _ := f.Review.Responses(ctx, f.a); len(old.Items) != 0 {
		t.Fatal("old session still receives the note")
	}
}

func TestVerdictsCommentsAndAgentReply(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	c, err := f.Review.AddReply(ctx, f.owner, spec.ReplyInput{PostID: show.PostID, Kind: spec.ReplyComment, Text: "move left", Spot: &spec.Spot{Attachment: 0, X: 0.2, Y: 0.4}})
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]spec.ReplyInput{
		"comment without spot": {PostID: show.PostID, Kind: spec.ReplyComment, Text: "x"},
		"spot out of range":    {PostID: show.PostID, Kind: spec.ReplyComment, Text: "x", Spot: &spec.Spot{Attachment: 5}},
		"empty note":           {PostID: show.PostID, Kind: spec.ReplyNote},
		"bad verdict":          {PostID: show.PostID, Kind: spec.ReplyVerdict, Verdict: "meh"},
		"bad kind":             {PostID: show.PostID, Kind: spec.ReplyAnswer, Text: "x"},
		"bad extra":            {PostID: show.PostID, Kind: spec.ReplyNote, Text: "x", Extra: json.RawMessage(`{`)},
	}
	for name, in := range bad {
		if _, err := f.Review.AddReply(ctx, f.owner, in); !kernel.Is(err, kernel.KindInvalid) {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
	if _, err := f.Review.AddReply(ctx, f.owner, spec.ReplyInput{PostID: show.PostID, Kind: spec.ReplyVerdict, Verdict: spec.VerdictRequestChanges}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.Review.ThreadView(ctx, f.owner, show.ThreadID)
	if v.Thread.Status != spec.ThreadChangesRequested {
		t.Fatalf("status %s", v.Thread.Status)
	}
	resp, _ := f.Review.Responses(ctx, f.a)
	if len(resp.Items) != 2 || !strings.HasPrefix(resp.NextStep, "Changes requested") || resp.Items[0].Spot == nil {
		t.Fatalf("responses %+v", resp)
	}
	if err := f.Review.AgentReply(ctx, f.a, c.ReplyID, "fixed in round 2"); err != nil {
		t.Fatal(err)
	}
	if err := f.Review.AgentReply(ctx, f.a, c.ReplyID, ""); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("empty agent reply accepted")
	}
	f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Round 2", RetryID: "r2", Attachments: []spec.Attachment{f.uiChange(t)}})
	v, _ = f.Review.ThreadView(ctx, f.owner, show.ThreadID)
	if v.Thread.Status != spec.ThreadActive || v.Replies[0].AgentReply != "fixed in round 2" {
		t.Fatalf("after round 2: %s %+v", v.Thread.Status, v.Replies[0])
	}
	if err := f.Review.Done(ctx, f.a, "", "all fixed"); err != nil { // the session's latest thread
		t.Fatal(err)
	}
	if err := f.Review.SetThreadStatus(ctx, f.owner, show.ThreadID, spec.ThreadArchived); err != nil {
		t.Fatal(err)
	}
	inbox, _ := f.Review.Inbox(ctx, f.owner, false)
	all, _ := f.Review.Inbox(ctx, f.owner, true)
	if len(inbox.Threads) != 0 || len(all.Threads) != 1 || all.Threads[0].Status != spec.ThreadArchived {
		t.Fatal("archive")
	}
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", ThreadID: show.ThreadID, RetryID: "r3", Attachments: []spec.Attachment{f.uiChange(t)}}); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("posting to an archived thread")
	}
	// Without an explicit thread, the agent starts a new one.
	next, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Next task", RetryID: "r4", Attachments: []spec.Attachment{f.uiChange(t)}})
	if err != nil || next.ThreadID == show.ThreadID {
		t.Fatal("archived thread must not be continued implicitly")
	}
}

func TestMarkRead(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	inbox, _ := f.Review.Inbox(ctx, f.owner, false)
	if !inbox.Threads[0].Unread {
		t.Fatal("new thread must be unread")
	}
	f.Clock.Advance(time.Second)
	if err := f.Review.MarkRead(ctx, f.owner, show.ThreadID); err != nil {
		t.Fatal(err)
	}
	inbox, _ = f.Review.Inbox(ctx, f.owner, false)
	if inbox.Threads[0].Unread {
		t.Fatal("marked read")
	}
	f.Clock.Advance(time.Second)
	f.Review.Ask(ctx, f.a, spec.AskInput{Text: "q?", RetryID: "q1"})
	inbox, _ = f.Review.Inbox(ctx, f.owner, false)
	if !inbox.Threads[0].Unread {
		t.Fatal("a new question makes it unread again")
	}
	if err := f.Review.MarkRead(ctx, f.owner, "t_missing"); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("unknown thread")
	}
}

func TestDescribe(t *testing.T) {
	f := setup(t)
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "Checkout redesign", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	ask, _ := f.Review.Ask(ctx, f.a, spec.AskInput{Text: "Keep promo?", Notify: "keep promo field visible?", Waiting: true, RetryID: "q1"})
	info, err := f.Review.Describe(ctx, eventlog.Event{Owner: f.owner, Kind: spec.EventQuestionAsked, ThreadID: ask.ThreadID, RefID: ask.QuestionID})
	if err != nil || info.Agent != "Claude Code" || info.Machine != "vm-03" || info.Title != "keep promo field visible?" || !info.Waiting {
		t.Fatalf("describe question %+v %v", info, err)
	}
	info, _ = f.Review.Describe(ctx, eventlog.Event{Owner: f.owner, Kind: spec.EventPostAdded, ThreadID: show.ThreadID, RefID: show.PostID})
	if !info.FirstPost || info.Title != "Checkout redesign" {
		t.Fatalf("describe post %+v", info)
	}
}

func TestStorageDownFailsClearly(t *testing.T) {
	f := setup(t)
	att := f.uiChange(t)
	f.Blobs.Down = true
	f.Clock.Advance(time.Second)
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{att}}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
}

func TestTemplateImagePreviewsAreRecorded(t *testing.T) {
	f := setup(t)
	preview := f.Upload("p.webp", media.TypeWebP, []byte("preview"))
	after := f.Upload("after.webp", media.TypeWebP, []byte("after"))
	after.PreviewSHA256 = preview.SHA256
	data, _ := json.Marshal(map[string]any{"summary": "x", "after": map[string]any{"file": after}})
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{{Template: "ui-change", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Files.DownloadURL(ctx, f.owner, preview.SHA256); err != nil {
		t.Fatalf("the preview of a template image must be downloadable: %v", err)
	}
	missing := after
	missing.PreviewSHA256 = strings.Repeat("e", 64)
	data, _ = json.Marshal(map[string]any{"summary": "x", "after": map[string]any{"file": missing}})
	if _, err := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r2", Attachments: []spec.Attachment{{Template: "ui-change", Data: data}}}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatalf("a missing preview must be refused: %v", err)
	}
}

func TestAckSpecificReplies(t *testing.T) {
	f := setup(t)
	ask, _ := f.Review.Ask(ctx, f.a, spec.AskInput{Title: "T", Text: "q?", Choices: []string{"a", "b"}, RetryID: "q1"})
	out, _ := f.Review.Answer(ctx, f.owner, ask.QuestionID, spec.AnswerInput{Choice: "a"})
	show, _ := f.Review.Show(ctx, f.a, spec.ShowInput{Title: "x", RetryID: "r1", Attachments: []spec.Attachment{f.uiChange(t)}})
	f.Review.AddReply(ctx, f.owner, spec.ReplyInput{PostID: show.PostID, Kind: spec.ReplyNote, Text: "later note"})
	_, other := f.Session(t, f.mk, "session-other")
	if err := f.Review.AckReplies(ctx, other, 0, []string{out.ReplyID}); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatalf("another session acknowledged the reply: %v", err)
	}
	if err := f.Review.AckReplies(ctx, f.a, 0, []string{out.ReplyID}); err != nil {
		t.Fatal(err)
	}
	if err := f.Review.AckReplies(ctx, f.a, 0, []string{out.ReplyID}); err != nil {
		t.Fatal("acknowledging twice is fine")
	}
	resp, _ := f.Review.Responses(ctx, f.a)
	if len(resp.Items) != 1 || resp.Items[0].Text != "later note" {
		t.Fatalf("only the unacknowledged note should remain: %+v", resp.Items)
	}
	if err := f.Review.AckReplies(ctx, f.a, 0, make([]string, 201)); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("too many ids accepted")
	}
}
