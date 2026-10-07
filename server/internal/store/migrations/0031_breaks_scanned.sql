-- Set once the server scanned a recording for breaks. A recording with
-- markers was scanned (or marked by hand) before the column existed.
ALTER TABLE recordings ADD COLUMN breaks_scanned INTEGER NOT NULL DEFAULT 0;
UPDATE recordings SET breaks_scanned = 1 WHERE id IN (SELECT recording_id FROM markers);
