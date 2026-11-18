-- Propagation batches and their dated log (2026-11-17). The rack knew shelves
-- and settings but not plants. These two tables are the two sheets of the
-- shared batch format (docs/BATCH-FORMAT.md): one row per batch, one row per
-- dated event. Column names match the CSV headers so export is a straight read.
--
-- zone_key is the shelf the batch sits on NOW ('' = not on the rack). Where it
-- sat before is in the log: every move writes a 'moved' row with the shelf.
-- source is a name or a section, never coordinates.
CREATE TABLE IF NOT EXISTS batches (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_code    TEXT NOT NULL UNIQUE,
  species       TEXT NOT NULL,
  species_code  TEXT NOT NULL DEFAULT '',
  source        TEXT NOT NULL DEFAULT '',
  started_on    TEXT NOT NULL,
  method        TEXT NOT NULL DEFAULT '',
  container     TEXT NOT NULL DEFAULT '',
  medium        TEXT NOT NULL DEFAULT '',
  count_started INTEGER,
  count_current INTEGER,
  status        TEXT NOT NULL DEFAULT 'active',
  zone_key      TEXT NOT NULL DEFAULT '',
  notes         TEXT NOT NULL DEFAULT '',
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS batch_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id    INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
  occurred_on TEXT NOT NULL,
  event       TEXT NOT NULL,
  count       INTEGER,
  zone_key    TEXT NOT NULL DEFAULT '',
  note        TEXT NOT NULL DEFAULT '',
  actor       TEXT NOT NULL DEFAULT '',
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_batch_log_batch ON batch_log(batch_id, occurred_on, id);
