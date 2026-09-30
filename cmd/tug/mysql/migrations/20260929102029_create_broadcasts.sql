-- The events the hub carries between the app's instances, each kept a
-- minute: broadcasts_db.go.
CREATE TABLE broadcasts (
	id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
	event      BLOB NOT NULL,
	created_at BIGINT NOT NULL
);
