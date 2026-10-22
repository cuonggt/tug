-- Who's on the hub's channels, a row for each page's connection to one,
-- which goes as the page does, or once its lease has ended, which
-- prune-presence deletes it after: presences_db.go. data is what the
-- pages should know of the member, as JSON. Times are Unix milliseconds.
CREATE TABLE presences (
	channel  TEXT NOT NULL,
	conn_key TEXT NOT NULL,
	member   TEXT NOT NULL,
	data     BLOB NOT NULL,
	until    INTEGER NOT NULL,
	PRIMARY KEY (channel, conn_key)
);
CREATE INDEX presences_here ON presences (channel, member, until);
CREATE INDEX presences_until ON presences (until);
