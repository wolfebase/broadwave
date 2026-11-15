-- When the server last settled which track carries a channel's main
-- language, and how: "twin" (its sound matched the other standard's main)
-- or "hand" (set through the API).
ALTER TABLE channels ADD COLUMN sound_checked_at TEXT NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN sound_checked_by TEXT NOT NULL DEFAULT '';
-- The track a recording plays as its main sound: the one the channel played
-- while it recorded. 0 plays the file's first sound.
ALTER TABLE recordings ADD COLUMN sound_pid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN sound_language TEXT NOT NULL DEFAULT '';
