-- What happened to a user's account, which the app tells them of, the
-- newest first; a read one goes after 90 days: notifications.go.
CREATE TABLE notifications (
	id         INTEGER PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	kind       TEXT NOT NULL,
	data       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	read_at    INTEGER
);
CREATE INDEX notifications_user ON notifications (user_id, id);
