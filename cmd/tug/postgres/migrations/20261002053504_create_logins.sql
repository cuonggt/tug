-- The browsers each user is logged in from, a row for each login, by the
-- hash of the ID its session keeps, which goes as the login ends: logged
-- out, ended from the security page, or past its session's lifetime, which
-- prune-logins deletes it after: logins.go. Times are Unix milliseconds.
CREATE TABLE logins (
	id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	login_hash   bytea NOT NULL UNIQUE,
	user_agent   text NOT NULL,
	ip           text NOT NULL,
	lifetime     bigint NOT NULL,
	created_at   bigint NOT NULL,
	last_seen_at bigint NOT NULL,
	ends_at      bigint NOT NULL
);
CREATE INDEX logins_user ON logins (user_id);
CREATE INDEX logins_ends ON logins (ends_at);

-- The browsers each user has logged in from, by the hash of the ID a
-- cookie of the browser's own keeps, a year: a login from one that isn't
-- here is a new browser's, which the user is told of.
CREATE TABLE browsers (
	user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	browser_hash bytea NOT NULL,
	last_seen_at bigint NOT NULL,
	PRIMARY KEY (user_id, browser_hash)
);
CREATE INDEX browsers_seen ON browsers (last_seen_at);
