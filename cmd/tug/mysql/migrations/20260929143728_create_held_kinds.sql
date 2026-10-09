-- The kinds a rate holds back, each until its time: jobs.go.
CREATE TABLE held_kinds (
	kind       VARCHAR(255) NOT NULL PRIMARY KEY,
	held_until BIGINT NOT NULL
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
