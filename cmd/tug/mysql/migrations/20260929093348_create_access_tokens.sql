-- The users' API tokens, their hashes, which a request to /api sends
-- one of in place of a login: tokens.go.
CREATE TABLE access_tokens (
	id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	user_id      BIGINT NOT NULL,
	name         TEXT NOT NULL,
	token_hash   BINARY(32) NOT NULL,
	abilities    TEXT NOT NULL,
	last_used_at BIGINT,
	expires_at   BIGINT,
	created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	UNIQUE KEY access_tokens_hash (token_hash),
	KEY access_tokens_user (user_id),
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
