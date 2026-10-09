-- The browsers each user has logged in from, by the hash of the ID a
-- cookie of the browser's own keeps, a year: a login from one that isn't
-- here is a new browser's, which the user is told of: logins.go. Times are
-- Unix milliseconds.
CREATE TABLE browsers (
	user_id      BIGINT NOT NULL,
	browser_hash BINARY(32) NOT NULL,
	last_seen_at BIGINT NOT NULL,
	PRIMARY KEY (user_id, browser_hash),
	KEY browsers_seen (last_seen_at),
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
