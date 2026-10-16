CREATE TABLE IF NOT EXISTS servers (
  id TEXT PRIMARY KEY,
  public_key TEXT NOT NULL UNIQUE,
  server_id TEXT NOT NULL,
  created INTEGER NOT NULL,
  ip TEXT NOT NULL,
  first_acme INTEGER NOT NULL DEFAULT 0,
  acme_day TEXT NOT NULL DEFAULT '',
  acme_count INTEGER NOT NULL DEFAULT 0,
  addr_hour INTEGER NOT NULL DEFAULT 0,
  addr_count INTEGER NOT NULL DEFAULT 0,
  seen INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS servers_ip_created ON servers (ip, created);
CREATE INDEX IF NOT EXISTS servers_first_acme ON servers (first_acme);
-- Added after the first deploy; on an existing database run:
--   ALTER TABLE servers ADD COLUMN addr_hour INTEGER NOT NULL DEFAULT 0;
--   ALTER TABLE servers ADD COLUMN addr_count INTEGER NOT NULL DEFAULT 0;
