-- The users' API tokens, their hashes, which a request to /api sends
-- one of in place of a login: tokens.go.
CREATE TABLE access_tokens (
	id           INTEGER PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	token_hash   BLOB NOT NULL UNIQUE,
	abilities    TEXT NOT NULL,
	last_used_at INTEGER,
	expires_at   INTEGER,
	created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX access_tokens_user ON access_tokens (user_id);
