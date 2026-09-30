-- The cache's values, and its locks, where every instance of the app
-- finds them: cache_db.go. A value is up to 16 MB, a MEDIUMBLOB's.
CREATE TABLE cache (
	key_hash   BINARY(32) NOT NULL PRIMARY KEY,
	value      MEDIUMBLOB NOT NULL,
	expires_at BIGINT NOT NULL
);
