-- Where a recording's intro and end titles are, found by comparing its sound
-- with other episodes of its show. 0 when not found.
ALTER TABLE recordings ADD COLUMN intro_start REAL NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN intro_end REAL NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN credits_start REAL NOT NULL DEFAULT 0;
-- Sound prints of the first and the last minutes of a recording, one 32-bit
-- print per 64 ms, little-endian. A row means the recording was listened to,
-- even when it had no sound. show_start and show_end are where its listing
-- starts and ends in the file; show_end is 0 when that is not known.
CREATE TABLE episode_prints (
	recording_id INTEGER PRIMARY KEY,
	head BLOB NOT NULL,
	tail BLOB NOT NULL,
	tail_from REAL NOT NULL DEFAULT 0,
	show_start REAL NOT NULL DEFAULT 0,
	show_end REAL NOT NULL DEFAULT 0
);
