-- Each user's passkeys: passkeys.go.
CREATE TABLE passkeys (
	id            BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	user_id       BIGINT NOT NULL,
	credential_id VARBINARY(1023) NOT NULL,
	public_key    BLOB NOT NULL,
	sign_count    BIGINT NOT NULL DEFAULT 0,
	transports    VARCHAR(255) NOT NULL DEFAULT '',
	backed_up     BOOLEAN NOT NULL DEFAULT FALSE,
	name          TEXT NOT NULL,
	created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	last_used_at  DATETIME(6),
	UNIQUE KEY passkeys_credential (credential_id),
	KEY passkeys_user (user_id),
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
