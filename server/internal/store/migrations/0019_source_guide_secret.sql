-- A playlist and its guide can each carry a login (Xtream sends the password
-- to both), so the guide keeps its own secret.

ALTER TABLE source_secrets ADD COLUMN guide_secret TEXT NOT NULL DEFAULT '';
