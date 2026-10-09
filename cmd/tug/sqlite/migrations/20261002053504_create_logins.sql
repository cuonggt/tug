-- The browsers each user is logged in from, a row for each login, by the
-- hash of the ID its session keeps, which goes as the login ends: logged
-- out, ended from the security page, or past its session's lifetime, which
-- prune-logins deletes it after: logins.go. Times are Unix milliseconds.
CREATE TABLE logins (
	id           INTEGER PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	login_hash   BLOB NOT NULL UNIQUE,
	user_agent   TEXT NOT NULL,
	ip           TEXT NOT NULL,
	lifetime     INTEGER NOT NULL,
	created_at   INTEGER NOT NULL,
	last_seen_at INTEGER NOT NULL,
	ends_at      INTEGER NOT NULL
);
CREATE INDEX logins_user ON logins (user_id);
CREATE INDEX logins_ends ON logins (ends_at);
