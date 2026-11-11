-- Cadenza schema, Postgres. Times are unix milliseconds. Keep in step with
-- schema/sqlite/0001_init.sql: the shared queries run on both.

CREATE TABLE owners (
  id            TEXT PRIMARY KEY,
  password_hash TEXT NOT NULL,
  created_at    BIGINT NOT NULL
);

CREATE TABLE web_sessions (
  token_hash TEXT PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL
);

CREATE TABLE machines (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL,
  name         TEXT NOT NULL,
  key_hash     TEXT NOT NULL UNIQUE,
  created_at   BIGINT NOT NULL,
  last_seen_at BIGINT NOT NULL,
  removed_at   BIGINT
);

CREATE TABLE pairings (
  id           TEXT PRIMARY KEY,
  code         TEXT NOT NULL,
  secret_hash  TEXT NOT NULL,
  hostname     TEXT NOT NULL,
  status       TEXT NOT NULL,
  owner_id     TEXT NOT NULL,
  machine_id   TEXT NOT NULL,
  machine_name TEXT NOT NULL,
  created_at   BIGINT NOT NULL,
  expires_at   BIGINT NOT NULL
);
CREATE INDEX pairings_code ON pairings (code, status);

CREATE TABLE agent_sessions (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL,
  machine_id   TEXT NOT NULL,
  agent        TEXT NOT NULL,
  external_id  TEXT NOT NULL,
  cwd          TEXT NOT NULL,
  repo         TEXT NOT NULL,
  key_hash     TEXT NOT NULL UNIQUE,
  created_at   BIGINT NOT NULL,
  last_used_at BIGINT NOT NULL,
  revoked_at   BIGINT,
  UNIQUE (machine_id, agent, external_id)
);

CREATE TABLE threads (
  id         TEXT PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  session_id TEXT NOT NULL,
  machine_id TEXT NOT NULL,
  title      TEXT NOT NULL,
  status     TEXT NOT NULL,
  next_seq   BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE INDEX threads_owner_updated ON threads (owner_id, updated_at);
CREATE INDEX threads_session_updated ON threads (session_id, updated_at);

CREATE TABLE posts (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL,
  thread_id    TEXT NOT NULL,
  seq          BIGINT NOT NULL,
  title        TEXT NOT NULL,
  summary      TEXT NOT NULL,
  summary_html TEXT NOT NULL,
  session_id   TEXT NOT NULL,
  retry_id     TEXT NOT NULL,
  created_at   BIGINT NOT NULL,
  UNIQUE (session_id, retry_id),
  UNIQUE (thread_id, seq)
);

CREATE TABLE attachments (
  post_id   TEXT NOT NULL,
  idx       BIGINT NOT NULL,
  template  TEXT NOT NULL,
  version   BIGINT NOT NULL,
  data      TEXT NOT NULL,
  view      TEXT NOT NULL,
  file      TEXT NOT NULL,
  PRIMARY KEY (post_id, idx)
);

CREATE TABLE files (
  owner_id   TEXT NOT NULL,
  sha256     TEXT NOT NULL,
  size       BIGINT NOT NULL,
  type       TEXT NOT NULL,
  name       TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (owner_id, sha256)
);

CREATE TABLE questions (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL,
  thread_id    TEXT NOT NULL,
  post_id      TEXT NOT NULL,
  session_id   TEXT NOT NULL,
  text         TEXT NOT NULL,
  text_html    TEXT NOT NULL,
  choices      TEXT NOT NULL,
  waiting      BOOLEAN NOT NULL,
  notify_title TEXT NOT NULL,
  status       TEXT NOT NULL,
  replaced_by  TEXT NOT NULL,
  retry_id     TEXT NOT NULL,
  created_at   BIGINT NOT NULL,
  closed_at    BIGINT,
  UNIQUE (session_id, retry_id)
);
CREATE INDEX questions_thread ON questions (thread_id, created_at);

CREATE TABLE replies (
  id             TEXT PRIMARY KEY,
  owner_id       TEXT NOT NULL,
  thread_id      TEXT NOT NULL,
  post_id        TEXT NOT NULL,
  question_id    TEXT NOT NULL,
  session_id     TEXT NOT NULL,
  kind           TEXT NOT NULL,
  choice         TEXT NOT NULL,
  text           TEXT NOT NULL,
  verdict        TEXT NOT NULL,
  spot           TEXT NOT NULL,
  files          TEXT NOT NULL,
  extra          TEXT NOT NULL,
  seq            BIGINT NOT NULL,
  created_at     BIGINT NOT NULL,
  delivered_at   BIGINT,
  agent_reply    TEXT NOT NULL,
  agent_reply_at BIGINT
);
CREATE INDEX replies_session_seq ON replies (session_id, seq);
CREATE INDEX replies_thread_seq ON replies (thread_id, seq);
CREATE INDEX replies_question ON replies (question_id);

CREATE TABLE session_cursors (
  session_id TEXT PRIMARY KEY,
  acked_seq  BIGINT NOT NULL
);

CREATE TABLE events (
  seq        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  kind       TEXT NOT NULL,
  thread_id  TEXT NOT NULL,
  ref_id     TEXT NOT NULL,
  session_id TEXT NOT NULL,
  at         BIGINT NOT NULL
);
CREATE INDEX events_owner_seq ON events (owner_id, seq);

CREATE TABLE templates (
  owner_id   TEXT NOT NULL,
  name       TEXT NOT NULL,
  version    BIGINT NOT NULL,
  source     TEXT NOT NULL,
  definition TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (owner_id, name, version)
);

CREATE TABLE devices (
  id          TEXT PRIMARY KEY,
  owner_id    TEXT NOT NULL,
  provider    TEXT NOT NULL,
  token       TEXT NOT NULL,
  name        TEXT NOT NULL,
  created_at  BIGINT NOT NULL,
  last_ok_at  BIGINT NOT NULL,
  disabled_at BIGINT
);
CREATE INDEX devices_owner ON devices (owner_id);

CREATE TABLE notifications (
  id         TEXT PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  device_id  TEXT NOT NULL,
  provider   TEXT NOT NULL,
  title      TEXT NOT NULL,
  body       TEXT NOT NULL,
  thread_id  TEXT NOT NULL,
  status     TEXT NOT NULL,
  error      TEXT NOT NULL,
  created_at BIGINT NOT NULL
);
CREATE INDEX notifications_owner_created ON notifications (owner_id, created_at);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
