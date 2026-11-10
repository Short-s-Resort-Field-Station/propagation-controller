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
| **11** | ZTS-3000 soil probe (cable label "1") | Shelf 1 (top) | **2026-11-10** `ztsmove.py 1 4800 11 2` (reg 0x07D0=11, 0x07D1=2 ⇒ 9600) |
| **12** | ZTS-3000 soil probe (cable label "2") | Shelf 2 (middle) | **2026-11-10** `ztsmove.py 2 4800 12 2` |
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
