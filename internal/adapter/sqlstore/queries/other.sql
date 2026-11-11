-- name: AppendEvent :one
INSERT INTO events (owner_id, kind, thread_id, ref_id, session_id, at)
VALUES (sqlc.arg(owner_id), sqlc.arg(kind), sqlc.arg(thread_id), sqlc.arg(ref_id), sqlc.arg(session_id), sqlc.arg(at))
RETURNING seq;

-- name: ListEvents :many
SELECT * FROM events WHERE owner_id = sqlc.arg(owner_id) AND seq > sqlc.arg(after_seq) ORDER BY seq LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: OldestEventSeq :one
SELECT CAST(COALESCE(MIN(seq), 0) AS BIGINT) AS oldest FROM events WHERE owner_id = sqlc.arg(owner_id);

-- name: LatestEventSeq :one
SELECT CAST(COALESCE(MAX(seq), 0) AS BIGINT) AS latest FROM events WHERE owner_id = sqlc.arg(owner_id);

-- name: DeleteEventsBefore :execrows
DELETE FROM events WHERE at < sqlc.arg(before);

-- name: GetFile :one
SELECT * FROM files WHERE owner_id = sqlc.arg(owner_id) AND sha256 = sqlc.arg(sha256);

-- name: SaveFile :exec
INSERT INTO files (owner_id, sha256, size, type, name, created_at)
VALUES (sqlc.arg(owner_id), sqlc.arg(sha256), sqlc.arg(size), sqlc.arg(type), sqlc.arg(name), sqlc.arg(created_at))
ON CONFLICT (owner_id, sha256) DO NOTHING;

-- name: CreateTemplate :exec
INSERT INTO templates (owner_id, name, version, source, definition, created_at)
VALUES (sqlc.arg(owner_id), sqlc.arg(name), sqlc.arg(version), sqlc.arg(source), sqlc.arg(definition), sqlc.arg(created_at));

-- name: GetTemplate :one
SELECT * FROM templates WHERE owner_id = sqlc.arg(owner_id) AND name = sqlc.arg(name) AND version = sqlc.arg(version);

-- name: LatestTemplate :one
SELECT * FROM templates WHERE owner_id = sqlc.arg(owner_id) AND name = sqlc.arg(name) ORDER BY version DESC LIMIT 1;

-- name: ListLatestTemplates :many
SELECT t.* FROM templates t
WHERE t.owner_id = sqlc.arg(owner_id)
  AND t.version = (SELECT MAX(t2.version) FROM templates t2 WHERE t2.owner_id = t.owner_id AND t2.name = t.name)
ORDER BY t.name;

-- name: CreateDevice :exec
INSERT INTO devices (id, owner_id, provider, token, name, created_at, last_ok_at, disabled_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(provider), sqlc.arg(token), sqlc.arg(name), sqlc.arg(created_at),
  sqlc.arg(last_ok_at), sqlc.narg(disabled_at));

-- name: GetDevice :one
SELECT * FROM devices WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: GetDeviceByToken :one
SELECT * FROM devices WHERE owner_id = sqlc.arg(owner_id) AND provider = sqlc.arg(provider) AND token = sqlc.arg(token);

-- name: UpdateDevice :exec
UPDATE devices SET name = sqlc.arg(name), last_ok_at = sqlc.arg(last_ok_at), disabled_at = sqlc.narg(disabled_at)
WHERE id = sqlc.arg(id);

-- name: ListActiveDevices :many
SELECT * FROM devices WHERE owner_id = sqlc.arg(owner_id) AND disabled_at IS NULL ORDER BY created_at;

-- name: CreateNotification :exec
INSERT INTO notifications (id, owner_id, device_id, provider, title, body, thread_id, status, error, created_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(device_id), sqlc.arg(provider), sqlc.arg(title), sqlc.arg(body),
  sqlc.arg(thread_id), sqlc.arg(status), sqlc.arg(error), sqlc.arg(created_at));

-- name: ListNotifications :many
SELECT * FROM notifications WHERE owner_id = sqlc.arg(owner_id) ORDER BY created_at DESC LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: LastNotificationForThread :one
SELECT CAST(COALESCE(MAX(created_at), 0) AS BIGINT) AS last_at FROM notifications
WHERE owner_id = sqlc.arg(owner_id) AND thread_id = sqlc.arg(thread_id) AND status = 'sent';

-- name: GetSetting :one
SELECT value FROM settings WHERE key = sqlc.arg(key);

-- name: SetSetting :exec
INSERT INTO settings (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE SET value = excluded.value;
