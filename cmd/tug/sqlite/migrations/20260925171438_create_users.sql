-- Each user's account: users.go. An email is unique whatever its case:
-- Ann@example.com is ann@example.com.
CREATE TABLE users (
	id                INTEGER PRIMARY KEY,
	name              TEXT NOT NULL,
	email             TEXT NOT NULL UNIQUE COLLATE NOCASE,
	email_verified_at DATETIME,
	password_hash     TEXT NOT NULL,
	two_factor_secret TEXT,
	recovery_codes    TEXT,
	two_factor_step   INTEGER NOT NULL DEFAULT 0,
	created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
