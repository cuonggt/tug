-- The browsers each user has logged in from, by the hash of the ID a
-- cookie of the browser's own keeps, a year: a login from one that isn't
-- here is a new browser's, which the user is told of: logins.go. Times are
-- Unix milliseconds.
CREATE TABLE browsers (
	user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	browser_hash bytea NOT NULL,
	last_seen_at bigint NOT NULL,
	PRIMARY KEY (user_id, browser_hash)
);
CREATE INDEX browsers_seen ON browsers (last_seen_at);
