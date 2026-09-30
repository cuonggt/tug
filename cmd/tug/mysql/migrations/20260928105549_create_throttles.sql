-- The throttles' counts, where every instance of the app counts:
-- throttles_db.go.
CREATE TABLE throttles (
	key_hash BINARY(32) NOT NULL PRIMARY KEY,
	tries    INT NOT NULL,
	ends_at  BIGINT NOT NULL
);
