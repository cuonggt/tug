-- Each user's passkeys: passkeys.go.
CREATE TABLE passkeys (
	id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id       bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	credential_id bytea NOT NULL UNIQUE,
	public_key    bytea NOT NULL,
	sign_count    bigint NOT NULL DEFAULT 0,
	transports    text NOT NULL DEFAULT '',
	backed_up     boolean NOT NULL DEFAULT false,
	name          text NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now(),
	last_used_at  timestamptz
);
CREATE INDEX passkeys_user ON passkeys (user_id);
