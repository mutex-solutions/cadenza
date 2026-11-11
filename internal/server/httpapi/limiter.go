package httpapi

import (
	"sync"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// limiter is a fixed-window counter per key, for unauthenticated calls
// (login, setup, pairing). One process, one owner: memory is enough.
type limiter struct {
	mu     sync.Mutex
	clock  kernel.Clock
	counts map[string]window
}

type window struct {
	start time.Time
	n     int
}

func newLimiter(c kernel.Clock) *limiter { return &limiter{clock: c, counts: map[string]window{}} }

func (l *limiter) allow(key string, max int, per time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	w := l.counts[key]
	if now.Sub(w.start) >= per {
		w = window{start: now}
	}
	w.n++
	l.counts[key] = w
	if len(l.counts) > 10000 { // keep memory bounded
		for k, v := range l.counts {
			if now.Sub(v.start) >= per {
				delete(l.counts, k)
			}
		}
	}
	return w.n <= max
}
