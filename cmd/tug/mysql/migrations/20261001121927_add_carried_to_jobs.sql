-- What a job carried from the context it was pushed from, as the ID of
-- the request that pushed it, as JSON, which a claim and the failed jobs'
-- list give back: jobs.go.
ALTER TABLE jobs ADD COLUMN carried TEXT;
