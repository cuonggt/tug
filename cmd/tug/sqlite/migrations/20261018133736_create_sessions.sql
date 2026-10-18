-- The sessions themselves, for an app that keeps them on the server
-- rather than in their cookies: sessions_db.go.
CREATE TABLE sessions (
	id_hash    BLOB PRIMARY KEY,
	payload    BLOB NOT NULL,
	expires_at INTEGER NOT NULL
);
