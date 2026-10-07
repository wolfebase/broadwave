-- A catalog from before the baseline may lack the table.
CREATE TABLE IF NOT EXISTS markers (
	id INTEGER PRIMARY KEY,
	recording_id INTEGER NOT NULL,
	start_sec REAL NOT NULL,
	end_sec REAL NOT NULL
);
ALTER TABLE markers ADD COLUMN confidence REAL NOT NULL DEFAULT 1;
