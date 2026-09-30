-- A row for each key of a kind that runs one at a time, which a claim
-- of one of its jobs locks, so that two claims of the key take turns:
-- jobs_db.go.
CREATE TABLE job_locks (
	kind      VARCHAR(255) NOT NULL,
	alone_key VARCHAR(255) NOT NULL,
	PRIMARY KEY (kind, alone_key)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
