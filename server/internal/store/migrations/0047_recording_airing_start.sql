-- The airing a scheduled recording was made for, so a late retry of one
-- airing is never taken for the next one's.
ALTER TABLE recordings ADD COLUMN airing_start TEXT NOT NULL DEFAULT '';
