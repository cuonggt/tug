-- A job of a kind that runs one at a time keeps its key all its life,
-- where a claim lets unique_key go, for a claim to see whether another
-- of the key runs: jobs.go.
ALTER TABLE jobs ADD COLUMN alone_key TEXT;
CREATE INDEX jobs_alone ON jobs (kind, alone_key) WHERE alone_key IS NOT NULL;
