package eventlog

import "sync"

// Bus fans published events out to subscribers in this process. It never
// blocks a publisher: a subscriber whose buffer is full is marked lagged and
// must reload from the log.
type Bus struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// Subscription receives events on C. When Lagged is closed, events were
// dropped and the subscriber should catch up from storage.
type Subscription struct {
	C      <-chan Event
	Lagged <-chan struct{}

	c      chan Event
	lagged chan struct{}
	once   sync.Once
	bus    *Bus
}

func NewBus() *Bus { return &Bus{subs: map[*Subscription]struct{}{}} }

// Subscribe returns a subscription with a buffer of size n.
func (b *Bus) Subscribe(n int) *Subscription {
	s := &Subscription{c: make(chan Event, n), lagged: make(chan struct{}), bus: b}
	s.C, s.Lagged = s.c, s.lagged
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

// Close stops delivery. It is safe to call more than once.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	delete(s.bus.subs, s)
	s.bus.mu.Unlock()
}

// Publish delivers events, in order, to every subscriber.
func (b *Bus) Publish(events ...Event) {
	if len(events) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		for _, e := range events {
			select {
			case s.c <- e:
			default:
				s.once.Do(func() { close(s.lagged) })
			}
		}
	}
}

// Len is the number of live subscriptions (for tests and metrics).
func (b *Bus) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
