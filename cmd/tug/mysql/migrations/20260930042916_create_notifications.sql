-- What happened to a user's account, which the app tells them of, the
-- newest first; a read one goes after 90 days: notifications.go.
CREATE TABLE notifications (
	id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	user_id    BIGINT NOT NULL,
	kind       VARCHAR(255) NOT NULL,
	data       TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	read_at    BIGINT,
	KEY notifications_user (user_id, id),
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
