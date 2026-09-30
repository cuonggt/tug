-- The cache's values, and its locks, where every instance of the app
-- finds them: cache_db.go.
CREATE TABLE cache (
	key_hash   bytea PRIMARY KEY,
	value      bytea NOT NULL,
	expires_at bigint NOT NULL
);
