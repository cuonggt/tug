-- The cache's values, and its locks, where every instance of the app
-- finds them: cache_db.go.
CREATE TABLE cache (
	key_hash   BLOB PRIMARY KEY,
	value      BLOB NOT NULL,
	expires_at INTEGER NOT NULL
);
