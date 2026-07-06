-- Large pots need soaks of up to 30 minutes (Austin, 2026-07-06). Raise the
-- per-zone caps that still carry the original 300 s default; values the
-- operator changed away from the default are left alone.
UPDATE irrigation_zones SET max_flood_seconds = 1800 WHERE max_flood_seconds = 300;
UPDATE irrigation_zones SET max_pump_seconds  = 1800 WHERE max_pump_seconds  = 300;
