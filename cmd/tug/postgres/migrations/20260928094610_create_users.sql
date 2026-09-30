-- An email is unique whatever its case, Ann@example.com being
-- ann@example.com, by the index on lower(email), which the queries
-- compare with. passkey_handle is the random handle a user's passkeys
-- name them by, made with their first: passkeys.go. photo_key is where
-- their photo is kept on the app's disk: photos.go.
CREATE TABLE users (
	id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name              text NOT NULL,
	email             text NOT NULL,
	email_verified_at timestamptz,
	password_hash     text NOT NULL,
	two_factor_secret text,
	recovery_codes    text,
	two_factor_step   bigint NOT NULL DEFAULT 0,
	passkey_handle    bytea UNIQUE,
	photo_key         text,
	created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email ON users (lower(email));
