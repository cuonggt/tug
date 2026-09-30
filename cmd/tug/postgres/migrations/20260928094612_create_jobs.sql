-- The jobs waiting to run, and the ones that failed for good: jobs.go.
-- A unique kind's job keeps its key in unique_key while it waits, and
-- of a kind and a key, one job can; a job of a kind that runs one at a
-- time keeps its key in alone_key all its life.
CREATE TABLE jobs (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	kind       text NOT NULL,
	payload    text NOT NULL,
	run_at     bigint NOT NULL,
	held_until bigint NOT NULL DEFAULT 0,
	attempts   integer NOT NULL DEFAULT 0,
	error      text,
	failed_at  timestamptz,
	unique_key text,
	alone_key  text,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_due ON jobs (run_at) WHERE failed_at IS NULL;
CREATE UNIQUE INDEX jobs_unique ON jobs (kind, unique_key) WHERE unique_key IS NOT NULL;
CREATE INDEX jobs_alone ON jobs (kind, alone_key) WHERE alone_key IS NOT NULL;
