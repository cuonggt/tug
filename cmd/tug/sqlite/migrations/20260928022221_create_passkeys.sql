-- Each user's passkeys, and the random handle they name the user by,
-- made with their first: passkeys.go.
ALTER TABLE users ADD COLUMN passkey_handle BLOB;
CREATE UNIQUE INDEX users_passkey_handle ON users (passkey_handle) WHERE passkey_handle IS NOT NULL;
CREATE TABLE passkeys (
	id            INTEGER PRIMARY KEY,
	user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	credential_id BLOB NOT NULL UNIQUE,
	public_key    BLOB NOT NULL,
	sign_count    INTEGER NOT NULL DEFAULT 0,
	transports    TEXT NOT NULL DEFAULT '',
	backed_up     INTEGER NOT NULL DEFAULT 0,
	name          TEXT NOT NULL,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_used_at  DATETIME
);
CREATE INDEX passkeys_user ON passkeys (user_id);
