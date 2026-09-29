ALTER TABLE recordings ADD COLUMN continuity_errors INTEGER;
ALTER TABLE recordings ADD COLUMN transport_errors INTEGER;
ALTER TABLE recordings ADD COLUMN sync_losses INTEGER;
ALTER TABLE recordings ADD COLUMN packets INTEGER;
