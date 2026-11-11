package access_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/testkit"
)

var ctx = context.Background()

func TestSetupLoginLogout(t *testing.T) {
	e := testkit.New(t)
	if has, _ := e.Access.HasOwner(ctx); has {
		t.Fatal("fresh server has no owner")
	}
	if _, err := e.Access.Login(ctx, "anything at all"); !kernel.Is(err, kernel.KindConflict) {
		t.Fatalf("login before setup: %v", err)
	}
	if err := e.Access.Setup(ctx, "short"); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("weak password accepted")
	}
	if err := e.Access.Setup(ctx, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if err := e.Access.Setup(ctx, "another long password"); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("second setup must fail")
	}
	if _, err := e.Access.Login(ctx, "wrong horse battery"); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("wrong password accepted")
	}
	tok, err := e.Access.Login(ctx, "correct horse battery")
	if err != nil || !strings.HasPrefix(tok, access.PrefixWebSession) {
		t.Fatalf("login: %q %v", tok, err)
	}
	p, err := e.Access.Authenticate(ctx, tok)
	if err != nil || p.Role != access.RoleOwner {
		t.Fatalf("auth: %+v %v", p, err)
	}
	e.Clock.Advance(access.WebSessionTTL)
	if _, err := e.Access.Authenticate(ctx, tok); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("expired web session accepted")
	}
	tok, _ = e.Access.Login(ctx, "correct horse battery")
	if err := e.Access.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Access.Authenticate(ctx, tok); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("logged-out token accepted")
	}
	if _, err := e.Access.Authenticate(ctx, "garbage"); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("garbage accepted")
	}
}

func TestResetPassword(t *testing.T) {
	e := testkit.New(t)
	if err := e.Access.ResetPassword(ctx, "first long password"); err != nil {
		t.Fatal(err)
	}
	if err := e.Access.ResetPassword(ctx, "second long password"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Access.Login(ctx, "first long password"); err == nil {
		t.Fatal("old password still works")
	}
	if _, err := e.Access.Login(ctx, "second long password"); err != nil {
		t.Fatal(err)
	}
	if err := e.Access.ResetPassword(ctx, "x"); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("weak reset accepted")
	}
}

func TestPairingFlow(t *testing.T) {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	start, err := e.Access.PairStart(ctx, "vm-03")
	if err != nil {
		t.Fatal(err)
	}
	poll, err := e.Access.PairPoll(ctx, start.PairID, start.Secret)
	if err != nil || poll.Status != spec.PairPending || poll.MachineKey != "" {
		t.Fatalf("pending poll: %+v %v", poll, err)
	}
	if _, err := e.Access.PairPoll(ctx, start.PairID, "wrong"); !kernel.Is(err, kernel.KindForbidden) {
		t.Fatal("wrong secret accepted")
	}
	if _, err := e.Access.PairApprove(ctx, owner, "AAA-AAA", "x"); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("unknown code approved")
	}
	// People type codes loosely.
	appr, err := e.Access.PairApprove(ctx, owner, strings.ToLower(strings.ReplaceAll(start.Code, "-", "")), "")
	if err != nil || appr.Name != "vm-03" {
		t.Fatalf("approve: %+v %v", appr, err)
	}
	poll, err = e.Access.PairPoll(ctx, start.PairID, start.Secret)
	if err != nil || poll.Status != spec.PairApproved || !strings.HasPrefix(poll.MachineKey, access.PrefixMachine) {
		t.Fatalf("approved poll: %+v %v", poll, err)
	}
	if _, err := e.Access.PairPoll(ctx, start.PairID, start.Secret); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("the machine key must be handed out only once")
	}
	p, err := e.Access.Authenticate(ctx, poll.MachineKey)
	if err != nil || p.Role != access.RoleMachine || p.MachineName != "vm-03" {
		t.Fatalf("machine auth: %+v %v", p, err)
	}
	list, _ := e.Access.Machines(ctx, owner)
	if len(list.Machines) != 1 || list.Machines[0].Name != "vm-03" {
		t.Fatalf("machines: %+v", list)
	}
}

func TestPairingExpires(t *testing.T) {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	start, _ := e.Access.PairStart(ctx, "box")
	e.Clock.Advance(access.PairingTTL)
	if poll, _ := e.Access.PairPoll(ctx, start.PairID, start.Secret); poll.Status != spec.PairExpired {
		t.Fatalf("status %s", poll.Status)
	}
	if _, err := e.Access.PairApprove(ctx, owner, start.Code, "box"); !kernel.Is(err, kernel.KindConflict) {
		t.Fatal("expired code approved")
	}
}

func TestSessionKeys(t *testing.T) {
	e := testkit.New(t)
	owner, ownerTok := e.Owner(t)
	mk := e.Machine(t, "vm-03")
	m, _ := e.Access.Authenticate(ctx, mk)
	ownerP, _ := e.Access.Authenticate(ctx, ownerTok)
	if _, err := e.Access.MintSessionKey(ctx, ownerP, spec.SessionKeyInput{Agent: "x", AgentSessionID: "y"}); !kernel.Is(err, kernel.KindForbidden) {
		t.Fatal("only machine keys may mint session keys")
	}
	if _, err := e.Access.MintSessionKey(ctx, m, spec.SessionKeyInput{Agent: "claude-code"}); !kernel.Is(err, kernel.KindInvalid) ||
		!strings.Contains(err.Error(), "--session") {
		t.Fatalf("missing session id must explain --session: %v", err)
	}
	a, err := e.Access.MintSessionKey(ctx, m, spec.SessionKeyInput{Agent: "Claude-Code", AgentSessionID: "abc", Cwd: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	// Resume: same agent session id, same session, new key; the old key stops.
	b, err := e.Access.MintSessionKey(ctx, m, spec.SessionKeyInput{Agent: "claude-code", AgentSessionID: "abc"})
	if err != nil || b.SessionID != a.SessionID || b.SessionKey == a.SessionKey {
		t.Fatalf("resume: %+v vs %+v", a, b)
	}
	if _, err := e.Access.Authenticate(ctx, a.SessionKey); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("replaced key still works")
	}
	p, err := e.Access.Authenticate(ctx, b.SessionKey)
	if err != nil || p.Role != access.RoleSession || p.SessionID != b.SessionID || p.Agent != "claude-code" || p.MachineName != "vm-03" {
		t.Fatalf("session auth: %+v %v", p, err)
	}
	s, _ := e.Access.Session(ctx, owner, b.SessionID)
	if s.Cwd != "/w" {
		t.Fatal("cwd from the first mint must be kept when later mints omit it")
	}
	// Idle for 7 days: the key expires; minting again revives the session.
	e.Clock.Advance(access.SessionKeyIdleTTL)
	if _, err := e.Access.Authenticate(ctx, b.SessionKey); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("idle key accepted")
	}
	c, _ := e.Access.MintSessionKey(ctx, m, spec.SessionKeyInput{Agent: "claude-code", AgentSessionID: "abc"})
	if c.SessionID != a.SessionID {
		t.Fatal("expired session must be revived, not replaced")
	}
	// Revoking.
	if err := e.Access.RevokeSession(ctx, owner, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Access.Authenticate(ctx, c.SessionKey); !kernel.Is(err, kernel.KindUnauthorized) {
		t.Fatal("revoked key accepted")
	}
}

func TestRenameAndRemoveMachine(t *testing.T) {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	mk := e.Machine(t, "vm-03")
	m, _ := e.Access.Authenticate(ctx, mk)
	sk, _ := e.Session(t, mk, "abc")
	if err := e.Access.RenameMachine(ctx, owner, m.MachineID, " "); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("empty name accepted")
	}
	if err := e.Access.RenameMachine(ctx, owner, m.MachineID, "build-box"); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.Access.Authenticate(ctx, mk); p.MachineName != "build-box" {
		t.Fatal("rename lost")
	}
	if err := e.Access.RemoveMachine(ctx, owner, m.MachineID); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{mk, sk} {
		if _, err := e.Access.Authenticate(ctx, key); !kernel.Is(err, kernel.KindUnauthorized) {
			t.Fatal("keys of a removed machine must stop at once")
		}
	}
	if err := e.Access.RemoveMachine(ctx, owner, m.MachineID); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("removing twice")
	}
	if list, _ := e.Access.Machines(ctx, owner); len(list.Machines) != 0 {
		t.Fatal("removed machine listed")
	}
}

func TestLastSeenIsThrottled(t *testing.T) {
	e := testkit.New(t)
	owner, _ := e.Owner(t)
	mk := e.Machine(t, "vm-03")
	e.Clock.Advance(30 * time.Second)
	e.Access.Authenticate(ctx, mk)
	list, _ := e.Access.Machines(ctx, owner)
	first := list.Machines[0].LastSeenAt
	e.Clock.Advance(2 * time.Minute)
	e.Access.Authenticate(ctx, mk)
	list, _ = e.Access.Machines(ctx, owner)
	if list.Machines[0].LastSeenAt <= first {
		t.Fatal("last seen not updated after a minute")
	}
}
