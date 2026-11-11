package review

import (
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

var t0 = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func actor(session string) Actor {
	return Actor{Owner: "o_1", SessionID: session, MachineID: "m_1", MachineName: "vm-03", Agent: "claude-code"}
}

func TestThreadLifecycle(t *testing.T) {
	th := NewThread(actor("s_1"), "Checkout redesign", t0)
	if th.Status != spec.ThreadActive || th.NextSeq != 1 || !kernel.HasPrefix(th.ID, "t") {
		t.Fatalf("bad new thread %+v", th)
	}
	seq, err := th.AddPost(t0)
	if err != nil || seq != 1 || th.NextSeq != 2 {
		t.Fatalf("AddPost = %d, %v", seq, err)
	}
	if err := th.ApplyVerdict(spec.VerdictRequestChanges, t0); err != nil || th.Status != spec.ThreadChangesRequested {
		t.Fatalf("request changes: %v %s", err, th.Status)
	}
	if seq, _ := th.AddPost(t0.Add(time.Minute)); seq != 2 || th.Status != spec.ThreadActive {
		t.Fatalf("a new post must answer the change request: seq %d status %s", seq, th.Status)
	}
	if !th.UpdatedAt.Equal(t0.Add(time.Minute)) {
		t.Fatal("UpdatedAt not moved")
	}
	if err := th.MarkReady(t0); err != nil || th.Status != spec.ThreadReady {
		t.Fatal("MarkReady failed")
	}
	if err := th.ApplyVerdict(spec.VerdictApprove, t0); err != nil || th.Status != spec.ThreadApproved {
		t.Fatal("approve failed")
	}
	if err := th.ApplyVerdict("maybe", t0); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("unknown verdict accepted")
	}
	if err := th.SetStatus(spec.ThreadArchived, t0); err != nil || th.Open() {
		t.Fatal("archive failed")
	}
	if _, err := th.AddPost(t0); !kernel.Is(err, kernel.KindConflict) || !strings.Contains(err.Error(), "--new") {
		t.Fatalf("posting to an archived thread: %v", err)
	}
	if err := th.MarkReady(t0); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("ready on archived thread")
	}
	if err := th.ApplyVerdict(spec.VerdictApprove, t0); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("verdict on archived thread")
	}
	if err := th.SetStatus(spec.ThreadReady, t0); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("owner may not set ready")
	}
	if err := th.SetStatus(spec.ThreadActive, t0); err != nil || !th.Open() {
		t.Fatal("reopen failed")
	}
}

func TestThreadVisibilityAndAdoption(t *testing.T) {
	th := NewThread(actor("s_1"), "x", t0)
	other := actor("s_2")
	if !th.VisibleTo(other) {
		t.Fatal("same machine must see the thread")
	}
	elsewhere := other
	elsewhere.MachineID = "m_2"
	if th.VisibleTo(elsewhere) {
		t.Fatal("other machines must not see the thread")
	}
	stranger := other
	stranger.Owner = "o_2"
	if th.VisibleTo(stranger) {
		t.Fatal("other owners must not see the thread")
	}
	if th.Adopt(actor("s_1")) {
		t.Fatal("adopting your own thread is a no-op")
	}
	if !th.Adopt(other) || th.SessionID != "s_2" {
		t.Fatal("adopt failed")
	}
}

func TestQuestionLifecycle(t *testing.T) {
	newQ := func() *Question {
		return &Question{ID: "q_1", Status: spec.QuestionOpen, Choices: []string{"yes", "no"}}
	}
	q := newQ()
	if err := q.Answer("maybe", "", t0); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("answer outside choices accepted")
	}
	if err := q.Answer("", "", t0); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("empty answer accepted")
	}
	if err := q.Answer("yes", "and keep it", t0); err != nil || q.Status != spec.QuestionAnswered || q.ClosedAt == nil {
		t.Fatalf("answer failed: %v", err)
	}
	if err := q.Dismiss(t0); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("dismissing an answered question")
	}
	q = newQ()
	if err := q.Answer("", "free text works", t0); err != nil {
		t.Fatal(err)
	}
	q = newQ()
	if err := q.Dismiss(t0); err != nil || q.Status != spec.QuestionDismissed {
		t.Fatal("dismiss failed")
	}
	q = newQ()
	if err := q.ReplaceWith("q_2", t0); err != nil || q.Status != spec.QuestionReplaced || q.ReplacedBy != "q_2" {
		t.Fatal("replace failed")
	}
	if err := q.ReplaceWith("q_3", t0); !kernel.Is(err, kernel.KindConflict) || q.ReplacedBy != "q_2" {
		t.Fatal("replacing twice")
	}
}

func TestNextStep(t *testing.T) {
	cases := []struct {
		items []spec.Response
		want  string
	}{
		{nil, "Nothing new"},
		{[]spec.Response{{Kind: spec.ReplyAnswer}}, "Use the answers below and carry on. The owner sent 1 answer."},
		{[]spec.Response{{Kind: spec.ReplyComment}, {Kind: spec.ReplyComment}, {Kind: spec.ReplyVerdict, Verdict: spec.VerdictRequestChanges}},
			"Changes requested: address every comment and note below, then show a new version with `cadenza show`. The owner sent 2 comments."},
		{[]spec.Response{{Kind: spec.ReplyVerdict, Verdict: spec.VerdictApprove}}, "Approved: carry on with the next step."},
		{[]spec.Response{{Kind: spec.ReplyNote}, {Kind: spec.ReplySketch}, {Kind: spec.ReplySketch}, {Kind: spec.ReplyAnswer}},
			"Read each item below and act on it; reply to comments with `cadenza reply <id> <text>` when done. The owner sent 1 answer, 1 note and 2 sketches."},
		{[]spec.Response{{Kind: spec.ReplyVerdict, Verdict: spec.VerdictRequestChanges}, {Kind: spec.ReplyVerdict, Verdict: spec.VerdictApprove}},
			"Approved"},
	}
	for _, c := range cases {
		if got := NextStep(c.items); !strings.HasPrefix(got, c.want) {
			t.Errorf("NextStep = %q, want prefix %q", got, c.want)
		}
	}
}

func TestTextHelpers(t *testing.T) {
	if firstLine("Keep promo?\nmore detail", 100) != "Keep promo?" {
		t.Fatal("firstLine")
	}
	if clipRunes("héllo", 2) != "hé" || clipRunes("ab", 5) != "ab" {
		t.Fatal("clipRunes")
	}
	if err := checkText("title", " ", true, 10); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("blank required text accepted")
	}
	if err := checkText("title", strings.Repeat("é", 11), false, 10); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("long text accepted")
	}
	if err := checkText("title", "", false, 10); err != nil {
		t.Fatal(err)
	}
}

func TestThreadRowSummary(t *testing.T) {
	r := ThreadRow{Thread: Thread{ID: "t_1", Title: "x", Status: spec.ThreadActive, UpdatedAt: t0}, MachineName: "vm-03", Agent: "claude-code", Posts: 2, OpenQuestions: 1}
	s := r.Summary()
	if s.ID != "t_1" || s.Machine != "vm-03" || s.Posts != 2 || s.Open != 1 || s.UpdatedAt != t0.UnixMilli() {
		t.Fatalf("bad summary %+v", s)
	}
}
