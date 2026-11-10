-- The rack has three shelves (top=1, middle=2, bottom=3). The speculative
-- "Shelf 4 (future)" zone made both UIs draw a fourth, permanently vacant bay
-- (Austin, 2026-11-09). Remove it unless it has ever watered. A fourth shelf
-- can be added back deliberately on relay 5 (silkscreen 4) if it's ever built.
DELETE FROM irrigation_zones
 WHERE zone_key = 'zone_4'
   AND id NOT IN (SELECT zone_id FROM irrigation_cycles);
