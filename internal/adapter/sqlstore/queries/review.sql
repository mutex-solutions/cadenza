-- name: CreateThread :exec
INSERT INTO threads (id, owner_id, session_id, machine_id, title, status, next_seq, created_at, updated_at, read_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(session_id), sqlc.arg(machine_id), sqlc.arg(title), sqlc.arg(status),
  sqlc.arg(next_seq), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(read_at));

-- name: GetThread :one
SELECT * FROM threads WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: UpdateThread :exec
UPDATE threads SET session_id = sqlc.arg(session_id), title = sqlc.arg(title), status = sqlc.arg(status),
  next_seq = sqlc.arg(next_seq), updated_at = sqlc.arg(updated_at), read_at = sqlc.arg(read_at)
WHERE id = sqlc.arg(id);

-- name: LatestOpenThread :one
SELECT * FROM threads
WHERE owner_id = sqlc.arg(owner_id) AND session_id = sqlc.arg(session_id) AND status <> 'archived'
ORDER BY updated_at DESC LIMIT 1;

-- name: GetThreadRow :one
SELECT t.id, t.owner_id, t.session_id, t.machine_id, t.title, t.status, t.next_seq, t.created_at, t.updated_at, t.read_at,
  COALESCE(m.name, '') AS machine_name, COALESCE(s.agent, '') AS agent,
  (SELECT COUNT(*) FROM posts p WHERE p.thread_id = t.id) AS post_count,
  (SELECT COUNT(*) FROM questions q WHERE q.thread_id = t.id AND q.status = 'open') AS open_questions
FROM threads t
LEFT JOIN machines m ON m.id = t.machine_id
LEFT JOIN agent_sessions s ON s.id = t.session_id
WHERE t.owner_id = sqlc.arg(owner_id) AND t.id = sqlc.arg(id);

-- name: ListThreadsAll :many
SELECT t.id, t.owner_id, t.session_id, t.machine_id, t.title, t.status, t.next_seq, t.created_at, t.updated_at, t.read_at,
  COALESCE(m.name, '') AS machine_name, COALESCE(s.agent, '') AS agent,
  (SELECT COUNT(*) FROM posts p WHERE p.thread_id = t.id) AS post_count,
  (SELECT COUNT(*) FROM questions q WHERE q.thread_id = t.id AND q.status = 'open') AS open_questions
FROM threads t
LEFT JOIN machines m ON m.id = t.machine_id
LEFT JOIN agent_sessions s ON s.id = t.session_id
WHERE t.owner_id = sqlc.arg(owner_id)
ORDER BY t.updated_at DESC
LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: ListThreadsOpen :many
SELECT t.id, t.owner_id, t.session_id, t.machine_id, t.title, t.status, t.next_seq, t.created_at, t.updated_at, t.read_at,
  COALESCE(m.name, '') AS machine_name, COALESCE(s.agent, '') AS agent,
  (SELECT COUNT(*) FROM posts p WHERE p.thread_id = t.id) AS post_count,
  (SELECT COUNT(*) FROM questions q WHERE q.thread_id = t.id AND q.status = 'open') AS open_questions
FROM threads t
LEFT JOIN machines m ON m.id = t.machine_id
LEFT JOIN agent_sessions s ON s.id = t.session_id
WHERE t.owner_id = sqlc.arg(owner_id) AND t.status <> 'archived'
ORDER BY t.updated_at DESC
LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: ListThreadsOpenOnMachine :many
SELECT t.id, t.owner_id, t.session_id, t.machine_id, t.title, t.status, t.next_seq, t.created_at, t.updated_at, t.read_at,
  COALESCE(m.name, '') AS machine_name, COALESCE(s.agent, '') AS agent,
  (SELECT COUNT(*) FROM posts p WHERE p.thread_id = t.id) AS post_count,
  (SELECT COUNT(*) FROM questions q WHERE q.thread_id = t.id AND q.status = 'open') AS open_questions
FROM threads t
LEFT JOIN machines m ON m.id = t.machine_id
LEFT JOIN agent_sessions s ON s.id = t.session_id
WHERE t.owner_id = sqlc.arg(owner_id) AND t.machine_id = sqlc.arg(machine_id) AND t.status <> 'archived'
ORDER BY t.updated_at DESC
LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: CreatePost :exec
INSERT INTO posts (id, owner_id, thread_id, seq, title, summary, summary_html, session_id, retry_id, created_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(thread_id), sqlc.arg(seq), sqlc.arg(title), sqlc.arg(summary),
  sqlc.arg(summary_html), sqlc.arg(session_id), sqlc.arg(retry_id), sqlc.arg(created_at));

-- name: CreateAttachment :exec
INSERT INTO attachments (post_id, idx, template, version, data, view, file)
VALUES (sqlc.arg(post_id), sqlc.arg(idx), sqlc.arg(template), sqlc.arg(version), sqlc.arg(data), sqlc.arg(view), sqlc.arg(file));

-- name: GetPost :one
SELECT * FROM posts WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: GetPostByRetry :one
SELECT * FROM posts WHERE owner_id = sqlc.arg(owner_id) AND session_id = sqlc.arg(session_id) AND retry_id = sqlc.arg(retry_id);

-- name: ListPosts :many
SELECT * FROM posts WHERE owner_id = sqlc.arg(owner_id) AND thread_id = sqlc.arg(thread_id) ORDER BY seq;

-- name: ListAttachments :many
SELECT * FROM attachments WHERE post_id = sqlc.arg(post_id) ORDER BY idx;

-- name: ListThreadAttachments :many
SELECT a.* FROM attachments a JOIN posts p ON p.id = a.post_id
WHERE p.owner_id = sqlc.arg(owner_id) AND p.thread_id = sqlc.arg(thread_id)
ORDER BY p.seq, a.idx;

-- name: CreateQuestion :exec
INSERT INTO questions (id, owner_id, thread_id, post_id, session_id, text, text_html, choices, waiting, notify_title,
  status, replaced_by, retry_id, created_at, closed_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(thread_id), sqlc.arg(post_id), sqlc.arg(session_id), sqlc.arg(text),
  sqlc.arg(text_html), sqlc.arg(choices), sqlc.arg(waiting), sqlc.arg(notify_title), sqlc.arg(status),
  sqlc.arg(replaced_by), sqlc.arg(retry_id), sqlc.arg(created_at), sqlc.narg(closed_at));

-- name: GetQuestion :one
SELECT * FROM questions WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: GetQuestionByRetry :one
SELECT * FROM questions WHERE owner_id = sqlc.arg(owner_id) AND session_id = sqlc.arg(session_id) AND retry_id = sqlc.arg(retry_id);

-- name: UpdateQuestion :exec
UPDATE questions SET status = sqlc.arg(status), replaced_by = sqlc.arg(replaced_by), closed_at = sqlc.narg(closed_at)
WHERE id = sqlc.arg(id);

-- name: ListQuestions :many
SELECT * FROM questions WHERE owner_id = sqlc.arg(owner_id) AND thread_id = sqlc.arg(thread_id) ORDER BY created_at, id;

-- name: CreateReply :exec
INSERT INTO replies (id, owner_id, thread_id, post_id, question_id, session_id, kind, choice, text, verdict, spot, files,
  extra, seq, created_at, delivered_at, agent_reply, agent_reply_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(thread_id), sqlc.arg(post_id), sqlc.arg(question_id), sqlc.arg(session_id),
  sqlc.arg(kind), sqlc.arg(choice), sqlc.arg(text), sqlc.arg(verdict), sqlc.arg(spot), sqlc.arg(files), sqlc.arg(extra),
  sqlc.arg(seq), sqlc.arg(created_at), sqlc.narg(delivered_at), sqlc.arg(agent_reply), sqlc.narg(agent_reply_at));

-- name: GetReply :one
SELECT * FROM replies WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: UpdateReply :exec
UPDATE replies SET delivered_at = sqlc.narg(delivered_at), agent_reply = sqlc.arg(agent_reply),
  agent_reply_at = sqlc.narg(agent_reply_at)
WHERE id = sqlc.arg(id);

-- name: ListReplies :many
SELECT * FROM replies WHERE owner_id = sqlc.arg(owner_id) AND thread_id = sqlc.arg(thread_id) ORDER BY seq;

-- name: AnswerFor :one
SELECT * FROM replies WHERE owner_id = sqlc.arg(owner_id) AND question_id = sqlc.arg(question_id) AND kind = 'answer'
ORDER BY seq DESC LIMIT 1;

-- name: Undelivered :many
SELECT * FROM replies
WHERE owner_id = sqlc.arg(owner_id) AND session_id = sqlc.arg(session_id) AND seq > sqlc.arg(after_seq) AND delivered_at IS NULL
ORDER BY seq
LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: MarkDelivered :execrows
UPDATE replies SET delivered_at = sqlc.arg(delivered_at)
WHERE owner_id = sqlc.arg(owner_id) AND session_id = sqlc.arg(session_id) AND seq <= sqlc.arg(up_to) AND delivered_at IS NULL;

-- name: MoveUndelivered :exec
UPDATE replies SET session_id = sqlc.arg(to_session_id)
WHERE owner_id = sqlc.arg(owner_id) AND thread_id = sqlc.arg(thread_id) AND delivered_at IS NULL;

-- name: GetCursor :one
SELECT acked_seq FROM session_cursors WHERE session_id = sqlc.arg(session_id);

-- name: SetCursor :exec
INSERT INTO session_cursors (session_id, acked_seq) VALUES (sqlc.arg(session_id), sqlc.arg(acked_seq))
ON CONFLICT (session_id) DO UPDATE SET acked_seq = excluded.acked_seq;
