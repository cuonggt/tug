-- The jobs waiting to run, and the ones that failed for good: jobs.go.
-- A unique kind's job keeps its key in unique_key while it waits, and
-- of a kind and a key, one job can, as a unique index lets NULLs
-- repeat; a job of a kind that runs one at a time keeps its key in
-- alone_key all its life.
CREATE TABLE jobs (
	id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	kind       VARCHAR(255) NOT NULL,
	payload    MEDIUMTEXT NOT NULL,
	run_at     BIGINT NOT NULL,
	held_until BIGINT NOT NULL DEFAULT 0,
	attempts   INT NOT NULL DEFAULT 0,
	error      TEXT,
	failed_at  DATETIME(6),
	unique_key VARCHAR(255),
	alone_key  VARCHAR(255),
	created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	KEY jobs_due (failed_at, run_at),
	UNIQUE KEY jobs_unique (kind, unique_key),
	KEY jobs_alone (kind, alone_key)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
