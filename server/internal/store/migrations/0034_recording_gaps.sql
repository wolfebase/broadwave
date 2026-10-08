-- Times a recording's stream stopped (the signal dropped) and the seconds lost
-- in all. NULL until the finished file was read, like the other health counts.
ALTER TABLE recordings ADD COLUMN gaps INTEGER;
ALTER TABLE recordings ADD COLUMN lost_seconds REAL;
