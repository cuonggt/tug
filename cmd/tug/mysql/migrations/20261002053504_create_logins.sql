-- The browsers each user is logged in from, a row for each login, by the
-- hash of the ID its session keeps, which goes as the login ends: logged
-- out, ended from the security page, or past its session's lifetime, which
-- prune-logins deletes it after: logins.go. Times are Unix milliseconds.
CREATE TABLE logins (
	id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	user_id      BIGINT NOT NULL,
	login_hash   BINARY(32) NOT NULL,
	user_agent   TEXT NOT NULL,
	ip           VARCHAR(45) NOT NULL,
	lifetime     BIGINT NOT NULL,
	created_at   BIGINT NOT NULL,
	last_seen_at BIGINT NOT NULL,
	ends_at      BIGINT NOT NULL,
	UNIQUE KEY logins_hash (login_hash),
	KEY logins_user (user_id),
	KEY logins_ends (ends_at),
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
