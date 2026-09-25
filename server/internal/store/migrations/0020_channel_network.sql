-- Network the guide decided (ABC, CBS, FOX, NBC). Empty until a guide names it.
-- A blank value is filled from the call-sign table when the channel is listed.

ALTER TABLE channels ADD COLUMN network TEXT NOT NULL DEFAULT '';
