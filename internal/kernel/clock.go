package kernel

import (
	"sync"
	"time"
)

// Clock is injected wherever time matters so tests can control it.
type Clock interface{ Now() time.Time }

// SystemClock is the real clock, truncated to milliseconds to match storage.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// FakeClock is a manually advanced clock for tests.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{t: t.UTC()} }

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// Millis and FromMillis convert to and from the int64 milliseconds used in storage.
func Millis(t time.Time) int64 { return t.UnixMilli() }

func FromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
