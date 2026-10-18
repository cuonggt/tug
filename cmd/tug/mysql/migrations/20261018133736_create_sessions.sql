-- The sessions themselves, for an app that keeps them on the server
-- rather than in their cookies: sessions_db.go. A session is up to 16 MB,
-- a MEDIUMBLOB's.
CREATE TABLE sessions (
	id_hash    BINARY(32) NOT NULL PRIMARY KEY,
	payload    MEDIUMBLOB NOT NULL,
	expires_at BIGINT NOT NULL
);
