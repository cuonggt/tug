-- The jobs waiting to run, and the ones that failed for good: jobs.go.
-- A unique kind's job keeps its key in unique_key while it waits, and
-- of a kind and a key, one job can, as a unique index lets NULLs
-- repeat; a job of a kind that runs one at a time keeps its key in
-- alone_key all its life. A kind whose jobs run so many at once at most
-- keeps its limit with each of them in at_once, and a claim counts the
-- jobs held, by jobs_held. carried is what a job carried from the
-- context it was pushed from, as the ID of the request that pushed it,
-- as JSON, which a claim and the failed jobs' list give back.
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
	at_once    INT,
	carried    TEXT,
	created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	KEY jobs_due (failed_at, run_at),
	UNIQUE KEY jobs_unique (kind, unique_key),
	KEY jobs_alone (kind, alone_key),
	KEY jobs_held (held_until)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
