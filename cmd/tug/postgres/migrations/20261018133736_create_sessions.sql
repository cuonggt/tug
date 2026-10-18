-- The sessions themselves, for an app that keeps them on the server
-- rather than in their cookies: sessions_db.go.
CREATE TABLE sessions (
	id_hash    bytea PRIMARY KEY,
	payload    bytea NOT NULL,
	expires_at bigint NOT NULL
);
