-- What happened to a user's account, which the app tells them of, the
-- newest first; a read one goes after 90 days: notifications.go.
CREATE TABLE notifications (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	kind       text NOT NULL,
	data       text NOT NULL,
	created_at bigint NOT NULL,
	read_at    bigint
);
CREATE INDEX notifications_user ON notifications (user_id, id);
