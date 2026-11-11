# Moisture R&D protocol — Propagation Unit 01

Written 2026-11-11 after Austin asked what the shelf moisture bands were based on
(answer: species plates + a guessed container capacity, no literature). This is
the replacement: a calibration you do once per mix, set-points in true
volumetric water content (VWC) taken from the literature, and a logging plan.

**Experiment epoch: 2026-11-10 ~20:00 CST.** Everything before that is
bring-up noise (plants moved shelves, probes swapped, bus outages). Raw
readings are retained 180 days from this build on (`retention.raw_days`);
hourly aggregates are kept forever.

## 1. Why the current numbers are provisional

The ZTS-3000 is a dielectric (FDR/capacitance-class) probe shipped with a
mineral-soil calibration. Capacitance sensors need a substrate-specific
calibration; the manufacturer's general potting-soil curve was off by up to
0.18 m³/m³ against real coir/perlite mixes, while individual mixes from pure
coir to 2:8 coir:perlite calibrated linearly and within ~0.06 of each other
(Rhie et al. 2017, HortScience). Our shelves are coir/perlite/native-soil or
potting-mix blends, so each shelf's probe reading is an uncalibrated number
until §2 is done. The 16/24, 22/26, 10/18 triggers loaded on 2026-11-10 are
ratios of a guessed container capacity (≈33 on the probe) and nothing more.

## 2. Two-point calibration, once per mix (≈ 30 min hands-on, overnight dry)

Do this for each shelf's mix (three mixes if they differ).

1. Fill a container of known volume V (a 1 L yogurt tub works; measure it
   with water first) with the shelf's mix, packed as the pots are.
2. Weigh it dry-ish as it comes from the bag → W0.
3. Flood it the way the tray floods (stand it in water to the rim for 15 min),
   let it drain 10 min → weigh W_cc. Push the probe in to full prong depth,
   wait 1 min, read moisture → **P_cc**.
4. True container capacity VWC_cc = (W_cc − W_dry) / V, where W_dry is the
   weight after oven-drying a sample at 105 °C overnight (or 3 days on the
   dash of a parked truck in the sun; note which).
5. Let the tub dry with the probe in it on the rack, reading the probe at
   roughly the same time each day and weighing it, until the plant-free mix
   reads nearly flat. Each (weight, reading) pair is a calibration point:
   VWC = (W − W_dry) / V.
6. Fit a straight line reading → VWC (two points are enough; the dry-down
   gives more). Record slope/intercept per shelf in §5.

Capacitance sensors in loamy/coir media calibrate well with a linear fit
(R² > 0.93 across coir/perlite ratios, Rhie 2017; low-cost capacitive
probes R² ≈ 0.85–0.87 in loamy silt, Abdelmoneim 2025).

**Shortcut if you only do one thing:** step 3 alone (P_cc) plus the
assumption VWC_cc ≈ 0.50 for coir-based mixes gets the ratio method within
striking distance. Do the full thing for the R&D record.

## 3. Set-points in true VWC (literature)

| Evidence | Finding |
| --- | --- |
| Ebb-and-flow, citrus liners in 56 cm³ cones, peat:perlite | Growth rose with trigger VWC 0.12 → 0.48; 0.48 recommended for small cones (Ferrarezi et al. 2017, Agric. Water Mgmt). |
| Ebb-and-flow, citrus rootstocks in treepots | Triggers 0.24 / 0.36 / 0.48 tested; **0.24** best for growth + water in the larger pots (Zambon et al. 2022, Agric. Water Mgmt). |
| Ebb-and-flow, pre-sprouted plantlets in 130 cm³ cones | 0.25 / 0.35 / 0.45 tested; **0.35** recommended, 0.45 raised substrate EC (Silva et al. 2021, Sugar Tech). |
| Perlite:coco 1:1, basil | No stress above ≈0.20; drought symptoms at ≈0.17 (−300 hPa); set-points 0.20–0.40 grew the same (Montesano et al. 2018, Agric. Water Mgmt). |
| Woody ornamentals (Hibiscus, Gardenia), UGA | Salable size above a **0.35** threshold; gardenia bloom best at 0.40 (Bayer 2014 dissertation). |
| Commercial woody nursery trial | Set points fixed **after a 7-day monitoring period**; averaging set-points across species caused mortality (ISHS 1150_70). |
| Boxwood / Deutzia | "On-demand" set-point = VWC supporting 90 % of max photosynthesis (JEH 38(1)). |

Read-across for our three regimes, in **true VWC**:

| Shelf | Plants | Trigger (water below) | Re-arm (must read above after a soak) | Basis |
| --- | --- | --- | --- | --- |
| 1 | Bald cypress, persimmon seed | **0.30** | 0.42 | moist-drained woody liner band (0.24–0.36) |
| 2 | Buttonbush, sea oats | **0.40** | 0.48 | wet-loving; small-container optimum 0.45–0.48 |
| 3 | Mountain laurel, chickasaw plum (+ forbs) | **0.20** | 0.32 | upland dry-down; stress onset ≈0.17 in coir/perlite |

Convert each to a probe reading with the shelf's line from §2:
reading = (VWC − intercept) / slope. Then load them (Controls tab or
`PUT /irrigation/zones/{z}/moisture-control`). Re-arm must sit **below** the
probe's post-soak reading or the zone never re-arms — check after the first
soak.

## 4. Logging plan

- Epoch 2026-11-10 20:00 CST. Automation OFF until Thu 2026-11-12 morning
  (leak repair); days 1–2 are pure dry-down observation.
- Day 7 (≈ 2026-11-17): first look at each shelf's dry-down curve
  (`tools/export_readings.py --days 7`); confirm the re-arm readings are
  reached, adjust triggers if a shelf never triggers or never re-arms.
- Day 14 (≈ 2026-11-24/26): review as planned.
- Keep every change to a trigger in the as-built log with the date, so the
  series can be read later.
- Raw retention 180 days; hourly aggregates forever. Export to CSV at
  each review and file under the R&D folder.

## 5. Calibration record (fill in)

| Shelf | Mix | V (mL) | W_dry (g) | W_cc (g) | VWC_cc | P_cc (probe) | slope | intercept | date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | | | | | | | | | |
| 2 | | | | | | | | | |
| 3 | | | | | | | | | |

## References

- Rhie Y. et al. 2017. Changes in physical properties of various coir dust and perlite mixes and their capacitance sensor volumetric water content calibrations. HortScience 52(1).
- Abdelmoneim A. et al. 2025. Calibration of low-cost capacitive soil moisture sensors for irrigation management applications. Sensors.
- Ferrarezi R. et al. 2017. Automated ebb-and-flow subirrigation for citrus liners production. I. Plant growth. Agric. Water Mgmt.
- Zambon F. et al. 2022. Automated ebb-and-flow subirrigation accelerates citrus liner production in treepots. Agric. Water Mgmt.
- Silva T. et al. 2021. Pre-sprouted sugarcane plantlets produced in ebb-and-flow subirrigation automated by soil moisture sensors. Sugar Tech.
- Montesano F. et al. 2018. Sensor-based irrigation management of soilless basil… effects of set-point. Agric. Water Mgmt.
- Bayer A. 2014. PhD dissertation, University of Georgia (sensor-based irrigation of woody ornamentals).
- ISHS Acta Hortic. 1150_70 (commercial nursery sensor-based irrigation of woody species); J. Environ. Hort. 38(1) (on-demand set-point, boxwood/deutzia).
