-- Ebb-and-flow conversion (Austin, 2026-07-09). The old plumbing closed the
-- valve with the pump (drain_seconds = 0). The new drain-back pump requires
-- the valve to hold open after the pump stops so the tray drains back through
-- the line. Set the ebb-and-flow default (5 min) on any zone still carrying
-- the old 0; zones an operator changed away from 0 are left alone.
UPDATE irrigation_zones
   SET config_json = json_set(config_json, '$.drain_seconds', 300)
 WHERE json_extract(config_json, '$.drain_seconds') = 0
    OR json_extract(config_json, '$.drain_seconds') IS NULL;
