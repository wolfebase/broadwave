-- A guide window with no channel list cannot use airings_channel_start.
-- starts_at leads so the window's upper bound is a range and the rows come
-- back in start order. ends_at is the second key so a listing that has
-- already ended is skipped without reading the row.
CREATE INDEX airings_starts ON airings(starts_at, ends_at);
