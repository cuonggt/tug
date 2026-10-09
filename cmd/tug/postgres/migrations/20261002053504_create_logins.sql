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
