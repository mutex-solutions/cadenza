// Package storetest is one test suite that every storage engine must pass.
// The SQLite tests run it in memory; the Postgres tests run it when
// CADENZA_TEST_POSTGRES points at a database.
package storetest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Store is what both engines provide.
type Store interface {
	Access() access.Store
	Review() review.Store
	Files() files.Repo
	Catalog() catalog.Repo
	Notify() notify.Repo
	Events() eventlog.Reader
	Settings() sqlstore.Settings
}

// Run runs every test against stores made by open. Each call to open must
// return an empty store.
func Run(t *testing.T, open func(t *testing.T) Store) {
	tests := map[string]func(*testing.T, Store){
		"Access":       testAccess,
		"Threads":      testThreads,
		"Posts":        testPosts,
		"Questions":    testQuestions,
		"Replies":      testReplies,
		"Transactions": testTransactions,
		"Events":       testEvents,
		"Files":        testFiles,
		"Catalog":      testCatalog,
		"Notify":       testNotify,
		"Settings":     testSettings,
	}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) { fn(t, open(t)) })
	}
}

var (
	ctx = context.Background()
	t0  = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
)

const owner = kernel.OwnerID("o_1")

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantNotFound(t *testing.T, err error) {
	t.Helper()
	if !kernel.Is(err, kernel.KindNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

// seed creates a machine and an agent session and returns their ids.
func seed(t *testing.T, s Store) (machineID, sessionID string) {
	t.Helper()
	a := s.Access()
	m := &access.Machine{ID: kernel.NewID("m"), Owner: owner, Name: "vm-03", KeyHash: kernel.NewID("k"), CreatedAt: t0, LastSeenAt: t0}
	must(t, a.CreateMachine(ctx, m))
	as := &access.AgentSession{ID: kernel.NewID("s"), Owner: owner, MachineID: m.ID, Agent: "claude-code", ExternalID: kernel.NewID("x"),
		KeyHash: kernel.NewID("k"), CreatedAt: t0, LastUsedAt: t0}
	must(t, a.CreateAgentSession(ctx, as))
	return m.ID, as.ID
}

func testAccess(t *testing.T, s Store) {
	a := s.Access()
	_, err := a.GetOwner(ctx)
	wantNotFound(t, err)
	o := &access.Owner{ID: owner, PasswordHash: "h1", CreatedAt: t0}
	must(t, a.SaveOwner(ctx, o))
	o.PasswordHash = "h2"
	must(t, a.SaveOwner(ctx, o))
	got, err := a.GetOwner(ctx)
	must(t, err)
	if got.ID != owner || got.PasswordHash != "h2" || !got.CreatedAt.Equal(t0) {
		t.Fatalf("owner = %+v", got)
	}

	ws := &access.WebSession{TokenHash: "th", Owner: owner, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	must(t, a.CreateWebSession(ctx, ws))
	gws, err := a.GetWebSession(ctx, "th")
	must(t, err)
	if !gws.ExpiresAt.Equal(ws.ExpiresAt) {
		t.Fatal("web session expiry lost")
	}
	must(t, a.DeleteWebSession(ctx, "th"))
	_, err = a.GetWebSession(ctx, "th")
	wantNotFound(t, err)

	m := &access.Machine{ID: "m_1", Owner: owner, Name: "vm-03", KeyHash: "kh1", CreatedAt: t0, LastSeenAt: t0}
	must(t, a.CreateMachine(ctx, m))
	removed := t0.Add(time.Hour)
	m.Name, m.KeyHash, m.RemovedAt = "vm-04", "kh2", &removed
	must(t, a.UpdateMachine(ctx, m))
	gm, err := a.GetMachineByKeyHash(ctx, "kh2")
	must(t, err)
	if gm.Name != "vm-04" || gm.RemovedAt == nil || !gm.RemovedAt.Equal(removed) {
		t.Fatalf("machine = %+v", gm)
	}
	_, err = a.GetMachine(ctx, "o_other", "m_1")
	wantNotFound(t, err)
	m2 := &access.Machine{ID: "m_2", Owner: owner, Name: "alpha", KeyHash: "kh3", CreatedAt: t0, LastSeenAt: t0}
	must(t, a.CreateMachine(ctx, m2))
	as := &access.AgentSession{ID: "s_1", Owner: owner, MachineID: "m_2", Agent: "codex", ExternalID: "abc", Cwd: "/w", Repo: "r",
		KeyHash: "sk1", CreatedAt: t0, LastUsedAt: t0}
	must(t, a.CreateAgentSession(ctx, as))
	rows, err := a.ListMachines(ctx, owner)
	must(t, err)
	if len(rows) != 1 || rows[0].ID != "m_2" || rows[0].Sessions != 1 {
		t.Fatalf("ListMachines must skip removed machines and count sessions: %+v", rows)
	}

	gas, err := a.GetAgentSessionByExternal(ctx, "m_2", "codex", "abc")
	must(t, err)
	if gas.ID != "s_1" || gas.Cwd != "/w" {
		t.Fatalf("session = %+v", gas)
	}
	revoked := t0.Add(time.Minute)
	gas.KeyHash, gas.RevokedAt, gas.LastUsedAt = "sk2", &revoked, t0.Add(time.Minute)
	must(t, a.UpdateAgentSession(ctx, gas))
	gas, err = a.GetAgentSessionByKeyHash(ctx, "sk2")
	must(t, err)
	if gas.RevokedAt == nil {
		t.Fatal("revoked_at lost")
	}
	_, err = a.GetAgentSession(ctx, owner, "s_missing")
	wantNotFound(t, err)
	rows, _ = a.ListMachines(ctx, owner)
	if rows[0].Sessions != 0 {
		t.Fatal("revoked sessions must not count")
	}
	live := &access.AgentSession{ID: "s_2", Owner: owner, MachineID: "m_2", Agent: "claude-code", ExternalID: "def",
		KeyHash: "sk3", CreatedAt: t0, LastUsedAt: t0.Add(time.Hour)}
	must(t, a.CreateAgentSession(ctx, live))
	gone := &access.AgentSession{ID: "s_3", Owner: owner, MachineID: "m_1", Agent: "codex", ExternalID: "x",
		KeyHash: "sk4", CreatedAt: t0, LastUsedAt: t0}
	must(t, a.CreateAgentSession(ctx, gone)) // on the removed machine
	sess, err := a.ListAgentSessions(ctx, owner)
	must(t, err)
	if len(sess) != 1 || sess[0].ID != "s_2" || sess[0].MachineName != "alpha" {
		t.Fatalf("ListAgentSessions must skip revoked sessions and removed machines: %+v", sess)
	}

	p := &access.Pairing{ID: "pr_1", Code: "KQ7-4XM", SecretHash: "sh", Hostname: "box", Status: access.PairingPending,
		CreatedAt: t0, ExpiresAt: t0.Add(10 * time.Minute)}
	must(t, a.CreatePairing(ctx, p))
	gp, err := a.GetPendingPairingByCode(ctx, "KQ7-4XM")
	must(t, err)
	gp.Status, gp.Owner, gp.MachineID, gp.MachineName = access.PairingApproved, owner, "m_2", "alpha"
	must(t, a.UpdatePairing(ctx, gp))
	_, err = a.GetPendingPairingByCode(ctx, "KQ7-4XM")
	wantNotFound(t, err)
	gp, err = a.GetPairing(ctx, "pr_1")
	must(t, err)
	if gp.Status != access.PairingApproved || gp.MachineName != "alpha" {
		t.Fatalf("pairing = %+v", gp)
	}
}

func testThreads(t *testing.T, s Store) {
	r := s.Review()
	mID, sID := seed(t, s)
	_, err := r.LatestOpenThread(ctx, owner, sID)
	wantNotFound(t, err)
	a := review.Actor{Owner: owner, SessionID: sID, MachineID: mID}
	older := review.NewThread(a, "older", t0)
	newer := review.NewThread(a, "newer", t0.Add(time.Minute))
	must(t, r.CreateThread(ctx, older))
	must(t, r.CreateThread(ctx, newer))
	older.MarkRead(t0.Add(time.Hour))
	must(t, r.UpdateThread(ctx, older))
	if g, _ := r.GetThread(ctx, owner, older.ID); !g.ReadAt.Equal(t0.Add(time.Hour)) {
		t.Fatal("read_at lost")
	}
	got, err := r.LatestOpenThread(ctx, owner, sID)
	must(t, err)
	if got.ID != newer.ID {
		t.Fatal("LatestOpenThread must pick the most recently updated")
	}
	must(t, newer.SetStatus(spec.ThreadArchived, t0.Add(2*time.Minute)))
	must(t, r.UpdateThread(ctx, newer))
	got, _ = r.LatestOpenThread(ctx, owner, sID)
	if got.ID != older.ID {
		t.Fatal("archived threads are not open")
	}
	open, err := r.ListThreads(ctx, owner, "", false, 10)
	must(t, err)
	all, err := r.ListThreads(ctx, owner, "", true, 10)
	must(t, err)
	onMachine, err := r.ListThreads(ctx, owner, mID, false, 10)
	must(t, err)
	none, err := r.ListThreads(ctx, owner, "m_other", false, 10)
	must(t, err)
	if len(open) != 1 || len(all) != 2 || len(onMachine) != 1 || len(none) != 0 {
		t.Fatalf("list sizes: open %d all %d machine %d other %d", len(open), len(all), len(onMachine), len(none))
	}
	if all[0].ID != newer.ID {
		t.Fatal("threads must be newest change first")
	}
	if open[0].MachineName != "vm-03" || open[0].Agent != "claude-code" {
		t.Fatalf("row joins: %+v", open[0])
	}
	row, err := r.GetThreadRow(ctx, owner, older.ID)
	must(t, err)
	if row.Posts != 0 || row.OpenQuestions != 0 || row.Title != "older" {
		t.Fatalf("row = %+v", row)
	}
	_, err = r.GetThread(ctx, "o_other", older.ID)
	wantNotFound(t, err)
}

func testPosts(t *testing.T, s Store) {
	r := s.Review()
	mID, sID := seed(t, s)
	th := review.NewThread(review.Actor{Owner: owner, SessionID: sID, MachineID: mID}, "x", t0)
	must(t, r.CreateThread(ctx, th))
	ref := &media.FileRef{SHA256: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", Size: 3, Type: media.TypeHTML, Name: "a.html"}
	p := &review.Post{ID: "p_1", Owner: owner, ThreadID: th.ID, Seq: 1, Title: "t", Summary: "s", SummaryHTML: "<p>s</p>",
		SessionID: sID, RetryID: "r1", CreatedAt: t0, Attachments: []review.Attachment{
			{Index: 0, Template: "ui-change", Version: 1, Data: json.RawMessage(`{"a":1}`), View: json.RawMessage(`{"v":1}`)},
			{Index: 1, File: ref},
		}}
	must(t, r.CreatePost(ctx, p))
	got, err := r.GetPostByRetry(ctx, owner, sID, "r1")
	must(t, err)
	if got.ID != "p_1" || len(got.Attachments) != 2 || got.Attachments[0].Template != "ui-change" ||
		string(got.Attachments[0].View) != `{"v":1}` || got.Attachments[1].File == nil || *got.Attachments[1].File != *ref ||
		got.Attachments[0].File != nil || got.SummaryHTML != "<p>s</p>" {
		t.Fatalf("post round trip: %+v", got)
	}
	p2 := &review.Post{ID: "p_2", Owner: owner, ThreadID: th.ID, Seq: 2, Title: "t2", SessionID: sID, RetryID: "r2", CreatedAt: t0,
		Attachments: []review.Attachment{{Index: 0, File: ref}}}
	must(t, r.CreatePost(ctx, p2))
	dup := *p2
	dup.ID, dup.Seq = "p_3", 3
	if err := r.CreatePost(ctx, &dup); err == nil {
		t.Fatal("same session and retry id must be rejected")
	}
	posts, err := r.ListPosts(ctx, owner, th.ID)
	must(t, err)
	if len(posts) != 2 || posts[0].Seq != 1 || len(posts[0].Attachments) != 2 || len(posts[1].Attachments) != 1 {
		t.Fatalf("ListPosts: %+v", posts)
	}
	row, _ := r.GetThreadRow(ctx, owner, th.ID)
	if row.Posts != 2 {
		t.Fatalf("post count %d", row.Posts)
	}
	_, err = r.GetPost(ctx, owner, "p_missing")
	wantNotFound(t, err)
}

func testQuestions(t *testing.T, s Store) {
	r := s.Review()
	mID, sID := seed(t, s)
	th := review.NewThread(review.Actor{Owner: owner, SessionID: sID, MachineID: mID}, "x", t0)
	must(t, r.CreateThread(ctx, th))
	q := &review.Question{ID: "q_1", Owner: owner, ThreadID: th.ID, SessionID: sID, Text: "keep?", TextHTML: "<p>keep?</p>",
		Choices: []string{"yes", "no"}, Waiting: true, NotifyTitle: "n", Status: spec.QuestionOpen, RetryID: "r1", CreatedAt: t0}
	must(t, r.CreateQuestion(ctx, q))
	q2 := &review.Question{ID: "q_2", Owner: owner, ThreadID: th.ID, SessionID: sID, Text: "free", Status: spec.QuestionOpen,
		RetryID: "r2", CreatedAt: t0.Add(time.Second)}
	must(t, r.CreateQuestion(ctx, q2))
	got, err := r.GetQuestionByRetry(ctx, owner, sID, "r1")
	must(t, err)
	if len(got.Choices) != 2 || !got.Waiting || got.NotifyTitle != "n" || got.ClosedAt != nil {
		t.Fatalf("question round trip: %+v", got)
	}
	must(t, got.ReplaceWith("q_2", t0))
	must(t, r.UpdateQuestion(ctx, got))
	got, _ = r.GetQuestion(ctx, owner, "q_1")
	if got.Status != spec.QuestionReplaced || got.ReplacedBy != "q_2" || got.ClosedAt == nil {
		t.Fatalf("update lost: %+v", got)
	}
	g2, _ := r.GetQuestion(ctx, owner, "q_2")
	if g2.Choices != nil {
		t.Fatal("no choices should read back as nil")
	}
	qs, err := r.ListQuestions(ctx, owner, th.ID)
	must(t, err)
	if len(qs) != 2 || qs[0].ID != "q_1" {
		t.Fatalf("ListQuestions: %+v", qs)
	}
	row, _ := r.GetThreadRow(ctx, owner, th.ID)
	if row.OpenQuestions != 1 {
		t.Fatalf("open questions %d", row.OpenQuestions)
	}
}

func testReplies(t *testing.T, s Store) {
	r := s.Review()
	mID, sID := seed(t, s)
	th := review.NewThread(review.Actor{Owner: owner, SessionID: sID, MachineID: mID}, "x", t0)
	must(t, r.CreateThread(ctx, th))
	files := []media.FileRef{{SHA256: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", Size: 3, Type: media.TypeWebP, Name: "photo.webp"}}
	mk := func(id string, seq int64, kind string) *review.Reply {
		return &review.Reply{ID: id, Owner: owner, ThreadID: th.ID, SessionID: sID, Kind: kind, Seq: seq, CreatedAt: t0}
	}
	a := mk("r_1", 10, spec.ReplyAnswer)
	a.QuestionID, a.Choice, a.Files = "q_1", "yes", files
	c := mk("r_2", 11, spec.ReplyComment)
	c.Spot = &spec.Spot{Attachment: 0, Line: 12, Quote: "x := 1"}
	c.Extra = json.RawMessage(`{"labels":["a"]}`)
	n := mk("r_3", 12, spec.ReplyNote)
	for _, x := range []*review.Reply{a, c, n} {
		must(t, r.CreateReply(ctx, x))
	}
	got, err := r.AnswerFor(ctx, owner, "q_1")
	must(t, err)
	if got.ID != "r_1" || len(got.Files) != 1 || got.Files[0].Name != "photo.webp" {
		t.Fatalf("AnswerFor: %+v", got)
	}
	gc, _ := r.GetReply(ctx, owner, "r_2")
	if gc.Spot == nil || gc.Spot.Line != 12 || string(gc.Extra) != `{"labels":["a"]}` || gc.Files != nil {
		t.Fatalf("comment round trip: %+v", gc)
	}
	und, err := r.Undelivered(ctx, owner, sID, 10, 10)
	must(t, err)
	if len(und) != 2 || und[0].ID != "r_2" {
		t.Fatalf("Undelivered after 10: %+v", und)
	}
	nMarked, err := r.MarkDelivered(ctx, owner, sID, 11, t0)
	must(t, err)
	if nMarked != 2 {
		t.Fatalf("MarkDelivered marked %d, want 2", nMarked)
	}
	und, _ = r.Undelivered(ctx, owner, sID, 0, 10)
	if len(und) != 1 || und[0].ID != "r_3" {
		t.Fatalf("Undelivered after marking: %+v", und)
	}
	must(t, r.MoveUndelivered(ctx, owner, th.ID, "s_new"))
	und, _ = r.Undelivered(ctx, owner, "s_new", 0, 10)
	if len(und) != 1 {
		t.Fatal("MoveUndelivered did not move the undelivered reply")
	}
	gc.AgentReply = "fixed"
	at := t0.Add(time.Hour)
	gc.AgentReplyAt = &at
	must(t, r.UpdateReply(ctx, gc))
	reps, err := r.ListReplies(ctx, owner, th.ID)
	must(t, err)
	if len(reps) != 3 || reps[1].AgentReply != "fixed" || reps[0].DeliveredAt == nil {
		t.Fatalf("ListReplies: %+v", reps)
	}
	cur, err := r.GetCursor(ctx, sID)
	must(t, err)
	if cur != 0 {
		t.Fatal("unset cursor must be 0")
	}
	must(t, r.SetCursor(ctx, sID, 11))
	must(t, r.SetCursor(ctx, sID, 12))
	if cur, _ = r.GetCursor(ctx, sID); cur != 12 {
		t.Fatalf("cursor = %d", cur)
	}
}

func testTransactions(t *testing.T, s Store) {
	r := s.Review()
	mID, sID := seed(t, s)
	th := review.NewThread(review.Actor{Owner: owner, SessionID: sID, MachineID: mID}, "x", t0)
	err := r.InTx(ctx, func(tx review.Repo) error {
		if err := tx.CreateThread(ctx, th); err != nil {
			return err
		}
		e := &eventlog.Event{Owner: owner, Kind: spec.EventThreadCreated, ThreadID: th.ID, At: t0}
		if err := tx.AppendEvent(ctx, e); err != nil {
			return err
		}
		return kernel.Conflict("roll back")
	})
	if !kernel.Is(err, kernel.KindConflict) {
		t.Fatalf("InTx must return fn's error, got %v", err)
	}
	_, err = r.GetThread(ctx, owner, th.ID)
	wantNotFound(t, err)
	if latest, _ := s.Events().LatestSeq(ctx, owner); latest != 0 {
		t.Fatal("rolled-back event was kept")
	}
	must(t, s.Access().InTx(ctx, func(tx access.Repo) error {
		return tx.SaveOwner(ctx, &access.Owner{ID: owner, PasswordHash: "h", CreatedAt: t0})
	}))
	if _, err := s.Access().GetOwner(ctx); err != nil {
		t.Fatal("committed owner missing")
	}
}

func testEvents(t *testing.T, s Store) {
	r := s.Review()
	ev := s.Events()
	if n, _ := ev.OldestSeq(ctx, owner); n != 0 {
		t.Fatal("empty log oldest must be 0")
	}
	var seqs []int64
	for i := range 5 {
		e := &eventlog.Event{Owner: owner, Kind: spec.EventPostAdded, ThreadID: "t_1", RefID: "p", SessionID: "s", At: t0.Add(time.Duration(i) * time.Hour)}
		must(t, r.AppendEvent(ctx, e))
		seqs = append(seqs, e.Seq)
	}
	other := &eventlog.Event{Owner: "o_other", Kind: spec.EventPostAdded, At: t0}
	must(t, r.AppendEvent(ctx, other))
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("seqs not increasing: %v", seqs)
		}
	}
	got, err := ev.ListEvents(ctx, owner, seqs[1], 2)
	must(t, err)
	if len(got) != 2 || got[0].Seq != seqs[2] || got[0].ThreadID != "t_1" || got[0].SessionID != "s" || !got[0].At.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("ListEvents: %+v", got)
	}
	latest, _ := ev.LatestSeq(ctx, owner)
	oldest, _ := ev.OldestSeq(ctx, owner)
	if latest != seqs[4] || oldest != seqs[0] {
		t.Fatalf("latest %d oldest %d", latest, oldest)
	}
	n, err := ev.DeleteEventsBefore(ctx, t0.Add(2*time.Hour))
	must(t, err)
	if n != 3 { // two of owner's, one of the other owner's
		t.Fatalf("pruned %d", n)
	}
	if oldest, _ = ev.OldestSeq(ctx, owner); oldest != seqs[2] {
		t.Fatalf("oldest after prune %d", oldest)
	}
}

func testFiles(t *testing.T, s Store) {
	f := s.Files()
	sha := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	_, err := f.GetFile(ctx, owner, sha)
	wantNotFound(t, err)
	must(t, f.SaveFile(ctx, &files.Record{Owner: owner, SHA256: sha, Size: 3, Type: media.TypePDF, Name: "a.pdf", CreatedAt: t0}))
	must(t, f.SaveFile(ctx, &files.Record{Owner: owner, SHA256: sha, Size: 99, Type: media.TypePDF, Name: "b.pdf", CreatedAt: t0}))
	got, err := f.GetFile(ctx, owner, sha)
	must(t, err)
	if got.Size != 3 || got.Name != "a.pdf" {
		t.Fatalf("SaveFile must keep the first record: %+v", got)
	}
}

func testCatalog(t *testing.T, s Store) {
	c := s.Catalog()
	_, err := c.LatestTemplate(ctx, owner, "db-migration")
	wantNotFound(t, err)
	for v := 1; v <= 2; v++ {
		must(t, c.CreateTemplate(ctx, &catalog.Stored{Owner: owner, Name: "db-migration", Version: v, Source: catalog.SourceAgent,
			Definition: json.RawMessage(`{"v":` + string(rune('0'+v)) + `}`), CreatedAt: t0}))
	}
	must(t, c.CreateTemplate(ctx, &catalog.Stored{Owner: owner, Name: "api-change", Version: 1, Source: catalog.SourceOwner,
		Definition: json.RawMessage(`{}`), CreatedAt: t0}))
	if err := c.CreateTemplate(ctx, &catalog.Stored{Owner: owner, Name: "api-change", Version: 1, Definition: json.RawMessage(`{}`), CreatedAt: t0}); err == nil {
		t.Fatal("versions are immutable: duplicate must fail")
	}
	latest, err := c.LatestTemplate(ctx, owner, "db-migration")
	must(t, err)
	v1, err := c.GetTemplate(ctx, owner, "db-migration", 1)
	must(t, err)
	if latest.Version != 2 || string(v1.Definition) != `{"v":1}` {
		t.Fatalf("latest %+v v1 %+v", latest, v1)
	}
	all, err := c.ListLatestTemplates(ctx, owner)
	must(t, err)
	if len(all) != 2 || all[0].Name != "api-change" || all[1].Version != 2 {
		t.Fatalf("ListLatestTemplates: %+v", all)
	}
}

func testNotify(t *testing.T, s Store) {
	n := s.Notify()
	tok := json.RawMessage(`"ExponentPushToken[abc]"`)
	d := &notify.Device{ID: "d_1", Owner: owner, Provider: "expo", Token: tok, Name: "Pixel", CreatedAt: t0, LastOKAt: t0}
	must(t, n.CreateDevice(ctx, d))
	got, err := n.GetDeviceByToken(ctx, owner, "expo", tok)
	must(t, err)
	if got.ID != "d_1" || string(got.Token) != string(tok) {
		t.Fatalf("device: %+v", got)
	}
	off := t0.Add(time.Hour)
	got.DisabledAt, got.Name = &off, "Pixel 6a"
	must(t, n.UpdateDevice(ctx, got))
	if ds, _ := n.ListActiveDevices(ctx, owner); len(ds) != 0 {
		t.Fatal("disabled devices are not active")
	}
	if g, _ := n.GetDevice(ctx, owner, "d_1"); g.Name != "Pixel 6a" || g.DisabledAt == nil {
		t.Fatal("device update lost")
	}
	last, err := n.LastSentForThread(ctx, owner, "t_1")
	must(t, err)
	if !last.IsZero() {
		t.Fatal("no sends yet")
	}
	for i, st := range []string{notify.StatusSent, notify.StatusFailed} {
		must(t, n.CreateNotification(ctx, &notify.Entry{ID: kernel.NewID("n"), Owner: owner, DeviceID: "d_1", Provider: "expo",
			Title: "x", ThreadID: "t_1", Status: st, CreatedAt: t0.Add(time.Duration(i) * time.Minute)}))
	}
	last, _ = n.LastSentForThread(ctx, owner, "t_1")
	if !last.Equal(t0) {
		t.Fatalf("failed sends do not count: last = %v", last)
	}
	log, err := n.ListNotifications(ctx, owner, 10)
	must(t, err)
	if len(log) != 2 || log[0].Status != notify.StatusFailed {
		t.Fatalf("log newest first: %+v", log)
	}
}

func testSettings(t *testing.T, s Store) {
	st := s.Settings()
	_, err := st.GetSetting(ctx, "vapid")
	wantNotFound(t, err)
	must(t, st.SetSetting(ctx, "vapid", "a"))
	must(t, st.SetSetting(ctx, "vapid", "b"))
	if v, _ := st.GetSetting(ctx, "vapid"); v != "b" {
		t.Fatalf("setting = %q", v)
	}
}
