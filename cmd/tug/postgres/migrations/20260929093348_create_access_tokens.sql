-- The users' API tokens, their hashes, which a request to /api sends
-- one of in place of a login: tokens.go.
CREATE TABLE access_tokens (
	id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	name         text NOT NULL,
	token_hash   bytea NOT NULL UNIQUE,
	abilities    text NOT NULL,
	last_used_at bigint,
	expires_at   bigint,
	created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX access_tokens_user ON access_tokens (user_id);
