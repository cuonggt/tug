-- The events the hub carries between the app's instances, each kept a
-- minute: broadcasts_db.go. AUTOINCREMENT keeps an ID from being used
-- again once its row is pruned, as an instance reads what's after the
-- last it read.
CREATE TABLE broadcasts (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	event      BLOB NOT NULL,
	created_at INTEGER NOT NULL
);
