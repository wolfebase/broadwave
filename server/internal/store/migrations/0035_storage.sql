-- keep: the viewer asked to keep this recording; no keep rule, watched
-- clean-up, or low-space clean-up removes it.
-- watched_at: when it was marked watched (empty otherwise). Rows already
-- marked have no mark time, so their clock starts now. A recording played to
-- its end counts from its last playhead save, as before.
ALTER TABLE recordings ADD COLUMN keep INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recordings ADD COLUMN watched_at TEXT NOT NULL DEFAULT '';
UPDATE recordings SET watched_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE watched = 1;
