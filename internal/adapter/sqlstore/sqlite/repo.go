package sqlite

// This file is the source of postgres/repo.go: `make gen` copies it with the
// package name and import path changed. Edit it here only.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/sqlite/db"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Store is the database. Its domain views (Access, Review, …) satisfy each
// domain's Store interface.
type Store struct {
	db *sql.DB
	q  *db.Queries
}

// DB exposes the underlying handle (for health checks and tests).
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Ping checks the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// repo runs queries on the database or inside one transaction.
type repo struct{ q *db.Queries }

func (s *Store) base() *repo { return &repo{q: s.q} }

func (s *Store) inTx(ctx context.Context, fn func(*repo) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return kernel.Internal(err, "begin transaction")
	}
	if err := fn(&repo{q: s.q.WithTx(tx)}); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return kernel.Internal(err, "commit")
	}
	return nil
}

type accessStore struct {
	*repo
	s *Store
}

func (a accessStore) InTx(ctx context.Context, fn func(access.Repo) error) error {
	return a.s.inTx(ctx, func(r *repo) error { return fn(r) })
}

type reviewStore struct {
	*repo
	s *Store
}

func (a reviewStore) InTx(ctx context.Context, fn func(review.Repo) error) error {
	return a.s.inTx(ctx, func(r *repo) error { return fn(r) })
}

// Access returns the store for package access.
func (s *Store) Access() access.Store { return accessStore{s.base(), s} }

// Review returns the store for package review.
func (s *Store) Review() review.Store { return reviewStore{s.base(), s} }

// Files returns the repo for package files.
func (s *Store) Files() files.Repo { return s.base() }

// Catalog returns the repo for package catalog.
func (s *Store) Catalog() catalog.Repo { return s.base() }

// Notify returns the repo for package notify.
func (s *Store) Notify() notify.Repo { return s.base() }

// Events returns the event log reader.
func (s *Store) Events() eventlog.Reader { return s.base() }

// Settings returns the key-value settings store.
func (s *Store) Settings() sqlstore.Settings { return s.base() }

var (
	_ access.Repo     = (*repo)(nil)
	_ review.Repo     = (*repo)(nil)
	_ files.Repo      = (*repo)(nil)
	_ catalog.Repo    = (*repo)(nil)
	_ notify.Repo     = (*repo)(nil)
	_ eventlog.Reader = (*repo)(nil)
)

// ---- helpers

func notFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return kernel.NotFound("%s not found", what)
	}
	if err != nil {
		return kernel.Internal(err, "read %s", what)
	}
	return nil
}

func wrap(err error, what string) error {
	if err != nil {
		return kernel.Internal(err, "%s", what)
	}
	return nil
}

func ms(t time.Time) int64 { return kernel.Millis(t) }

func tm(v int64) time.Time { return kernel.FromMillis(v) }

func nullMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: kernel.Millis(*t), Valid: true}
}

func nullTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := kernel.FromMillis(v.Int64)
	return &t
}

// toJSON encodes v, or "" for nil values, so empty columns stay empty.
func toJSON(v any) string {
	switch x := v.(type) {
	case json.RawMessage:
		return string(x)
	case *spec.Spot:
		if x == nil {
			return ""
		}
	case *media.FileRef:
		if x == nil {
			return ""
		}
	case []media.FileRef:
		if len(x) == 0 {
			return ""
		}
	case []string:
		if len(x) == 0 {
			return ""
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func fromJSON[T any](s string) (T, error) {
	var v T
	if s == "" {
		return v, nil
	}
	err := json.Unmarshal([]byte(s), &v)
	return v, err
}

func raw(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

// ---- access

func (r *repo) GetOwner(ctx context.Context) (*access.Owner, error) {
	o, err := r.q.GetOwner(ctx)
	if err := notFound(err, "owner"); err != nil {
		return nil, err
	}
	return &access.Owner{ID: kernel.OwnerID(o.ID), PasswordHash: o.PasswordHash, CreatedAt: tm(o.CreatedAt)}, nil
}

func (r *repo) SaveOwner(ctx context.Context, o *access.Owner) error {
	return wrap(r.q.UpsertOwner(ctx, db.UpsertOwnerParams{ID: string(o.ID), PasswordHash: o.PasswordHash, CreatedAt: ms(o.CreatedAt)}), "save owner")
}

func (r *repo) CreateWebSession(ctx context.Context, s *access.WebSession) error {
	return wrap(r.q.CreateWebSession(ctx, db.CreateWebSessionParams{TokenHash: s.TokenHash, OwnerID: string(s.Owner),
		CreatedAt: ms(s.CreatedAt), ExpiresAt: ms(s.ExpiresAt)}), "create web session")
}

func (r *repo) GetWebSession(ctx context.Context, tokenHash string) (*access.WebSession, error) {
	s, err := r.q.GetWebSession(ctx, tokenHash)
	if err := notFound(err, "web session"); err != nil {
		return nil, err
	}
	return &access.WebSession{TokenHash: s.TokenHash, Owner: kernel.OwnerID(s.OwnerID), CreatedAt: tm(s.CreatedAt), ExpiresAt: tm(s.ExpiresAt)}, nil
}

func (r *repo) DeleteWebSession(ctx context.Context, tokenHash string) error {
	return wrap(r.q.DeleteWebSession(ctx, tokenHash), "delete web session")
}

func toMachine(m db.Machine) *access.Machine {
	return &access.Machine{ID: m.ID, Owner: kernel.OwnerID(m.OwnerID), Name: m.Name, KeyHash: m.KeyHash,
		CreatedAt: tm(m.CreatedAt), LastSeenAt: tm(m.LastSeenAt), RemovedAt: nullTime(m.RemovedAt)}
}

func (r *repo) CreateMachine(ctx context.Context, m *access.Machine) error {
	return wrap(r.q.CreateMachine(ctx, db.CreateMachineParams{ID: m.ID, OwnerID: string(m.Owner), Name: m.Name, KeyHash: m.KeyHash,
		CreatedAt: ms(m.CreatedAt), LastSeenAt: ms(m.LastSeenAt), RemovedAt: nullMS(m.RemovedAt)}), "create machine")
}

func (r *repo) GetMachine(ctx context.Context, owner kernel.OwnerID, id string) (*access.Machine, error) {
	m, err := r.q.GetMachine(ctx, db.GetMachineParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "machine"); err != nil {
		return nil, err
	}
	return toMachine(m), nil
}

func (r *repo) GetMachineByKeyHash(ctx context.Context, keyHash string) (*access.Machine, error) {
	m, err := r.q.GetMachineByKeyHash(ctx, keyHash)
	if err := notFound(err, "machine"); err != nil {
		return nil, err
	}
	return toMachine(m), nil
}

func (r *repo) UpdateMachine(ctx context.Context, m *access.Machine) error {
	return wrap(r.q.UpdateMachine(ctx, db.UpdateMachineParams{ID: m.ID, Name: m.Name, KeyHash: m.KeyHash,
		LastSeenAt: ms(m.LastSeenAt), RemovedAt: nullMS(m.RemovedAt)}), "update machine")
}

func (r *repo) ListMachines(ctx context.Context, owner kernel.OwnerID) ([]access.MachineRow, error) {
	rows, err := r.q.ListMachines(ctx, string(owner))
	if err != nil {
		return nil, wrap(err, "list machines")
	}
	out := make([]access.MachineRow, 0, len(rows))
	for _, m := range rows {
		mm := toMachine(db.Machine{ID: m.ID, OwnerID: m.OwnerID, Name: m.Name, KeyHash: m.KeyHash, CreatedAt: m.CreatedAt,
			LastSeenAt: m.LastSeenAt, RemovedAt: m.RemovedAt})
		out = append(out, access.MachineRow{Machine: *mm, Sessions: int(m.SessionCount)})
	}
	return out, nil
}

func toPairing(p db.Pairing) *access.Pairing {
	return &access.Pairing{ID: p.ID, Code: p.Code, SecretHash: p.SecretHash, Hostname: p.Hostname, Status: p.Status,
		Owner: kernel.OwnerID(p.OwnerID), MachineID: p.MachineID, MachineName: p.MachineName, CreatedAt: tm(p.CreatedAt), ExpiresAt: tm(p.ExpiresAt)}
}

func (r *repo) CreatePairing(ctx context.Context, p *access.Pairing) error {
	return wrap(r.q.CreatePairing(ctx, db.CreatePairingParams{ID: p.ID, Code: p.Code, SecretHash: p.SecretHash, Hostname: p.Hostname,
		Status: p.Status, OwnerID: string(p.Owner), MachineID: p.MachineID, MachineName: p.MachineName,
		CreatedAt: ms(p.CreatedAt), ExpiresAt: ms(p.ExpiresAt)}), "create pairing")
}

func (r *repo) GetPairing(ctx context.Context, id string) (*access.Pairing, error) {
	p, err := r.q.GetPairing(ctx, id)
	if err := notFound(err, "pairing"); err != nil {
		return nil, err
	}
	return toPairing(p), nil
}

func (r *repo) GetPendingPairingByCode(ctx context.Context, code string) (*access.Pairing, error) {
	p, err := r.q.GetPendingPairingByCode(ctx, code)
	if err := notFound(err, "pairing"); err != nil {
		return nil, err
	}
	return toPairing(p), nil
}

func (r *repo) UpdatePairing(ctx context.Context, p *access.Pairing) error {
	return wrap(r.q.UpdatePairing(ctx, db.UpdatePairingParams{ID: p.ID, Status: p.Status, OwnerID: string(p.Owner),
		MachineID: p.MachineID, MachineName: p.MachineName}), "update pairing")
}

func toSession(s db.AgentSession) *access.AgentSession {
	return &access.AgentSession{ID: s.ID, Owner: kernel.OwnerID(s.OwnerID), MachineID: s.MachineID, Agent: s.Agent,
		ExternalID: s.ExternalID, Cwd: s.Cwd, Repo: s.Repo, KeyHash: s.KeyHash, CreatedAt: tm(s.CreatedAt),
		LastUsedAt: tm(s.LastUsedAt), RevokedAt: nullTime(s.RevokedAt)}
}

func (r *repo) GetAgentSessionByExternal(ctx context.Context, machineID, agent, externalID string) (*access.AgentSession, error) {
	s, err := r.q.GetAgentSessionByExternal(ctx, db.GetAgentSessionByExternalParams{MachineID: machineID, Agent: agent, ExternalID: externalID})
	if err := notFound(err, "agent session"); err != nil {
		return nil, err
	}
	return toSession(s), nil
}

func (r *repo) GetAgentSessionByKeyHash(ctx context.Context, keyHash string) (*access.AgentSession, error) {
	s, err := r.q.GetAgentSessionByKeyHash(ctx, keyHash)
	if err := notFound(err, "agent session"); err != nil {
		return nil, err
	}
	return toSession(s), nil
}

func (r *repo) GetAgentSession(ctx context.Context, owner kernel.OwnerID, id string) (*access.AgentSession, error) {
	s, err := r.q.GetAgentSession(ctx, db.GetAgentSessionParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "agent session"); err != nil {
		return nil, err
	}
	return toSession(s), nil
}

func (r *repo) CreateAgentSession(ctx context.Context, s *access.AgentSession) error {
	return wrap(r.q.CreateAgentSession(ctx, db.CreateAgentSessionParams{ID: s.ID, OwnerID: string(s.Owner), MachineID: s.MachineID,
		Agent: s.Agent, ExternalID: s.ExternalID, Cwd: s.Cwd, Repo: s.Repo, KeyHash: s.KeyHash, CreatedAt: ms(s.CreatedAt),
		LastUsedAt: ms(s.LastUsedAt), RevokedAt: nullMS(s.RevokedAt)}), "create agent session")
}

func (r *repo) UpdateAgentSession(ctx context.Context, s *access.AgentSession) error {
	return wrap(r.q.UpdateAgentSession(ctx, db.UpdateAgentSessionParams{ID: s.ID, Cwd: s.Cwd, Repo: s.Repo, KeyHash: s.KeyHash,
		LastUsedAt: ms(s.LastUsedAt), RevokedAt: nullMS(s.RevokedAt)}), "update agent session")
}

// ---- events

func (r *repo) AppendEvent(ctx context.Context, e *eventlog.Event) error {
	seq, err := r.q.AppendEvent(ctx, db.AppendEventParams{OwnerID: string(e.Owner), Kind: e.Kind, ThreadID: e.ThreadID,
		RefID: e.RefID, SessionID: e.SessionID, At: ms(e.At)})
	if err != nil {
		return wrap(err, "append event")
	}
	e.Seq = seq
	return nil
}

func (r *repo) ListEvents(ctx context.Context, owner kernel.OwnerID, after int64, limit int) ([]eventlog.Event, error) {
	rows, err := r.q.ListEvents(ctx, db.ListEventsParams{OwnerID: string(owner), AfterSeq: after, RowLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list events")
	}
	out := make([]eventlog.Event, 0, len(rows))
	for _, e := range rows {
		out = append(out, eventlog.Event{Seq: e.Seq, Owner: kernel.OwnerID(e.OwnerID), Kind: e.Kind, ThreadID: e.ThreadID,
			RefID: e.RefID, SessionID: e.SessionID, At: tm(e.At)})
	}
	return out, nil
}

func (r *repo) OldestSeq(ctx context.Context, owner kernel.OwnerID) (int64, error) {
	v, err := r.q.OldestEventSeq(ctx, string(owner))
	return v, wrap(err, "oldest event")
}

func (r *repo) LatestSeq(ctx context.Context, owner kernel.OwnerID) (int64, error) {
	v, err := r.q.LatestEventSeq(ctx, string(owner))
	return v, wrap(err, "latest event")
}

func (r *repo) DeleteEventsBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.q.DeleteEventsBefore(ctx, ms(before))
	return n, wrap(err, "prune events")
}

// ---- review: threads

func toThread(t db.Thread) *review.Thread {
	return &review.Thread{ID: t.ID, Owner: kernel.OwnerID(t.OwnerID), SessionID: t.SessionID, MachineID: t.MachineID,
		Title: t.Title, Status: t.Status, NextSeq: int(t.NextSeq), CreatedAt: tm(t.CreatedAt), UpdatedAt: tm(t.UpdatedAt)}
}

func toRow(r db.GetThreadRowRow) review.ThreadRow {
	t := toThread(db.Thread{ID: r.ID, OwnerID: r.OwnerID, SessionID: r.SessionID, MachineID: r.MachineID, Title: r.Title,
		Status: r.Status, NextSeq: r.NextSeq, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	return review.ThreadRow{Thread: *t, MachineName: r.MachineName, Agent: r.Agent, Posts: int(r.PostCount), OpenQuestions: int(r.OpenQuestions)}
}

func (r *repo) CreateThread(ctx context.Context, t *review.Thread) error {
	return wrap(r.q.CreateThread(ctx, db.CreateThreadParams{ID: t.ID, OwnerID: string(t.Owner), SessionID: t.SessionID,
		MachineID: t.MachineID, Title: t.Title, Status: t.Status, NextSeq: int64(t.NextSeq), CreatedAt: ms(t.CreatedAt),
		UpdatedAt: ms(t.UpdatedAt)}), "create thread")
}

func (r *repo) GetThread(ctx context.Context, owner kernel.OwnerID, id string) (*review.Thread, error) {
	t, err := r.q.GetThread(ctx, db.GetThreadParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "thread"); err != nil {
		return nil, err
	}
	return toThread(t), nil
}

func (r *repo) UpdateThread(ctx context.Context, t *review.Thread) error {
	return wrap(r.q.UpdateThread(ctx, db.UpdateThreadParams{ID: t.ID, SessionID: t.SessionID, Title: t.Title, Status: t.Status,
		NextSeq: int64(t.NextSeq), UpdatedAt: ms(t.UpdatedAt)}), "update thread")
}

func (r *repo) LatestOpenThread(ctx context.Context, owner kernel.OwnerID, sessionID string) (*review.Thread, error) {
	t, err := r.q.LatestOpenThread(ctx, db.LatestOpenThreadParams{OwnerID: string(owner), SessionID: sessionID})
	if err := notFound(err, "thread"); err != nil {
		return nil, err
	}
	return toThread(t), nil
}

func (r *repo) GetThreadRow(ctx context.Context, owner kernel.OwnerID, id string) (*review.ThreadRow, error) {
	row, err := r.q.GetThreadRow(ctx, db.GetThreadRowParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "thread"); err != nil {
		return nil, err
	}
	out := toRow(row)
	return &out, nil
}

func (r *repo) ListThreads(ctx context.Context, owner kernel.OwnerID, machineID string, includeArchived bool, limit int) ([]review.ThreadRow, error) {
	var rows []db.GetThreadRowRow
	switch {
	case machineID != "":
		rs, err := r.q.ListThreadsOpenOnMachine(ctx, db.ListThreadsOpenOnMachineParams{OwnerID: string(owner), MachineID: machineID, RowLimit: int64(limit)})
		if err != nil {
			return nil, wrap(err, "list threads")
		}
		for _, x := range rs {
			rows = append(rows, db.GetThreadRowRow(x))
		}
	case includeArchived:
		rs, err := r.q.ListThreadsAll(ctx, db.ListThreadsAllParams{OwnerID: string(owner), RowLimit: int64(limit)})
		if err != nil {
			return nil, wrap(err, "list threads")
		}
		for _, x := range rs {
			rows = append(rows, db.GetThreadRowRow(x))
		}
	default:
		rs, err := r.q.ListThreadsOpen(ctx, db.ListThreadsOpenParams{OwnerID: string(owner), RowLimit: int64(limit)})
		if err != nil {
			return nil, wrap(err, "list threads")
		}
		for _, x := range rs {
			rows = append(rows, db.GetThreadRowRow(x))
		}
	}
	out := make([]review.ThreadRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, toRow(x))
	}
	return out, nil
}

// ---- review: posts

func toPost(p db.Post) review.Post {
	return review.Post{ID: p.ID, Owner: kernel.OwnerID(p.OwnerID), ThreadID: p.ThreadID, Seq: int(p.Seq), Title: p.Title,
		Summary: p.Summary, SummaryHTML: p.SummaryHtml, SessionID: p.SessionID, RetryID: p.RetryID, CreatedAt: tm(p.CreatedAt)}
}

func toAttachment(a db.Attachment) (review.Attachment, error) {
	f, err := fromJSON[*media.FileRef](a.File)
	if err != nil {
		return review.Attachment{}, kernel.Internal(err, "decode attachment file")
	}
	return review.Attachment{Index: int(a.Idx), Template: a.Template, Version: int(a.Version), Data: raw(a.Data), View: raw(a.View), File: f}, nil
}

func (r *repo) CreatePost(ctx context.Context, p *review.Post) error {
	if err := r.q.CreatePost(ctx, db.CreatePostParams{ID: p.ID, OwnerID: string(p.Owner), ThreadID: p.ThreadID, Seq: int64(p.Seq),
		Title: p.Title, Summary: p.Summary, SummaryHtml: p.SummaryHTML, SessionID: p.SessionID, RetryID: p.RetryID,
		CreatedAt: ms(p.CreatedAt)}); err != nil {
		return wrap(err, "create post")
	}
	for _, a := range p.Attachments {
		if err := r.q.CreateAttachment(ctx, db.CreateAttachmentParams{PostID: p.ID, Idx: int64(a.Index), Template: a.Template,
			Version: int64(a.Version), Data: string(a.Data), View: string(a.View), File: toJSON(a.File)}); err != nil {
			return wrap(err, "create attachment")
		}
	}
	return nil
}

func (r *repo) withAttachments(ctx context.Context, p db.Post) (*review.Post, error) {
	post := toPost(p)
	atts, err := r.q.ListAttachments(ctx, p.ID)
	if err != nil {
		return nil, wrap(err, "list attachments")
	}
	for _, a := range atts {
		att, err := toAttachment(a)
		if err != nil {
			return nil, err
		}
		post.Attachments = append(post.Attachments, att)
	}
	return &post, nil
}

func (r *repo) GetPost(ctx context.Context, owner kernel.OwnerID, id string) (*review.Post, error) {
	p, err := r.q.GetPost(ctx, db.GetPostParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "post"); err != nil {
		return nil, err
	}
	return r.withAttachments(ctx, p)
}

func (r *repo) GetPostByRetry(ctx context.Context, owner kernel.OwnerID, sessionID, retryID string) (*review.Post, error) {
	p, err := r.q.GetPostByRetry(ctx, db.GetPostByRetryParams{OwnerID: string(owner), SessionID: sessionID, RetryID: retryID})
	if err := notFound(err, "post"); err != nil {
		return nil, err
	}
	return r.withAttachments(ctx, p)
}

func (r *repo) ListPosts(ctx context.Context, owner kernel.OwnerID, threadID string) ([]review.Post, error) {
	ps, err := r.q.ListPosts(ctx, db.ListPostsParams{OwnerID: string(owner), ThreadID: threadID})
	if err != nil {
		return nil, wrap(err, "list posts")
	}
	atts, err := r.q.ListThreadAttachments(ctx, db.ListThreadAttachmentsParams{OwnerID: string(owner), ThreadID: threadID})
	if err != nil {
		return nil, wrap(err, "list attachments")
	}
	byPost := map[string][]review.Attachment{}
	for _, a := range atts {
		att, err := toAttachment(a)
		if err != nil {
			return nil, err
		}
		byPost[a.PostID] = append(byPost[a.PostID], att)
	}
	out := make([]review.Post, 0, len(ps))
	for _, p := range ps {
		post := toPost(p)
		post.Attachments = byPost[p.ID]
		out = append(out, post)
	}
	return out, nil
}

// ---- review: questions

func toQuestion(q db.Question) (*review.Question, error) {
	choices, err := fromJSON[[]string](q.Choices)
	if err != nil {
		return nil, kernel.Internal(err, "decode choices")
	}
	return &review.Question{ID: q.ID, Owner: kernel.OwnerID(q.OwnerID), ThreadID: q.ThreadID, PostID: q.PostID, SessionID: q.SessionID,
		Text: q.Text, TextHTML: q.TextHtml, Choices: choices, Waiting: q.Waiting, NotifyTitle: q.NotifyTitle, Status: q.Status,
		ReplacedBy: q.ReplacedBy, RetryID: q.RetryID, CreatedAt: tm(q.CreatedAt), ClosedAt: nullTime(q.ClosedAt)}, nil
}

func (r *repo) CreateQuestion(ctx context.Context, q *review.Question) error {
	return wrap(r.q.CreateQuestion(ctx, db.CreateQuestionParams{ID: q.ID, OwnerID: string(q.Owner), ThreadID: q.ThreadID, PostID: q.PostID,
		SessionID: q.SessionID, Text: q.Text, TextHtml: q.TextHTML, Choices: toJSON(q.Choices), Waiting: q.Waiting,
		NotifyTitle: q.NotifyTitle, Status: q.Status, ReplacedBy: q.ReplacedBy, RetryID: q.RetryID, CreatedAt: ms(q.CreatedAt),
		ClosedAt: nullMS(q.ClosedAt)}), "create question")
}

func (r *repo) GetQuestion(ctx context.Context, owner kernel.OwnerID, id string) (*review.Question, error) {
	q, err := r.q.GetQuestion(ctx, db.GetQuestionParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "question"); err != nil {
		return nil, err
	}
	return toQuestion(q)
}

func (r *repo) GetQuestionByRetry(ctx context.Context, owner kernel.OwnerID, sessionID, retryID string) (*review.Question, error) {
	q, err := r.q.GetQuestionByRetry(ctx, db.GetQuestionByRetryParams{OwnerID: string(owner), SessionID: sessionID, RetryID: retryID})
	if err := notFound(err, "question"); err != nil {
		return nil, err
	}
	return toQuestion(q)
}

func (r *repo) UpdateQuestion(ctx context.Context, q *review.Question) error {
	return wrap(r.q.UpdateQuestion(ctx, db.UpdateQuestionParams{ID: q.ID, Status: q.Status, ReplacedBy: q.ReplacedBy,
		ClosedAt: nullMS(q.ClosedAt)}), "update question")
}

func (r *repo) ListQuestions(ctx context.Context, owner kernel.OwnerID, threadID string) ([]review.Question, error) {
	qs, err := r.q.ListQuestions(ctx, db.ListQuestionsParams{OwnerID: string(owner), ThreadID: threadID})
	if err != nil {
		return nil, wrap(err, "list questions")
	}
	out := make([]review.Question, 0, len(qs))
	for _, q := range qs {
		qq, err := toQuestion(q)
		if err != nil {
			return nil, err
		}
		out = append(out, *qq)
	}
	return out, nil
}

// ---- review: replies

func toReply(x db.Reply) (*review.Reply, error) {
	spot, err := fromJSON[*spec.Spot](x.Spot)
	if err != nil {
		return nil, kernel.Internal(err, "decode spot")
	}
	fs, err := fromJSON[[]media.FileRef](x.Files)
	if err != nil {
		return nil, kernel.Internal(err, "decode files")
	}
	return &review.Reply{ID: x.ID, Owner: kernel.OwnerID(x.OwnerID), ThreadID: x.ThreadID, PostID: x.PostID, QuestionID: x.QuestionID,
		SessionID: x.SessionID, Kind: x.Kind, Choice: x.Choice, Text: x.Text, Verdict: x.Verdict, Spot: spot, Files: fs,
		Extra: raw(x.Extra), Seq: x.Seq, CreatedAt: tm(x.CreatedAt), DeliveredAt: nullTime(x.DeliveredAt),
		AgentReply: x.AgentReply, AgentReplyAt: nullTime(x.AgentReplyAt)}, nil
}

func toReplies(xs []db.Reply) ([]review.Reply, error) {
	out := make([]review.Reply, 0, len(xs))
	for _, x := range xs {
		rr, err := toReply(x)
		if err != nil {
			return nil, err
		}
		out = append(out, *rr)
	}
	return out, nil
}

func (r *repo) CreateReply(ctx context.Context, x *review.Reply) error {
	return wrap(r.q.CreateReply(ctx, db.CreateReplyParams{ID: x.ID, OwnerID: string(x.Owner), ThreadID: x.ThreadID, PostID: x.PostID,
		QuestionID: x.QuestionID, SessionID: x.SessionID, Kind: x.Kind, Choice: x.Choice, Text: x.Text, Verdict: x.Verdict,
		Spot: toJSON(x.Spot), Files: toJSON(x.Files), Extra: string(x.Extra), Seq: x.Seq, CreatedAt: ms(x.CreatedAt),
		DeliveredAt: nullMS(x.DeliveredAt), AgentReply: x.AgentReply, AgentReplyAt: nullMS(x.AgentReplyAt)}), "create reply")
}

func (r *repo) GetReply(ctx context.Context, owner kernel.OwnerID, id string) (*review.Reply, error) {
	x, err := r.q.GetReply(ctx, db.GetReplyParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "reply"); err != nil {
		return nil, err
	}
	return toReply(x)
}

func (r *repo) UpdateReply(ctx context.Context, x *review.Reply) error {
	return wrap(r.q.UpdateReply(ctx, db.UpdateReplyParams{ID: x.ID, DeliveredAt: nullMS(x.DeliveredAt), AgentReply: x.AgentReply,
		AgentReplyAt: nullMS(x.AgentReplyAt)}), "update reply")
}

func (r *repo) ListReplies(ctx context.Context, owner kernel.OwnerID, threadID string) ([]review.Reply, error) {
	xs, err := r.q.ListReplies(ctx, db.ListRepliesParams{OwnerID: string(owner), ThreadID: threadID})
	if err != nil {
		return nil, wrap(err, "list replies")
	}
	return toReplies(xs)
}

func (r *repo) AnswerFor(ctx context.Context, owner kernel.OwnerID, questionID string) (*review.Reply, error) {
	x, err := r.q.AnswerFor(ctx, db.AnswerForParams{OwnerID: string(owner), QuestionID: questionID})
	if err := notFound(err, "answer"); err != nil {
		return nil, err
	}
	return toReply(x)
}

func (r *repo) Undelivered(ctx context.Context, owner kernel.OwnerID, sessionID string, after int64, limit int) ([]review.Reply, error) {
	xs, err := r.q.Undelivered(ctx, db.UndeliveredParams{OwnerID: string(owner), SessionID: sessionID, AfterSeq: after, RowLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list undelivered")
	}
	return toReplies(xs)
}

func (r *repo) MarkDelivered(ctx context.Context, owner kernel.OwnerID, sessionID string, upTo int64, at time.Time) (int, error) {
	n, err := r.q.MarkDelivered(ctx, db.MarkDeliveredParams{OwnerID: string(owner), SessionID: sessionID, UpTo: upTo,
		DeliveredAt: sql.NullInt64{Int64: ms(at), Valid: true}})
	return int(n), wrap(err, "mark delivered")
}

func (r *repo) MoveUndelivered(ctx context.Context, owner kernel.OwnerID, threadID, toSessionID string) error {
	return wrap(r.q.MoveUndelivered(ctx, db.MoveUndeliveredParams{OwnerID: string(owner), ThreadID: threadID, ToSessionID: toSessionID}), "move replies")
}

func (r *repo) GetCursor(ctx context.Context, sessionID string) (int64, error) {
	v, err := r.q.GetCursor(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, wrap(err, "get cursor")
}

func (r *repo) SetCursor(ctx context.Context, sessionID string, seq int64) error {
	return wrap(r.q.SetCursor(ctx, db.SetCursorParams{SessionID: sessionID, AckedSeq: seq}), "set cursor")
}

// ---- files

func (r *repo) GetFile(ctx context.Context, owner kernel.OwnerID, sha256 string) (*files.Record, error) {
	f, err := r.q.GetFile(ctx, db.GetFileParams{OwnerID: string(owner), Sha256: sha256})
	if err := notFound(err, "file"); err != nil {
		return nil, err
	}
	return &files.Record{Owner: kernel.OwnerID(f.OwnerID), SHA256: f.Sha256, Size: f.Size, Type: f.Type, Name: f.Name, CreatedAt: tm(f.CreatedAt)}, nil
}

func (r *repo) SaveFile(ctx context.Context, f *files.Record) error {
	return wrap(r.q.SaveFile(ctx, db.SaveFileParams{OwnerID: string(f.Owner), Sha256: f.SHA256, Size: f.Size, Type: f.Type,
		Name: f.Name, CreatedAt: ms(f.CreatedAt)}), "save file")
}

// ---- catalog

func toStored(t db.Template) catalog.Stored {
	return catalog.Stored{Owner: kernel.OwnerID(t.OwnerID), Name: t.Name, Version: int(t.Version), Source: t.Source,
		Definition: raw(t.Definition), CreatedAt: tm(t.CreatedAt)}
}

func (r *repo) CreateTemplate(ctx context.Context, t *catalog.Stored) error {
	return wrap(r.q.CreateTemplate(ctx, db.CreateTemplateParams{OwnerID: string(t.Owner), Name: t.Name, Version: int64(t.Version),
		Source: t.Source, Definition: string(t.Definition), CreatedAt: ms(t.CreatedAt)}), "create template")
}

func (r *repo) GetTemplate(ctx context.Context, owner kernel.OwnerID, name string, version int) (*catalog.Stored, error) {
	t, err := r.q.GetTemplate(ctx, db.GetTemplateParams{OwnerID: string(owner), Name: name, Version: int64(version)})
	if err := notFound(err, "template"); err != nil {
		return nil, err
	}
	s := toStored(t)
	return &s, nil
}

func (r *repo) LatestTemplate(ctx context.Context, owner kernel.OwnerID, name string) (*catalog.Stored, error) {
	t, err := r.q.LatestTemplate(ctx, db.LatestTemplateParams{OwnerID: string(owner), Name: name})
	if err := notFound(err, "template"); err != nil {
		return nil, err
	}
	s := toStored(t)
	return &s, nil
}

func (r *repo) ListLatestTemplates(ctx context.Context, owner kernel.OwnerID) ([]catalog.Stored, error) {
	ts, err := r.q.ListLatestTemplates(ctx, string(owner))
	if err != nil {
		return nil, wrap(err, "list templates")
	}
	out := make([]catalog.Stored, 0, len(ts))
	for _, t := range ts {
		out = append(out, toStored(t))
	}
	return out, nil
}

// ---- notify

func toDevice(d db.Device) notify.Device {
	return notify.Device{ID: d.ID, Owner: kernel.OwnerID(d.OwnerID), Provider: d.Provider, Token: raw(d.Token), Name: d.Name,
		CreatedAt: tm(d.CreatedAt), LastOKAt: tm(d.LastOkAt), DisabledAt: nullTime(d.DisabledAt)}
}

func (r *repo) CreateDevice(ctx context.Context, d *notify.Device) error {
	return wrap(r.q.CreateDevice(ctx, db.CreateDeviceParams{ID: d.ID, OwnerID: string(d.Owner), Provider: d.Provider,
		Token: string(d.Token), Name: d.Name, CreatedAt: ms(d.CreatedAt), LastOkAt: ms(d.LastOKAt), DisabledAt: nullMS(d.DisabledAt)}), "create device")
}

func (r *repo) GetDevice(ctx context.Context, owner kernel.OwnerID, id string) (*notify.Device, error) {
	d, err := r.q.GetDevice(ctx, db.GetDeviceParams{OwnerID: string(owner), ID: id})
	if err := notFound(err, "device"); err != nil {
		return nil, err
	}
	dd := toDevice(d)
	return &dd, nil
}

func (r *repo) GetDeviceByToken(ctx context.Context, owner kernel.OwnerID, provider string, token json.RawMessage) (*notify.Device, error) {
	d, err := r.q.GetDeviceByToken(ctx, db.GetDeviceByTokenParams{OwnerID: string(owner), Provider: provider, Token: string(token)})
	if err := notFound(err, "device"); err != nil {
		return nil, err
	}
	dd := toDevice(d)
	return &dd, nil
}

func (r *repo) UpdateDevice(ctx context.Context, d *notify.Device) error {
	return wrap(r.q.UpdateDevice(ctx, db.UpdateDeviceParams{ID: d.ID, Name: d.Name, LastOkAt: ms(d.LastOKAt), DisabledAt: nullMS(d.DisabledAt)}), "update device")
}

func (r *repo) ListActiveDevices(ctx context.Context, owner kernel.OwnerID) ([]notify.Device, error) {
	ds, err := r.q.ListActiveDevices(ctx, string(owner))
	if err != nil {
		return nil, wrap(err, "list devices")
	}
	out := make([]notify.Device, 0, len(ds))
	for _, d := range ds {
		out = append(out, toDevice(d))
	}
	return out, nil
}

func (r *repo) CreateNotification(ctx context.Context, e *notify.Entry) error {
	return wrap(r.q.CreateNotification(ctx, db.CreateNotificationParams{ID: e.ID, OwnerID: string(e.Owner), DeviceID: e.DeviceID,
		Provider: e.Provider, Title: e.Title, Body: e.Body, ThreadID: e.ThreadID, Status: e.Status, Error: e.Error,
		CreatedAt: ms(e.CreatedAt)}), "log notification")
}

func (r *repo) ListNotifications(ctx context.Context, owner kernel.OwnerID, limit int) ([]notify.Entry, error) {
	ns, err := r.q.ListNotifications(ctx, db.ListNotificationsParams{OwnerID: string(owner), RowLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list notifications")
	}
	out := make([]notify.Entry, 0, len(ns))
	for _, n := range ns {
		out = append(out, notify.Entry{ID: n.ID, Owner: kernel.OwnerID(n.OwnerID), DeviceID: n.DeviceID, Provider: n.Provider,
			Title: n.Title, Body: n.Body, ThreadID: n.ThreadID, Status: n.Status, Error: n.Error, CreatedAt: tm(n.CreatedAt)})
	}
	return out, nil
}

func (r *repo) LastSentForThread(ctx context.Context, owner kernel.OwnerID, threadID string) (time.Time, error) {
	v, err := r.q.LastNotificationForThread(ctx, db.LastNotificationForThreadParams{OwnerID: string(owner), ThreadID: threadID})
	if err != nil {
		return time.Time{}, wrap(err, "last notification")
	}
	if v == 0 {
		return time.Time{}, nil
	}
	return tm(v), nil
}

// ---- settings

// GetSetting returns a stored setting, or a NotFound error.
func (r *repo) GetSetting(ctx context.Context, key string) (string, error) {
	v, err := r.q.GetSetting(ctx, key)
	if err := notFound(err, "setting "+key); err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting stores a setting.
func (r *repo) SetSetting(ctx context.Context, key, value string) error {
	return wrap(r.q.SetSetting(ctx, db.SetSettingParams{Key: key, Value: value}), "set setting")
}
