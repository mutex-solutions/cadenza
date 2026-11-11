package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// PostCooldown limits notifications about new posts to one per thread in
// this window, so an agent posting progress does not buzz the phone each time.
// Questions and "finished" always notify.
const PostCooldown = 2 * time.Minute

// Describer explains an event (implemented by review.Service).
type Describer interface {
	Describe(ctx context.Context, e eventlog.Event) (review.EventInfo, error)
}

// Service sends notifications only when the owner is needed (plan D14).
type Service struct {
	repo     Repo
	senders  map[string]Sender
	describe Describer
	clock    kernel.Clock
	log      *slog.Logger
	vapidPub string
}

func NewService(repo Repo, senders []Sender, describe Describer, clock kernel.Clock, log *slog.Logger, vapidPublicKey string) *Service {
	m := map[string]Sender{}
	for _, s := range senders {
		m[s.Provider()] = s
	}
	return &Service{repo: repo, senders: m, describe: describe, clock: clock, log: log, vapidPub: vapidPublicKey}
}

// Config tells the apps which providers they may register with.
func (s *Service) Config() spec.PushConfig {
	out := spec.PushConfig{Providers: []string{}}
	for p := range s.senders {
		out.Providers = append(out.Providers, p)
	}
	sort.Strings(out.Providers)
	if _, ok := s.senders["webpush"]; ok {
		out.VAPIDPublicKey = s.vapidPub
	}
	return out
}

// Register adds a device, or re-enables and renames one with the same token.
func (s *Service) Register(ctx context.Context, owner kernel.OwnerID, in spec.DeviceInput) (spec.DeviceOutput, error) {
	if _, ok := s.senders[in.Provider]; !ok {
		return spec.DeviceOutput{}, kernel.Invalid("push provider %q is not enabled on this server", in.Provider).
			WithFix("enabled providers: %s", strings.Join(s.Config().Providers, ", "))
	}
	if len(in.Token) == 0 || len(in.Token) > 4096 || !json.Valid(in.Token) {
		return spec.DeviceOutput{}, kernel.Invalid("the device token must be JSON of at most 4 KB")
	}
	name := strings.TrimSpace(in.Name)
	if len(name) > 64 {
		name = name[:64]
	}
	now := s.clock.Now()
	d, err := s.repo.GetDeviceByToken(ctx, owner, in.Provider, in.Token)
	switch {
	case err == nil:
		d.Name, d.DisabledAt, d.LastOKAt = name, nil, now
		return spec.DeviceOutput{DeviceID: d.ID}, s.repo.UpdateDevice(ctx, d)
	case !kernel.Is(err, kernel.KindNotFound):
		return spec.DeviceOutput{}, err
	}
	d = &Device{ID: kernel.NewID("d"), Owner: owner, Provider: in.Provider, Token: in.Token, Name: name, CreatedAt: now, LastOKAt: now}
	return spec.DeviceOutput{DeviceID: d.ID}, s.repo.CreateDevice(ctx, d)
}

// Remove stops notifications to a device.
func (s *Service) Remove(ctx context.Context, owner kernel.OwnerID, id string) error {
	d, err := s.repo.GetDevice(ctx, owner, id)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	d.DisabledAt = &now
	return s.repo.UpdateDevice(ctx, d)
}

// Log returns recent sends, newest first.
func (s *Service) Log(ctx context.Context, owner kernel.OwnerID) (spec.NotificationLog, error) {
	es, err := s.repo.ListNotifications(ctx, owner, 100)
	if err != nil {
		return spec.NotificationLog{}, err
	}
	out := spec.NotificationLog{Items: []spec.NotificationEntry{}}
	for _, e := range es {
		out.Items = append(out.Items, spec.NotificationEntry{ID: e.ID, Device: e.DeviceID, Provider: e.Provider, Title: e.Title,
			Body: e.Body, Status: e.Status, Error: e.Error, CreatedAt: kernel.Millis(e.CreatedAt)})
	}
	return out, nil
}

// Test sends a test notification to every device and reports how many got it.
func (s *Service) Test(ctx context.Context, owner kernel.OwnerID) (sent int, err error) {
	return s.sendAll(ctx, owner, Message{Title: "Cadenza", Body: "Test notification: this device will get Cadenza alerts.", URL: "/"}, "")
}

// Handle decides whether an event needs the owner and, if so, notifies.
func (s *Service) Handle(ctx context.Context, e eventlog.Event) error {
	var body string
	switch e.Kind {
	case spec.EventQuestionAsked, spec.EventPostAdded, spec.EventThreadReady:
	default:
		return nil
	}
	info, err := s.describe.Describe(ctx, e)
	if err != nil {
		return err
	}
	switch e.Kind {
	case spec.EventQuestionAsked:
		body = "Has a question for you."
		if info.Waiting {
			body = "Is waiting for your answer."
		}
	case spec.EventPostAdded:
		if !info.FirstPost {
			last, err := s.repo.LastSentForThread(ctx, e.Owner, e.ThreadID)
			if err != nil {
				return err
			}
			if !last.IsZero() && s.clock.Now().Sub(last) < PostCooldown {
				return nil
			}
		}
		body = "Posted work to look at."
	case spec.EventThreadReady:
		body = "Finished: ready for your review."
	}
	msg := Message{Title: Title(info.Agent, info.Machine, info.Title), Body: body, ThreadID: e.ThreadID, URL: "/t/" + e.ThreadID}
	_, err = s.sendAll(ctx, e.Owner, msg, e.ThreadID)
	return err
}

// Title is "<agent> · <machine> — <title>", e.g.
// "Claude Code · vm-03 — Checkout redesign: keep promo field visible?".
func Title(agent, machine, title string) string {
	t := agent + " · " + machine
	if title != "" {
		t += " — " + title
	}
	if r := []rune(t); len(r) > 120 {
		t = string(r[:119]) + "…"
	}
	return t
}

func (s *Service) sendAll(ctx context.Context, owner kernel.OwnerID, msg Message, threadID string) (int, error) {
	devices, err := s.repo.ListActiveDevices(ctx, owner)
	if err != nil {
		return 0, err
	}
	sent := 0
	for i := range devices {
		d := &devices[i]
		sender, ok := s.senders[d.Provider]
		if !ok {
			continue // provider turned off since the device registered
		}
		now := s.clock.Now()
		entry := &Entry{ID: kernel.NewID("n"), Owner: owner, DeviceID: d.ID, Provider: d.Provider, Title: msg.Title, Body: msg.Body,
			ThreadID: threadID, Status: StatusSent, CreatedAt: now}
		res, err := sender.Send(ctx, d.Token, msg)
		switch {
		case err != nil:
			entry.Status, entry.Error = StatusFailed, err.Error()
		case res.Gone:
			entry.Status, entry.Error = StatusFailed, "device no longer registered; disabled"
			d.DisabledAt = &now
		default:
			sent++
			d.LastOKAt = now
		}
		if err := s.repo.UpdateDevice(ctx, d); err != nil {
			return sent, err
		}
		if err := s.repo.CreateNotification(ctx, entry); err != nil {
			return sent, err
		}
		if entry.Status == StatusFailed {
			s.log.Warn("notification failed", "device", d.ID, "provider", d.Provider, "error", entry.Error)
		}
	}
	return sent, nil
}

// Run handles events from the bus until ctx ends. Sends happen one at a time
// so a slow provider never blocks the request that caused the event.
func (s *Service) Run(ctx context.Context, bus *eventlog.Bus) {
	for ctx.Err() == nil {
		sub := bus.Subscribe(256)
		s.consume(ctx, sub)
		sub.Close()
	}
}

func (s *Service) consume(ctx context.Context, sub *eventlog.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Lagged:
			s.log.Warn("notifications fell behind; some events were not checked")
			return
		case e := <-sub.C:
			hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := s.Handle(hctx, e); err != nil {
				s.log.Warn("notification handling failed", "event", e.Seq, "kind", e.Kind, "error", err)
			}
			cancel()
		}
	}
}
