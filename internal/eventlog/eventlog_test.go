package eventlog

import (
	"testing"
	"time"
)

func TestBusDeliversInOrderToAllSubscribers(t *testing.T) {
	b := NewBus()
	s1, s2 := b.Subscribe(10), b.Subscribe(10)
	defer s1.Close()
	defer s2.Close()
	b.Publish(Event{Seq: 1}, Event{Seq: 2})
	b.Publish(Event{Seq: 3})
	for _, s := range []*Subscription{s1, s2} {
		for want := int64(1); want <= 3; want++ {
			select {
			case e := <-s.C:
				if e.Seq != want {
					t.Fatalf("got seq %d, want %d", e.Seq, want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out")
			}
		}
	}
}

func TestBusMarksSlowSubscriberLaggedWithoutBlocking(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(1)
	defer s.Close()
	done := make(chan struct{})
	go func() {
		b.Publish(Event{Seq: 1}, Event{Seq: 2}, Event{Seq: 3})
		b.Publish(Event{Seq: 4})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}
	select {
	case <-s.Lagged:
	default:
		t.Fatal("subscriber should be marked lagged")
	}
}

func TestCloseStopsDelivery(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(1)
	s.Close()
	s.Close() // twice is fine
	if b.Len() != 0 {
		t.Fatalf("Len = %d after Close", b.Len())
	}
	b.Publish(Event{Seq: 1})
	select {
	case <-s.C:
		t.Fatal("closed subscription received an event")
	default:
	}
	b.Publish() // no events is a no-op
}
