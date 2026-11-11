package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

var ctx = context.Background()

func setup(t *testing.T) (*testkit.Env, kernel.OwnerID, review.Actor) {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	mk := e.Machine(t, "vm-03")
	_, a := e.Session(t, mk, "s1")
	if _, err := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "webpush", Token: json.RawMessage(`{"endpoint":"https://push.test/1"}`), Name: "Laptop"}); err != nil {
		t.Fatal(err)
	}
	return e, owner, a
}

func file(e *testkit.Env) spec.Attachment {
	ref := e.Upload("r.html", media.TypeHTML, []byte("<p>x"))
	return spec.Attachment{File: &ref}
}

func TestTitleFormat(t *testing.T) {
	if got := notify.Title("Claude Code", "vm-03", "Checkout redesign: keep promo field visible?"); got != "Claude Code · vm-03 — Checkout redesign: keep promo field visible?" {
		t.Fatalf("title %q", got)
	}
	if got := notify.Title("Hermes", "box", ""); got != "Hermes · box" {
		t.Fatalf("title %q", got)
	}
	long := notify.Title("A", "b", strings.Repeat("x", 300))
	if len([]rune(long)) != 120 || !strings.HasSuffix(long, "…") {
		t.Fatalf("long titles are cut to 120 runes: %d", len([]rune(long)))
	}
}

func TestRegisterValidatesAndUpserts(t *testing.T) {
	e, owner, _ := setup(t)
	if _, err := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "expo", Token: json.RawMessage(`"x"`)}); !kernel.Is(err, kernel.KindInvalid) ||
		!strings.Contains(err.Error(), "webpush") {
		t.Fatalf("disabled provider: %v", err)
	}
	if _, err := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "webpush", Token: json.RawMessage(`{`)}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("bad token accepted")
	}
	tok := json.RawMessage(`{"endpoint":"https://push.test/1"}`)
	a, _ := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "webpush", Token: tok, Name: "Laptop again"})
	b, _ := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "webpush", Token: tok})
	if a.DeviceID != b.DeviceID {
		t.Fatal("same token must reuse the device")
	}
	if err := e.Notify.Remove(ctx, owner, a.DeviceID); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.Notify.Test(ctx, owner); n != 0 {
		t.Fatal("removed device notified")
	}
	if c, _ := e.Notify.Register(ctx, owner, spec.DeviceInput{Provider: "webpush", Token: tok}); c.DeviceID != a.DeviceID {
		t.Fatal("re-registering must re-enable the same device")
	}
	if n, _ := e.Notify.Test(ctx, owner); n != 1 {
		t.Fatal("re-enabled device not notified")
	}
}

func TestConfig(t *testing.T) {
	e, _, _ := setup(t)
	c := e.Notify.Config()
	if len(c.Providers) != 1 || c.Providers[0] != "webpush" || c.VAPIDPublicKey != "vapid-public" {
		t.Fatalf("config %+v", c)
	}
}

func TestQuestionNotifiesWithAgentMachineAndTitle(t *testing.T) {
	e, _, a := setup(t)
	sub := e.Bus.Subscribe(20)
	defer sub.Close()
	e.Review.Show(ctx, a, spec.ShowInput{Title: "Checkout redesign", RetryID: "r1", Attachments: []spec.Attachment{file(e)}})
	e.Review.Ask(ctx, a, spec.AskInput{Text: "Keep promo?", Notify: "keep promo field visible?", Waiting: true, RetryID: "q1"})
	drain(t, e, sub)
	msgs := e.Push.Messages()
	if len(msgs) != 2 {
		t.Fatalf("want post + question notifications, got %d: %+v", len(msgs), msgs)
	}
	q := msgs[1]
	if q.Title != "Claude Code · vm-03 — keep promo field visible?" || q.Body != "Is waiting for your answer." || !strings.HasPrefix(q.URL, "/t/t_") {
		t.Fatalf("question notification %+v", q)
	}
}

func TestPostCooldownAndFinished(t *testing.T) {
	e, owner, a := setup(t)
	sub := e.Bus.Subscribe(50)
	defer sub.Close()
	show, _ := e.Review.Show(ctx, a, spec.ShowInput{Title: "Task", RetryID: "r1", Attachments: []spec.Attachment{file(e)}})
	e.Review.Show(ctx, a, spec.ShowInput{Title: "progress", RetryID: "r2", Attachments: []spec.Attachment{file(e)}})
	drain(t, e, sub)
	if n := len(e.Push.Messages()); n != 1 {
		t.Fatalf("a second post within the cooldown must not notify, got %d", n)
	}
	e.Clock.Advance(notify.PostCooldown)
	e.Review.Show(ctx, a, spec.ShowInput{Title: "progress 2", RetryID: "r3", Attachments: []spec.Attachment{file(e)}})
	drain(t, e, sub)
	if n := len(e.Push.Messages()); n != 2 {
		t.Fatalf("a post after the cooldown notifies, got %d", n)
	}
	// The owner's own status changes never notify; the agent finishing does.
	e.Review.SetThreadStatus(ctx, owner, show.ThreadID, spec.ThreadApproved)
	e.Review.Done(ctx, a, show.ThreadID, "")
	drain(t, e, sub)
	msgs := e.Push.Messages()
	if len(msgs) != 3 || msgs[2].Body != "Finished: ready for your review." {
		t.Fatalf("finished notification: %+v", msgs)
	}
	// Replies and deliveries never notify.
	e.Review.AddReply(ctx, owner, spec.ReplyInput{PostID: show.PostID, Kind: spec.ReplyNote, Text: "x"})
	drain(t, e, sub)
	if len(e.Push.Messages()) != 3 {
		t.Fatal("the owner's own reply notified")
	}
	log, _ := e.Notify.Log(ctx, owner)
	if len(log.Items) != 3 || log.Items[0].Status != notify.StatusSent {
		t.Fatalf("every send is logged: %+v", log.Items)
	}
}

func TestFailuresAreLoggedAndGoneDevicesDisabled(t *testing.T) {
	e, owner, _ := setup(t)
	e.Push.Fail = errors.New("push service down")
	if n, err := e.Notify.Test(ctx, owner); err != nil || n != 0 {
		t.Fatalf("test send: %d %v", n, err)
	}
	log, _ := e.Notify.Log(ctx, owner)
	if log.Items[0].Status != notify.StatusFailed || log.Items[0].Error != "push service down" {
		t.Fatalf("failure not logged: %+v", log.Items[0])
	}
	e.Push.Fail = nil
	e.Push.Gone = true
	e.Notify.Test(ctx, owner)
	e.Push.Gone = false
	if n, _ := e.Notify.Test(ctx, owner); n != 0 {
		t.Fatal("a gone device must be disabled")
	}
}

func TestRunConsumesBus(t *testing.T) {
	e, _, a := setup(t)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { e.Notify.Run(runCtx, e.Bus); close(done) }()
	for e.Bus.Len() == 0 {
		time.Sleep(time.Millisecond)
	}
	e.Review.Ask(ctx, a, spec.AskInput{Title: "T", Text: "q?", RetryID: "q1"})
	deadline := time.Now().Add(2 * time.Second)
	for len(e.Push.Messages()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if len(e.Push.Messages()) != 1 {
		t.Fatal("Run did not notify")
	}
}

// drain hands every published event to Handle, like Run does.
func drain(t *testing.T, e *testkit.Env, sub *eventlog.Subscription) {
	t.Helper()
	for {
		select {
		case ev := <-sub.C:
			if err := e.Notify.Handle(ctx, ev); err != nil {
				t.Fatal(err)
			}
		default:
			return
		}
	}
}
