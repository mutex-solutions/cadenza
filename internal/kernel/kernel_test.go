package kernel

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewIDFormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := NewID("t")
		if !HasPrefix(id, "t") {
			t.Fatalf("NewID produced %q which HasPrefix rejects", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestHasPrefixRejects(t *testing.T) {
	for _, id := range []string{"", "t", "t_", "p_abcdefghijklmnop", "t_abc", "t_ABCDEFGHIJKLMNOP", "t_abcdefghijklmno1", "t_abcdefghijklmnopq"} {
		if HasPrefix(id, "t") {
			t.Errorf("HasPrefix(%q, t) = true, want false", id)
		}
	}
}

func TestErrorKindsAndWrapping(t *testing.T) {
	base := Invalid("slot %q needs an image", "after").WithFix(`pass {"file": "<path>"}`)
	if base.Kind != KindInvalid || !strings.Contains(base.Error(), "fix: pass") {
		t.Fatalf("unexpected error: %v", base)
	}
	wrapped := fmt.Errorf("show: %w", base)
	if !Is(wrapped, KindInvalid) || Is(wrapped, KindNotFound) {
		t.Fatalf("Is did not see through wrapping")
	}
	if KindOf(errors.New("plain")) != KindInternal {
		t.Fatalf("plain errors should be internal")
	}
	if Is(nil, KindInternal) {
		t.Fatalf("nil is not an error of any kind")
	}
	cause := errors.New("disk full")
	ie := Internal(cause, "saving post")
	if !errors.Is(ie, cause) {
		t.Fatalf("Internal must wrap its cause")
	}
}

func TestWithFixDoesNotMutate(t *testing.T) {
	e := NotFound("no thread")
	_ = e.WithFix("x")
	if e.Fix != "" {
		t.Fatalf("WithFix mutated the receiver")
	}
}

func TestSecrets(t *testing.T) {
	a, b := NewSecret("cz_sk_"), NewSecret("cz_sk_")
	if a == b || !strings.HasPrefix(a, "cz_sk_") || len(a) != len("cz_sk_")+43 {
		t.Fatalf("bad secrets %q %q", a, b)
	}
	if HashSecret(a) == HashSecret(b) || len(HashSecret(a)) != 64 {
		t.Fatalf("bad hashes")
	}
	if !SecretEqual("x", "x") || SecretEqual("x", "y") {
		t.Fatalf("SecretEqual wrong")
	}
}

func TestClocks(t *testing.T) {
	start := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	c := NewFakeClock(start)
	c.Advance(time.Minute)
	if !c.Now().Equal(start.Add(time.Minute)) {
		t.Fatalf("fake clock did not advance")
	}
	now := SystemClock{}.Now()
	if now.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("system clock not truncated to ms")
	}
	if !FromMillis(Millis(now)).Equal(now) {
		t.Fatalf("millis round trip failed")
	}
}
