-- When the owner last looked at each thread; a thread changed since then is unread.
ALTER TABLE threads ADD COLUMN read_at BIGINT NOT NULL DEFAULT 0;
