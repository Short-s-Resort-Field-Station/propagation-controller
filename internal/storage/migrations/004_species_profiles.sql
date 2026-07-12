-- Species profiles: per-species, per-stage saved settings a grower can dial in
-- once and re-apply. settings_json holds the moisture-control settings (active
-- control) plus pH and temperature target RANGES (monitoring-only — the unit
-- senses but does not control pH/temp; the ranges drive the gauge "happy band"
-- and the time-in-range tracking). species_id matches the code species catalog
-- botanical ID (e.g. TADI); stage is free text (Seeds/Trimmings/Suckers/…).
CREATE TABLE IF NOT EXISTS species_profiles (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  species_id    TEXT NOT NULL,
  stage         TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL,
  settings_json TEXT NOT NULL,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_species_profiles_species ON species_profiles(species_id);
