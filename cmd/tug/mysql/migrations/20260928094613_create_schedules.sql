-- The run of each schedule pushed last, so that one instance pushes
-- it: jobs.go.
CREATE TABLE schedules (
	name   VARCHAR(255) NOT NULL PRIMARY KEY,
	run_at BIGINT NOT NULL
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
