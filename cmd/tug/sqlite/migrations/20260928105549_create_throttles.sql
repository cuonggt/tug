-- The throttles' counts, where every instance of the app counts:
-- throttles_db.go.
CREATE TABLE throttles (
	key_hash BLOB PRIMARY KEY,
	tries    INTEGER NOT NULL,
	ends_at  INTEGER NOT NULL
);
