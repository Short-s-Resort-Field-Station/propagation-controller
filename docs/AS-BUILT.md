# AS-BUILT — Propagation Unit 01 commissioning record

Started 2026-11-09 at the rack, Austin at the bench, Claude over SSH.
Source drawing: `propagation_wiring.html` Rev A (two-enclosure build:
Box 1 power/control, Box 2 field junction, one 12-pin umbilical).
This file is the record of what is actually plugged where. Update it the
moment something changes; the software must match this table, not the
other way round.

**Shelf numbering (Austin, 2026-11-09): top to bottom. Shelf 1 is the top shelf, Shelf 3 the bottom.**

## Relay channels — TWO numbering systems, always record both

The Numato board silkscreen and its serial protocol (`relay on N`) are
**0-indexed (0–7)**. The spec, the API, the event log and the UI are
**1-indexed (1–8)**. The driver (`internal/devices/relay/numato_usb.go`)
subtracts one on the wire. Software constants live in
`internal/devices/manager.go` (`DefaultAssignments`, `LightsChannel`,
`PumpChannel`, `FansChannel`) and the zone seeds in
`internal/irrigation/engine.go`; migration 005 moved existing DB rows.

| Silkscreen / `relay on N` | Software / API channel | Load | Voltage | Verified |
| --- | --- | --- | --- | --- |
| 0 | 1 | Solenoid valve 1 → Shelf 1 (top) | 12 VDC via F1 | **2026-11-09** 2 s pulse, clicked at top shelf |
| 1 | 2 | Solenoid valve 2 → Shelf 2 (middle) | 12 VDC via F1 | **2026-11-09** 2 s pulse, clicked at middle shelf |
| 2 | 3 | Solenoid valve 3 → Shelf 3 (bottom) | 12 VDC via F2 | **2026-11-09** 2 s pulse, clicked at bottom shelf |
| 3 | 4 | Circulation fans, all ganged, 12→5 V buck in Box 2 | 12 VDC via F3 | **2026-11-09** relay held: Fan 2 runs; **Fan 1 cable faulty (flagged)**. **FIXED 2026-11-10 13:40**: Fan 2 ran with relay 3 OFF because the blue P4 wire was touching the hot (COM/+12 V) at the Numato relay-3 terminal block — a stray strand/bad landing, not the field box. Re-landed on NO; 10 s pulse through the daemon: fans on, fans off, all relays off. Fan negatives moved from the field-box ground bus to the buck's OUT−. Fan 1 found wired red/black reversed and fried; replace. |
| 4 | 5 | Spare — reserved for a 4th valve (zone_4 seeded here, disabled) | — | — |
| 5 | 6 | Spare | — | — |
| 6 | 7 | Hydrofarm pump, 33 W, via tail H1 → gang box | **120 VAC** | **2026-11-10** Austin found NO and COM swapped at R6, rewired; 3 s pulse through the daemon (safety-authorized): pump ran and stopped with the relay. VERIFIED |
| 7 | 8 | T5 grow lights, 8 × 5 W, via tail H2 → gang box | **120 VAC** | **2026-11-09** 10 s pulse, lit and went out, board read 0x80 |

Lighting is rack-wide by design: one relay, one photoperiod schedule, all
eight fixtures together. There is no per-shelf lighting anywhere in the
software (Austin, 2026-11-09).

## RS-485 / Modbus bus — device addresses (FINAL LAYOUT 2026-11-10)

Bus: USB-RS485 (FTDI, `/dev/serial/by-id/usb-FTDI_USB-RS485-WE_FTBCX8VV-if00-port0`),
**9600 baud 8N1**, A/B ride umbilical P8 (yellow) / P10 (pink) to the rails in
Box 2. 120 Ω terminator at the far end of the bus.

**Why 9600 and why the probes are at 11-13:** the level transmitter does NOT
persist configuration writes across power loss — fc06, fc16 and its mirror
block were all tried (2026-11-10); every power cycle put it back at addr 1 @
9600. The ZTS-3000 probes DO persist (proven across the 09:42 reboot). So the
transmitter stays at factory settings and the rest of the bus was moved to
match it. Nothing on this bus needs re-commissioning after an outage.
Daemon: `PROPAGATION_MODBUS_BAUD=9600` (drop-in `20-modbus-baud.conf`; also
the code default now).

| Modbus addr | Device | Location | Commissioned |
| --- | --- | --- | --- |
| **1** | Hydrostatic reservoir level sensor, **factory settings, never written** (24 V via boost; **A/B swapped vs soil probes**: Blu=A, Yel=B) | Reservoir | **2026-11-10** register map below |
| **11** | ZTS-3000 soil probe (cable label "1") | **Shelf 2 (middle)** — swapped with "2" for cord length, 2026-11-10 PM; daemon key `soil_zone_2` | **2026-11-10** `ztsmove.py 1 4800 11 2` (reg 0x07D0=11, 0x07D1=2 ⇒ 9600) |
| **12** | ZTS-3000 soil probe (cable label "2") | **Shelf 1 (top)** — swapped with "1", 2026-11-10 PM; daemon key `soil_zone_1` | **2026-11-10** `ztsmove.py 2 4800 12 2` |
| **13** | ZTS-3000 soil probe (cable label "3") | Shelf 3 (bottom) | **2026-11-10** `ztsmove.py 3 4800 13 2` |
| 14, 15 | spare ZTS-3000 probes (if installed) | | pending — a factory-fresh probe answers at 1 @ 4800: `ztsmove.py 1 4800 14 2` with it alone on the bus |

ZTS-3000 config registers: `0x07D0` slave address, `0x07D1` baud code (**1 = 4800, 2 = 9600**). Both persist.

### Reservoir level sensor — register map (bench-verified 2026-11-09)

Holding (fc 0x03) and input (fc 0x04) return the same block; valid 0x0000–0x0027, exception 1 above. Writes via fc 0x06.

| Reg | Meaning | Value as commissioned |
| --- | --- | --- |
| 0x0000 | Modbus address | 1 (factory; writes don't persist) |
| 0x0001 | Baud code (2 = 4800, 3 = 9600) | 3 (factory) |
| 0x0002 | Unit code | 17 (cm) |
| 0x0003 | Decimal places | 1 |
| 0x0004 | **Measured level ×10** | 61 → 6.1 cm with ~3 in of water; 164 → 16.4 cm with 6 in (Austin's tape) — ±1 cm, uncalibrated |
| 0x0005 | Zero offset | 0 |
| 0x0006 | Full-scale range ×10 | 1000 → 100.0 cm |
| 0x0007 | ? | 1 |
| 0x0020–0x0027 | mirror of 0x0000–0x0007 | |

Scan/dump/monitor/write scripts: `mbscan.py`, `mbdump.py`, `mbmon.py`, `mbwrite.py` (pure pyserial, no modbus lib) — copies kept in `tools/modbus/`.

## Solenoid valves — which lead feeds which shelf

| Umbilical pin | Relay (silkscreen / software) | Shelf | Verified |
| --- | --- | --- | --- |
| P1 grey | 0 / 1 | Shelf 1 (top) | **2026-11-09** clicked |
| P2 white | 1 / 2 | Shelf 2 (middle) | **2026-11-09** clicked |
| P3 black | 2 / 3 | Shelf 3 (bottom) | **2026-11-09** clicked |

## OPEN ITEMS (updated 2026-11-10 12:45 — see docs/AUDIT-2026-11-10.md §6 for what was fixed)

1. ~~Fan 2 runs with the fan relay off~~ **FIXED 2026-11-10**: blue P4 was touching +12 V at the Numato relay-3 terminals. Lesson (twice now, pump R6 and fan R3): meter every Numato output terminal to ground with the relay off before loads go on.
2. **Fan 1 is dead** (2026-11-10): its red/black were reversed at the buck, so it sat reverse-biased on 5 V — driver fried; confirmed by swapping onto Fan 2's terminals. Replace with any 5 V USB fan, plug cut, red → buck OUT+, black → OUT−. Fan 2 verified good.
3. ~~Level sensor forgets address/baud on power loss~~ **RESOLVED 2026-11-10** by leaving it at factory (1 @ 9600) and moving the bus + probes to it (11-13 @ 9600). No re-commissioning ever needed. Pending: one more unplug/replug of the transmitter as the final proof (it should come back and simply keep reporting).
4. ~~Reservoir placeholders~~ **SET 2026-11-10**: reservoir is a 5-gallon bucket (`PG.TANK_L` 18.9 L), full depth 13 in (`ReservoirFullDepthMM` 330, live config scale 100/330). Lockout at 20 % = 2.6 in. Calibration note: transmitter read 16.4 cm at 6 in (15.2 cm) and 6.1 cm at ~3 in — zero/span against a ruler still worthwhile.
5. (unchanged) **All automation disabled for testing**: automation.enabled=false, maintenance.active=true, lighting schedule enabled=false. Re-enable deliberately when configuring.
6. **Drain-back time** default 300 s — confirm against the real tray on Shelf 2.
7. Code is uncommitted on main (relay remap, migrations 005/006, sensor seeding, UI changes).

## Public access (2026-11-10)

Cloudflare Tunnel `propagation-unit-01` (id c66ac740-a684-4e44-9e10-45663239e8bb) runs as the `cloudflared` systemd service on the Piconium
(`/etc/cloudflared/config.yml`, creds `/etc/cloudflared/propagation-unit-01.json`, origin cert `/etc/cloudflared/cert.pem`).
Hostname `unit01.shortsfieldstation.org` → `http://localhost:80`. Verified 200 for `/propagate/api/v1/health` and `/propagate/ui/touch` through the edge.
Public page = Worker `srfs-propagate-proxy` on route `shortsfieldstation.org/propagate*` (deploy/cloudflare/propagate-worker), noindex.

## Screen features (2026-11-10)

- **Hidden power-down**: press and hold the thin divider line under the title for 3 s → "Power down Unit 01?" popup → Shut Down stops any cycle, forces every relay off, logs `system_shutdown_requested`, and runs `sudo -n systemctl poweroff` (sudoers rule `deploy/sudoers/propagation-poweroff` installed in /etc/sudoers.d). Endpoint `POST /system/shutdown`, admin-only (kiosk = loopback admin).
- **Species catalog** mirrors the website's 29 woody plates + Inland Sea Oats (30; Monterrey Oak and Muscadine Grape dropped at Austin's request). Latin names shown in italics after the common name on the plate, ledger, detail, dashboard card, and in the picker. Reservoir labels now derive from `PG.TANK_L` (18.9 L).
- Page handlers send `Cache-Control: no-cache` so the kiosk picks up new bundles on restart.

## Log
- 2026-11-10 20:20 — **DASHBOARD CONTROL PANEL + DIALS** (Austin: "the whole point of the web interface… push buttons on the control panel"). Controls tab rebuilt (`web/src/design/controls.jsx`): rack-wide Watering Automation (on/off w/ confirm, maintenance, watering window HH:MM, reservoir cutoff), Lighting and Circulation Fans (schedule HH:MM to the minute + on/off/auto 2-h overrides), per-shelf rule (water-below / re-arm 0–100 %, soak, drain 0–60, min gap, soaks/day, apply to one or all) with Water now / Drain / Stop and live cycle state. Grower = manual buttons, Steward = schedules/rules/settings (backend roles). Environmental tab: line charts replaced by the rack face's DialGauge per shelf (moisture / root temp / EC vs bands); pump-time bars + adherence table kept. Moisture bounds opened to 0–100 everywhere (assign modal too). Verified in local simulation (Water now → flooding, confirms, saves). Hook: fans via /fans override API, state.fanOverride/fanOn/automation. **Then cut back on Austin's UX rule (20:40): a setting lives in exactly one place.** Per-shelf rule editor removed from Controls (shelf settings = Shelf Management edit modal only); the rack photoperiod removed from the assign/edit modal (lights + fans = Controls only); Controls keeps the three rack-wide panels plus a Shelves action row (Water now / Drain / Stop). Touch screen unchanged visually; its fan On/Off now a 2-h scheduler override instead of a raw relay set. **Binary staged on the Piconium — needs Austin's `sudo ./install.sh`.**
- 2026-11-10 19:50 — Watering automation turned back OFF until **Thursday 2026-11-12** (one more leak to fix); lights + fans stay scheduled. Safety `min_reservoir_level` briefly lowered to 10 % then **put back to 20 %** (Austin) until the pump has a bottom intake; 10 % is only ~3.3 cm above the level-sensor tip.
- 2026-11-10 19:40 — **AUTOMATION LIVE.** Lighting override cleared (lights off on schedule), drain-only run on tray 2 (valve 2 held 10 min, pump off), then maintenance OFF + automation ON. First soaks eligible 07:45 tomorrow on any shelf below trigger, one at a time.
- 2026-11-10 19:36 — **UPGRADED BINARY INSTALLED** by Austin (`ssh -t … sudo ./install.sh`; install.sh now needs `chmod +x` after scp and `scp -rp` to carry unit files). Health ok, bus online, all 4 sensors online (Shelf 3 probe now seated: 18.8 %). `automation.watering_window` set **07:45–11:00**; fan schedule seeded **13:30–15:00 enabled**; zone rules/species/1020 s/600 s all intact. Restart aborted a Shelf 2 water-now that was running (19:36:34) — tray 2 may be holding water with valve closed; a drain-only run clears it. Lighting manual-on override (touch, 18:09) restored after restart → lights stay on until cleared to auto. State: maintenance ON, automation OFF.
- 2026-11-10 18:14–19:15 — **ALL THREE VALVES DEAD, then fixed.** Austin ran "water now" on Shelf 3 from the touch screen: pump ran, solenoid silent; stopped at 18:14:32. Relay board read back ON for every valve channel on four pulse passes (pump off, maintenance on, one relay at a time via `POST /relays/{ch}/set` with `timeout_seconds`), so the controller and Numato were cleared. Fans (relay 4) worked at 19:14 → 12 V rail and buck fine. Cause (Austin): the **green whip conductor in the field box (Box 2) had been isolated during the afternoon fan-wiring work and not restored** — it is the common the valve circuit depends on. Re-landed; valve 3 confirmed 19:14:51, Austin reports all valves + fan working. **Lesson: the green umbilical conductor is the valve return; never leave it lifted. Add it to the umbilical pin table once Austin confirms the pin number.** State left: maintenance ON, automation OFF, all relays off except lights (schedule). Shelf 2 "water now" that was queued at 18:20 was cancelled during the test.
- 2026-11-10 ~18:10 — **Fan schedule added** (Austin: "schedule them like the lights"). `lighting.Service` is now parameterised by relay/rows; `NewFanService` drives `FansChannel` (relay 4) with schedule row `fans` (type `fans`) and override row `fans_override`. API mirrors lighting: `/fans/status|on|off|override|schedule`. **Austin's rule: fans run 90 min every afternoon → seeds 13:30–15:00 daily, enabled**, on first boot of the new binary (adjust via PUT /fans/schedule). (Recommendation given was follow-the-lights; Austin chose the 90-min afternoon run.) Hardware caveat unchanged: Fan 2 has an unswitched feed (runs regardless) and Fan 1's cable is faulty, so the schedule is moot until both are fixed. **Same staged binary as the watering window — not yet installed (needs Austin's sudo).** UI gap: the dashboard's assign flow covers per-shelf rules (lo/hi, soak min, drain min, min interval, max/day, EC/temp, lights on-hour + hours), but has NO control for the watering window, the fan schedule, or minute-resolution light times; the fan button is a raw relay toggle.
- 2026-11-11 ~21:45 — **Dashboard is an installable PWA with a phone layout** (Austin: control the rack from his phone). `web/public/manifest.json` + icons (from seal.png) + `sw.js` (installability only, caches nothing — a control panel must not serve stale state), Apple home-screen meta tags, routes in `internal/api/ui.go`. CSS `@media (max-width:760px)`: sticky wrapped header, 2×2 tabs, single-column cards, full-screen modals, 44 px+ touch targets, 16 px inputs (no iOS zoom), charts respond to touch. Control from the phone needs the home LAN address (`http://192.168.5.127/propagate/`); the public tunnel stays GET-only. The worker only registers on HTTPS/localhost; Add to Home Screen works either way. Touch-screen kiosk (`touch.html`) untouched.
- 2026-11-11 ~21:30 — **Environmental tab, per Austin's follow-ups:** one section per shelf (its own moisture / temperature / EC chart, Y range hugging that shelf); dashed straight line at each trace's window average, labelled at the right edge; shaded ideal band (moisture = the rule's trigger–re-arm, temp/EC = the shelf's happy-band); a water drop on the baseline wherever that shelf was watered (pump runs only — drains and purges excluded), hover names the run's length and trigger.
- 2026-11-11 ~21:15 — **ENVIRONMENTAL TAB = TIMELINE CHARTS** (Austin: drop the dials, line charts but more detailed). New `web/src/design/timechart.jsx`: SVG with a real date axis that scales with the window (24 h → every 3 h, midnight dated; 7 d → each day; 30 d → every 5 days; season → weekly; viewer's local clock), min–max band per shelf, period peak and low labelled, hover crosshair with timestamp + every shelf's value and bucket range. Data: `GET /sensors/{key}/readings?interval=5m` (NEW: raw readings bucketed server-side, min/avg/max/count, same shape as the hourly aggregates, ≤ 7-day window) for the 24-hour view; `interval=hour&limit=2400` for 7 d / 30 d / season. Pump-time bars and adherence table unchanged. Shelf-3 retest flood with the tank at 95.8 % bottomed at 43.9 % (full 7 min).
- 2026-11-11 evening — **LEAK TEST + RESERVOIR FINDINGS + POST-DRAIN PURGE.** a4dbb14 build installed 19:12 (control panel, dials, 180-day retention); the install restarted the daemon mid-soak on shelf 2 — startup safe-state shut the valve with the tray full (lesson: never install while a cycle runs; use Drain after). 7-min leak-test floods: shelf 1 leaked at 19:25 (fixed by Austin), rerun 20:20 clean; shelf 2 clean 20:33; shelf 3 cut at 99 s by the **reservoir_low lockout (19.1 % < 20 %)** — one tray flood pulls the tank 35–57 points (shelf 1: 81→27, shelf 2: 78→43, shelf 3: 76→19 and falling) and it comes back within ~3 min of pump-off. Austin: doubling the reservoir. Drain raised to **1200 s (20 min) on all zones**. **NEW post-drain purge** (Austin: water keeps trickling out of the pots after a drain and sits in the tray/pipe): `ZoneConfig.purge_delay_seconds` / `purge_seconds` — after a flood's drain-back closes the valve, the engine arms a timer; when it fires the same valve re-opens, pump off, as a drain-only cycle tagged `purge` through the normal one-shelf queue (events `irrigation_purge_scheduled/_cancelled/_skipped`; `engine.pending_purges` in `/irrigation/zones`; Stop on an idle shelf cancels its pending purge; a new flood supersedes it; a restart drops pending purges). Seeds 3600 s / 60 s; **loaded on all three zones = 60 min after drain, valve open 60 s**. Edited in the dashboard shelf modal (Purge after drain / Purge valve open); touch screen unchanged. Tests `TestPostDrainPurgeReopensValve`, `TestStopCancelsPendingPurge`. Rack left in maintenance (test mode), automation off, zone_2 rule disabled for the night.
- 2026-11-10 ~18:00 — **Austin's watering rules, in order:** (1) pump runs for the entire soak — yes by design, `flooding` = pump on for the full run duration, drain-back only after; (2) drain 10 min — `drain_seconds` 600 on all zones, done; (3) one shelf / one solenoid at a time — already guaranteed by the engine's single-worker FIFO queue (`irrigation/engine.go`), nothing to change; (4) water only in the morning after lights-on — **NEW `watering_window` on the `automation` setting** (`{"enabled":…, "watering_window":{"start":"07:45","end":"11:00"}}`): moisture triggers are only enqueued inside the window (block reason `outside_window`), schedules + manual runs ungated, dashboard toggle preserves the stored window, `{"start":"","end":""}` clears. Test `TestWateringWindowGatesMoistureTrigger`. **Binary built + staged on the Piconium (~anelsen/propagationd-linux-amd64, ~anelsen/deploy/) — NOT yet installed, needs sudo:** `cd ~/deploy && sudo ./install.sh ../propagationd-linux-amd64 ../propagation-watchdog-linux-amd64` (install.sh now restarts the service). Then set the window via PUT /settings.
- 2026-11-10 ~18:00 — **FIRST REAL CONFIGURATION LOADED** (Austin's layout, via loopback API). Shelf 1 = Bald Cypress + Texas Persimmon; Shelf 2 = Buttonbush + Inland Sea Oats; Shelf 3 = Texas Mountain Laurel + Chickasaw Plum (milkweed + purple prairie clover germinate raised on 2, then live on 3). Ebb-and-flow soak-and-dry for all natives: `default_run_seconds`/rule run = **1020 s (2 min fill + 15 min soak, pump on throughout)**, `drain_seconds` = **600**. Moisture rules (probe %, trigger_below / recovered_above / min interval / max per day): z1 16/24/720 min/1, z2 22/26/480 min/2, z3 10/18/2880 min/1, sensor offline = lockout. EC bands z1 50–600, z2 100–800, z3 50–400. Lighting schedule **07:30–17:05** America/Chicago (≈ sunrise+30 / sunset−30 for Nov 10–24; re-set at the two-week review ~Nov 24, December wants ~07:50–17:00); manual-on override cleared, relay followed the schedule off at 17:52 check. **automation.enabled still false, maintenance.active still true** — Austin flips them from the touch screen once the Shelf 3 probe is seated in media (it reads 0 % in air and would trigger a flood on the spot). Two-week review: `tools/export_readings.py` dumps hourly/raw/cycles CSV; raw retention is 30 days. Not implemented: early-stop of a soak when moisture recovers (recovered_above is hysteresis only, automation/service.go); candidate engine change.
- 2026-11-10 PM — **Probes 1 and 2 swapped between shelves** (Austin, cord length). Cable "2"/addr 12 now on Shelf 1, cable "1"/addr 11 on Shelf 2. Fixed in software, not by re-addressing: `PATCH /sensors/soil_zone_1/config` → addr 12, `soil_zone_2` → addr 11 (loopback, hot-reloaded); zone keys and moisture rules unchanged; seed defaults in `sensors/service.go` updated to match. **Found while doing it: the whole RS-485 bus has been dead since the 15:32 reboot** — reservoir (addr 1) and all three probes time out, scan 1–13 answers nothing, last good readings 13:27 CST. Both USB adapters re-enumerated at 15:32. Suspects: sensor-side supply off during the rewire, A/B disturbed, or a stale FTDI handle from the daemon opening the port before udev settled (fix = `systemctl restart propagationd`; needs sudo, not available over ssh as the ordinary user). **Resolved 17:07 CST: Austin plugged the sensor side back in; all four devices came back online on their own, no daemon restart needed, lockout cleared.** Shelf 1 (addr 12) 20.0 %, Shelf 2 (addr 11) 18.7 %, Shelf 3 in air 0 %.
- 2026-11-10 — First real watering: Shelf 2, 60 s manual via engine. valve 2 + pump on → pump off at 60 s → 5 min drain with valve held → valve closed, cycle complete, reservoir 64 % → 52 % (placeholder scale). Lights on/off via lighting service verified. Then Fan 2 observed running with relay 3 off (see open item 1); everything switched off, parked for the night.

- 2026-11-09 — **PUMP FAULT.** With every relay reading off (API + readall), the pump started the instant it was plugged into the gang box. No unswitched hot exists in the gang box by design, so either R6 is wired COM→NC (load live when de-energized) or H1 is landed on filtered line, bypassing the relay. Austin powering down to inspect the relay. All automation disabled until testing is done: settings automation.enabled=false, maintenance.active=true, lighting schedule enabled=false, no irrigation schedules. Shelf 1 zone re-enabled (was left disabled by July's sim play). UI: zone_4 removed (migration 006), probe presence now sensor-driven (0 %VWC in air is a reading, not "no probe"), per-shelf temperature, plates sized to shelf count, dial 100 px.
- 2026-11-09 — **REAL MODE.** All three probes (1/2/3) + level sensor (6) scanned clean together on one bus at 4800. Removed `10-simulation.conf` (kept at /root/10-simulation.conf.disabled), daemon restarted: owns /dev/ttyACM0 + /dev/ttyUSB0, health reports modbus_bus online / relay_controller online, all 8 relays off. First real readings: Shelf 1 in soil 18.6 % / 20.5 °C / 36 µS; Shelf 2 in soil 13.9 % / 26 °C / 27 µS; Shelf 3 in air 0 / 23 °C / 0 (unplanted); reservoir 6.1 cm in a bucket = 20.3 % of the 30 cm PLACEHOLDER full depth (safety min is 20 % — right on the edge until the real depth is set). Sensor seeding now per-shelf (soil_zone_1..3 at addr 1..3, reservoir at addr 6 reg 4, `ReservoirFullDepthMM` placeholder 300); zones 2/3 linked to their probes via PATCH. Stale simulation-era "running" cycle on zone_2 marked aborted. zone_1 is `enabled=0` from July's sim play — Austin re-enables it when he assigns a specimen.
- 2026-11-09 — Level sensor: scanned bus (9600/4800/19200 × addr 1–32), found at 1/9600, level word confirmed by dipping in water (0 → 61), moved to addr 6 @ 4800.
- 2026-11-09 — Solenoids: all three valves pulsed one at a time, each clicked on its own shelf. P1/relay 0 → top, P2/relay 1 → middle, P3/relay 2 → bottom. Matches the drawing exactly.
- 2026-11-09 — Fans: relay 3 (software 4) verified with fans connected. Fan 1 cable faulty, flagged for replacement.
- 2026-11-09 — Piconium on Wi-Fi (Pattonhaus-Devices, 192.168.5.127). Numato
  at `/dev/ttyACM0`, FTDI at `/dev/ttyUSB0`. Lights (silkscreen 7) and fans
  (silkscreen 3) pulsed directly over the Numato serial protocol with the
  daemon in simulation; both loads responded. Software remapped to this
  table (manager.go, engine.go seeds, migration 005, UI fan constant) and
  deployed; API confirms new names and zone valve channels 1/2/3/5.
