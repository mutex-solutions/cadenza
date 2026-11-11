package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// wait holds the request until the question is no longer open, the timeout
// passes or the client leaves (plan gap G-8). The CLI calls it in a loop.
func (a *API) wait(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
	id := r.PathValue("id")
	timeout := waitTimeout(r)
	// Subscribe before reading so an answer between the read and the wait is not missed.
	sub := a.d.Bus.Subscribe(64)
	defer func() { sub.Close() }()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		out, _, err := a.d.Review.QuestionState(r.Context(), actor(p), id)
		if err != nil {
			return err
		}
		if out.Status != spec.QuestionOpen || timeout == 0 {
			return ok(w, out)
		}
		if done, err := a.waitForChange(w, r, &sub, deadline.C, id, out); done {
			return err
		}
	}
}

// waitForChange blocks until the question may have changed (false) or the
// request is finished (true).
func (a *API) waitForChange(w http.ResponseWriter, r *http.Request, sub **eventlog.Subscription, deadline <-chan time.Time, id string, current spec.WaitOutput) (bool, error) {
	for {
		select {
		case <-r.Context().Done():
			return true, nil
		case <-deadline:
			return true, ok(w, current)
		case <-(*sub).Lagged:
			(*sub).Close()
			*sub = a.d.Bus.Subscribe(64)
			return false, nil
		case e := <-(*sub).C:
			if e.Kind == spec.EventQuestionChanged && e.RefID == id {
				return false, nil
			}
		}
	}
}

// events returns numbered changes after ?after=N for catching up (plan D15).
func (a *API) events(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	ctx := r.Context()
	oldest, err := a.d.Events.OldestSeq(ctx, p.Owner)
	if err != nil {
		return err
	}
	if after > 0 && oldest > 0 && after < oldest-1 {
		latest, err := a.d.Events.LatestSeq(ctx, p.Owner)
		if err != nil {
			return err
		}
		return ok(w, spec.EventList{Events: []spec.Event{}, Reset: true, Last: latest})
	}
	evs, err := a.d.Events.ListEvents(ctx, p.Owner, after, 500)
	if err != nil {
		return err
	}
	out := spec.EventList{Events: make([]spec.Event, 0, len(evs)), Last: after}
	for _, e := range evs {
		out.Events = append(out.Events, toSpecEvent(e))
		out.Last = e.Seq
	}
	return ok(w, out)
}

func toSpecEvent(e eventlog.Event) spec.Event {
	return spec.Event{Seq: e.Seq, Kind: e.Kind, ThreadID: e.ThreadID, RefID: e.RefID, At: e.At.UnixMilli()}
}

// heartbeat keeps proxies from closing an idle stream.
const heartbeat = 20 * time.Second

// stream sends each change as a server-sent event. A client resumes with
// Last-Event-ID (or ?after=N); a client too far behind gets a "reset" event.
func (a *API) stream(w http.ResponseWriter, r *http.Request, p *access.Principal) error {
	rc := http.NewResponseController(w)
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	sub := a.d.Bus.Subscribe(256)
	defer sub.Close()
	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	send := func(event string, id int64, data any) error {
		rc.SetWriteDeadline(time.Now().Add(heartbeat * 2))
		b, _ := json.Marshal(data)
		if id > 0 {
			fmt.Fprintf(w, "id: %d\n", id)
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return err
		}
		return rc.Flush()
	}

	// Catch up from storage, then switch to live events.
	last := after
	if after > 0 {
		oldest, err := a.d.Events.OldestSeq(ctx, p.Owner)
		if err != nil {
			return nil
		}
		if oldest > 0 && after < oldest-1 {
			send("reset", 0, map[string]bool{"reset": true})
			return nil
		}
		for {
			evs, err := a.d.Events.ListEvents(ctx, p.Owner, last, 500)
			if err != nil {
				return nil
			}
			for _, e := range evs {
				if err := send("change", e.Seq, toSpecEvent(e)); err != nil {
					return nil
				}
				last = e.Seq
			}
			if len(evs) < 500 {
				break
			}
		}
	} else if err := send("ready", 0, map[string]bool{"ready": true}); err != nil {
		return nil
	}

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.Lagged:
			send("reset", 0, map[string]bool{"reset": true})
			return nil
		case <-tick.C:
			rc.SetWriteDeadline(time.Now().Add(heartbeat * 2))
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			if err := rc.Flush(); err != nil {
				return nil
			}
		case e := <-sub.C:
			if e.Owner != p.Owner || e.Seq <= last {
				continue
			}
			if err := send("change", e.Seq, toSpecEvent(e)); err != nil {
				return nil
			}
			last = e.Seq
		}
	}
}
