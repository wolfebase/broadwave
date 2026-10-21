-- A manual pass records a channel for a set time: duration is its length in minutes.
ALTER TABLE passes ADD COLUMN duration INTEGER NOT NULL DEFAULT 0;
