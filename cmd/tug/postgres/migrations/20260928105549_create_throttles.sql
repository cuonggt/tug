-- The throttles' counts, where every instance of the app counts:
-- throttles_db.go.
CREATE TABLE throttles (
	key_hash bytea PRIMARY KEY,
	tries    integer NOT NULL,
	ends_at  bigint NOT NULL
);
