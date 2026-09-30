-- passkey_handle is the random handle a user's passkeys name them by,
-- made with their first: passkeys.go. photo_key is where their photo is
-- kept on the app's disk: photos.go.
CREATE TABLE users (
	id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	name              TEXT NOT NULL,
	email             VARCHAR(254) COLLATE utf8mb4_0900_as_ci NOT NULL,
	email_verified_at DATETIME(6),
	password_hash     TEXT NOT NULL,
	two_factor_secret TEXT,
	recovery_codes    TEXT,
	two_factor_step   BIGINT NOT NULL DEFAULT 0,
	passkey_handle    VARBINARY(64),
	photo_key         TEXT,
	created_at        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	UNIQUE KEY users_email (email),
	UNIQUE KEY users_passkey_handle (passkey_handle)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
