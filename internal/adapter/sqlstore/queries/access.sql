-- name: GetOwner :one
SELECT * FROM owners ORDER BY created_at LIMIT 1;

-- name: UpsertOwner :exec
INSERT INTO owners (id, password_hash, created_at) VALUES (sqlc.arg(id), sqlc.arg(password_hash), sqlc.arg(created_at))
ON CONFLICT (id) DO UPDATE SET password_hash = excluded.password_hash;

-- name: CreateWebSession :exec
INSERT INTO web_sessions (token_hash, owner_id, created_at, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(owner_id), sqlc.arg(created_at), sqlc.arg(expires_at));

-- name: GetWebSession :one
SELECT * FROM web_sessions WHERE token_hash = sqlc.arg(token_hash);

-- name: DeleteWebSession :exec
DELETE FROM web_sessions WHERE token_hash = sqlc.arg(token_hash);

-- name: CreateMachine :exec
INSERT INTO machines (id, owner_id, name, key_hash, created_at, last_seen_at, removed_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(name), sqlc.arg(key_hash), sqlc.arg(created_at), sqlc.arg(last_seen_at), sqlc.narg(removed_at));

-- name: GetMachine :one
SELECT * FROM machines WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: GetMachineByKeyHash :one
SELECT * FROM machines WHERE key_hash = sqlc.arg(key_hash);

-- name: UpdateMachine :exec
UPDATE machines SET name = sqlc.arg(name), key_hash = sqlc.arg(key_hash), last_seen_at = sqlc.arg(last_seen_at),
  removed_at = sqlc.narg(removed_at)
WHERE id = sqlc.arg(id);

-- name: ListMachines :many
SELECT m.id, m.owner_id, m.name, m.key_hash, m.created_at, m.last_seen_at, m.removed_at,
  (SELECT COUNT(*) FROM agent_sessions s WHERE s.machine_id = m.id AND s.revoked_at IS NULL) AS session_count
FROM machines m
WHERE m.owner_id = sqlc.arg(owner_id) AND m.removed_at IS NULL
ORDER BY m.name;

-- name: CreatePairing :exec
INSERT INTO pairings (id, code, secret_hash, hostname, status, owner_id, machine_id, machine_name, created_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(code), sqlc.arg(secret_hash), sqlc.arg(hostname), sqlc.arg(status), sqlc.arg(owner_id),
  sqlc.arg(machine_id), sqlc.arg(machine_name), sqlc.arg(created_at), sqlc.arg(expires_at));

-- name: GetPairing :one
SELECT * FROM pairings WHERE id = sqlc.arg(id);

-- name: GetPendingPairingByCode :one
SELECT * FROM pairings WHERE code = sqlc.arg(code) AND status = 'pending' ORDER BY created_at DESC LIMIT 1;

-- name: UpdatePairing :exec
UPDATE pairings SET status = sqlc.arg(status), owner_id = sqlc.arg(owner_id), machine_id = sqlc.arg(machine_id),
  machine_name = sqlc.arg(machine_name)
WHERE id = sqlc.arg(id);

-- name: GetAgentSessionByExternal :one
SELECT * FROM agent_sessions
WHERE machine_id = sqlc.arg(machine_id) AND agent = sqlc.arg(agent) AND external_id = sqlc.arg(external_id);

-- name: GetAgentSessionByKeyHash :one
SELECT * FROM agent_sessions WHERE key_hash = sqlc.arg(key_hash);

-- name: GetAgentSession :one
SELECT * FROM agent_sessions WHERE owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id);

-- name: CreateAgentSession :exec
INSERT INTO agent_sessions (id, owner_id, machine_id, agent, external_id, cwd, repo, key_hash, created_at, last_used_at, revoked_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(machine_id), sqlc.arg(agent), sqlc.arg(external_id), sqlc.arg(cwd),
  sqlc.arg(repo), sqlc.arg(key_hash), sqlc.arg(created_at), sqlc.arg(last_used_at), sqlc.narg(revoked_at));

-- name: UpdateAgentSession :exec
UPDATE agent_sessions SET cwd = sqlc.arg(cwd), repo = sqlc.arg(repo), key_hash = sqlc.arg(key_hash),
  last_used_at = sqlc.arg(last_used_at), revoked_at = sqlc.narg(revoked_at)
WHERE id = sqlc.arg(id);
