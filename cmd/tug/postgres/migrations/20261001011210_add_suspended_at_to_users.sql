-- A suspended account can't log in, and its API tokens are turned away,
-- until an admin restores it: admin.go.
ALTER TABLE users ADD COLUMN suspended_at timestamptz;
