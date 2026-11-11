package access

import (
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

func TestPairingCodeFormat(t *testing.T) {
	seen := map[string]bool{}
	for range 500 {
		c := NewPairingCode()
		if len(c) != 7 || c[3] != '-' {
			t.Fatalf("bad code %q", c)
		}
		for i, r := range c {
			if i != 3 && !strings.ContainsRune(codeAlphabet, r) {
				t.Fatalf("code %q has %q outside the alphabet", c, r)
			}
		}
		seen[c] = true
	}
	if len(seen) < 490 {
		t.Fatalf("codes repeat too often: %d unique of 500", len(seen))
	}
}

func TestNormalizeCode(t *testing.T) {
	for in, want := range map[string]string{"kq7-4xm": "KQ7-4XM", "KQ74XM": "KQ7-4XM", " kq7 4xm ": "KQ7-4XM", "ab": "AB"} {
		if got := NormalizeCode(in); got != want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPasswordHashing(t *testing.T) {
	h := HashPassword("correct horse battery")
	if !strings.HasPrefix(h, "argon2id$v=19$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	if !VerifyPassword("correct horse battery", h) {
		t.Fatal("right password rejected")
	}
	if VerifyPassword("wrong horse battery", h) {
		t.Fatal("wrong password accepted")
	}
	if HashPassword("correct horse battery") == h {
		t.Fatal("hashes must be salted")
	}
	for _, bad := range []string{"", "argon2id$v=19$x$y$z", "bcrypt$...", "argon2id$v=19$m=1,t=1,p=1$!!$!!", "argon2id$v=19$m=1,t=1,p=1$c2FsdA$"} {
		if VerifyPassword("x", bad) {
			t.Errorf("VerifyPassword accepted malformed hash %q", bad)
		}
	}
}

func TestPasswordRules(t *testing.T) {
	if err := CheckPasswordRules("short"); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("short password allowed")
	}
	if err := CheckPasswordRules(strings.Repeat("a", MaxPassword+1)); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("huge password allowed")
	}
	if err := CheckPasswordRules("long enough password"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionUsable(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	s := &AgentSession{LastUsedAt: now.Add(-6 * 24 * time.Hour)}
	if !s.Usable(now) {
		t.Fatal("6 days idle should be usable")
	}
	s.LastUsedAt = now.Add(-SessionKeyIdleTTL)
	if s.Usable(now) {
		t.Fatal("7 days idle should be expired")
	}
	s.LastUsedAt = now
	s.RevokedAt = &now
	if s.Usable(now) {
		t.Fatal("revoked session usable")
	}
}

func TestPairingExpired(t *testing.T) {
	now := time.Now()
	p := &Pairing{ExpiresAt: now.Add(time.Second)}
	if p.Expired(now) || !p.Expired(now.Add(time.Second)) {
		t.Fatal("Expired wrong")
	}
}

func TestAgentLabel(t *testing.T) {
	if AgentLabel("claude-code") != "Claude Code" || AgentLabel("aider") != "aider" || AgentLabel("") != "Agent" {
		t.Fatal("AgentLabel wrong")
	}
}

func TestClipKeepsRunes(t *testing.T) {
	if clip("héllo", 2) != "h" { // é is 2 bytes; cutting inside it backs off
		t.Fatalf("clip cut a rune: %q", clip("héllo", 2))
	}
	if clip("abc", 5) != "abc" || clip("abcdef", 3) != "abc" {
		t.Fatal("clip wrong")
	}
}
