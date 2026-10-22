-- Who's on the hub's channels, a row for each page's connection to one,
-- which goes as the page does, or once its lease has ended, which
-- prune-presence deletes it after: presences_db.go. data is what the
-- pages should know of the member, as JSON. Times are Unix milliseconds.
CREATE TABLE presences (
	channel  VARCHAR(255) NOT NULL,
	conn_key VARCHAR(64) NOT NULL,
	member   VARCHAR(255) NOT NULL,
	data     BLOB NOT NULL,
	until    BIGINT NOT NULL,
	PRIMARY KEY (channel, conn_key),
	KEY presences_here (channel, member, until),
	KEY presences_until (until)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
