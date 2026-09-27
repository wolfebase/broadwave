-- A pass of kind 'once' records the one airing that starts at this time on its channel.
-- Unix seconds; 0 on every other pass.

ALTER TABLE passes ADD COLUMN airing_start INTEGER NOT NULL DEFAULT 0;
