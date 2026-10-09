-- Each user's account: users.go. An email is unique whatever its case:
-- Ann@example.com is ann@example.com. passkey_handle is the random handle
-- a user's passkeys name them by, made with their first: passkeys.go.
-- photo_key is where their photo is kept on the app's disk: photos.go.
CREATE TABLE users (
	id                INTEGER PRIMARY KEY,
	name              TEXT NOT NULL,
	email             TEXT NOT NULL UNIQUE COLLATE NOCASE,
	email_verified_at DATETIME,
	password_hash     TEXT NOT NULL,
	two_factor_secret TEXT,
	recovery_codes    TEXT,
	two_factor_step   INTEGER NOT NULL DEFAULT 0,
	passkey_handle    BLOB,
	photo_key         TEXT,
	created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX users_passkey_handle ON users (passkey_handle) WHERE passkey_handle IS NOT NULL;
