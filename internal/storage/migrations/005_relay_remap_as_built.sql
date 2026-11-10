-- Relay channels remapped to the AS-BUILT wiring (propagation_wiring.html
-- Rev A, bench-verified 2026-11-09). Software channels are 1-indexed; the
-- Numato wire index is N-1. Old seed: valves on 3-6, pump 2, lights 1, fans 7.
-- New: valves 1-3, fans 4, 5 reserved for a 4th valve, pump 7, lights 8.
UPDATE irrigation_zones SET valve_relay_channel = CASE zone_key
  WHEN 'zone_1' THEN 1
  WHEN 'zone_2' THEN 2
  WHEN 'zone_3' THEN 3
  WHEN 'zone_4' THEN 5
  ELSE valve_relay_channel END
WHERE zone_key IN ('zone_1','zone_2','zone_3','zone_4');
