-- A unique kind's job keeps its key while it waits, and of a kind and a
-- key, one job can: jobs.go.
ALTER TABLE jobs ADD COLUMN unique_key TEXT;
CREATE UNIQUE INDEX jobs_unique ON jobs (kind, unique_key) WHERE unique_key IS NOT NULL;
