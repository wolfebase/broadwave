-- What a listing says of its picture, sound, and captions; empty or 0 when it says nothing.
ALTER TABLE airings ADD COLUMN cc INTEGER NOT NULL DEFAULT 0;
ALTER TABLE airings ADD COLUMN video TEXT NOT NULL DEFAULT '';
ALTER TABLE airings ADD COLUMN audio TEXT NOT NULL DEFAULT '';

-- What the last tune read in a channel's stream: caption service descriptors,
-- and the main sound's layout (mono, stereo, surround).
ALTER TABLE channels ADD COLUMN captions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE channels ADD COLUMN audio_layout TEXT NOT NULL DEFAULT '';
