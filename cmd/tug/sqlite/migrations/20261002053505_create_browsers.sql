-- The browsers each user has logged in from, by the hash of the ID a
-- cookie of the browser's own keeps, a year: a login from one that isn't
-- here is a new browser's, which the user is told of: logins.go. Times are
-- Unix milliseconds.
CREATE TABLE browsers (
	user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	browser_hash BLOB NOT NULL,
	last_seen_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, browser_hash)
);
CREATE INDEX browsers_seen ON browsers (last_seen_at);
