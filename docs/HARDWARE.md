# Propagation Unit 01 — Hardware Inventory

Updated 2026-07-05 with Austin's answers. Soil-probe section updated 2026-07-10 after reverse-engineering the ZTS-3000 (moisture / temperature / EC, **no pH**) against the real hardware.

## Main Board — Piconium (Embedded Now), one of two kits

| | Kit 002 (likely) | Kit 003 |
| --- | --- | --- |
| CPU | Intel Pentium N3710 quad-core (Braswell) | Intel Atom E3845 quad-core (Bay Trail) |
| RAM | 4 GB DDR3L 1600 MT/s | 4 GB DDR3L 1333 MT/s |
| Storage | **6 GB eMMC** | **8 GB eMMC** |
| Temp range | 0–60 °C | (industrial) |

Both are **x86_64** → Go build target `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`.

Notes:
- **Storage is the tightest constraint** — 6–8 GB eMMC must hold OS + Chromium + app + database. Requires a slim Debian install (no desktop task, doc/locale stripping). Chromium is the biggest disk consumer; fallback option is `cog` (WPE WebKit kiosk browser, ~10× smaller) if space gets tight.
- Kit 002 (N3710) is the better CPU for Chromium rendering but has 2 GB *less* storage than Kit 003. Both workable; N3710 preferred, plan the slim install either way.
- If Kit 003 (E3845 / Bay Trail): known Linux c-state erratum causes random freezes — add `intel_idle.max_cstate=1` to kernel cmdline.
- eMMC wear mitigation: SQLite in WAL mode with `synchronous=NORMAL`, size-capped journald, no separate file logging.
- Network: Wi-Fi, with internet access (Austin confirmed) → NTP available via systemd-timesyncd. Still confirm whether the board has onboard Wi-Fi or needs a USB adapter, and whether it has a battery-backed RTC (matters for schedules during network outages).

## Chosen OS

**Debian 13 "trixie" (amd64), minimal netinst install.** Rationale: stable, long support window, non-free firmware (Wi-Fi) included in installer since Debian 12, `cage` + `chromium` in main repos for the kiosk, systemd native. Details in `propagation-controller/docs/deployment.md`.

## Peripherals

| Component | Details | Status |
| --- | --- | --- |
| Relay module | Numato 8-ch USB relay | Known — USB CDC serial, ASCII protocol (below) |
| Touchscreen | Waveshare 5" 800×480, HDMI video + USB touch | **Works as-is** — HDMI + standard USB HID touch, no drivers needed on Linux. Touch UI will be designed for exactly 800×480. No new screen required. Austin has several on hand. |
| RS485 adapter | Xiusiyt RS485-WE-1800-BT (UPC 714855564012) | **FT232RQ + SP485EN** (genuine FTDI, matches spec) → mainline `ftdi_sio` driver, `/dev/ttyUSB*`, stable `/dev/serial/by-id/usb-FTDI_*` path. SP485EN gives automatic TX direction via TXDEN. Tx/Rx LEDs on the connector help bus debugging. Pinout: GND/black, Data−/yellow, 5V/red, Term1/brown, Data+/orange, Term2/green — jumper Term1↔Term2 to insert the 120 Ω termination at the adapter end of the bus. |
| Grow lights | Barina — relay 1 | **120 VAC** — only mains load. Confirm wattage/amps vs relay contact rating. Rack-wide control only (per-shelf cuts via the physical side switch — Austin, 2026-07-05). |
| Circulation fans | 12 V USB desk fans, USB plug stripped, single speed — **all ganged on relay 7** | Simple on/off toggle in the GUI (Austin: keep it simple). One relay for all fans preserves relay 6 for Shelf 4's valve and relay 8 as the last spare. The design handoff's per-shelf 0–10 fan stepper is superseded by this decision. |
| Pump | SEAFLO diaphragm — relay 2 | **12 VDC.** ⚠ Check current draw: SEAFLO diaphragm pumps commonly pull 4–8 A at 12 V; Numato contacts typically rated ~7 A @ 30 VDC. If near limit, drive a 12 V automotive relay from the Numato channel instead of switching the pump directly. |
| Valves | 4× U.S. Solid JFSV00012 (USS2-00012) solenoid — relays 3–6 | 12 VDC, **normally closed** → fails closed on power loss (correct fail-safe: de-energized = no water). Coil draw well under 1 A, fine for the Numato contacts. Add flyback diodes across each coil (and pump). Verify whether the valve is direct-acting (works at 0 PSI) or pilot-operated (needs pressure differential) for any gravity-drain path. |
| Soil sensors | **ZTS-3000-TR-ECWS-N01** (OEM Jinan Zhaotaisheng) — 3-prong moisture / temperature / **EC** probe, FDR-class dielectric, 4.5–30 V DC, IP68, Modbus RTU. **No pH and no humidity channel.** | **Reverse-engineered and verified 2026-07-10** against the real probe. Modbus **addr 1, 4800 baud 8N1**; function codes 0x03 and 0x04 return the same data. Registers: `0x0000` moisture ×10 %VWC · `0x0001` temperature ×10 °C (signed) · `0x0002` EC in µS/cm (0–20000 ceiling). Salt-water test confirmed a genuine dielectric response (EC 246→20000 moved moisture only 92.0→88.2%). Driver reads register 2 as substrate EC end-to-end — there is no pH anywhere in the stack. Still editable live via `PATCH /api/v1/sensors/{key}/config`. |
| Reservoir sensor | Submersible hydrostatic level transmitter, SS304, IP68, RS485 output variant, 12–30 V DC, 0.2% accuracy | Reports level (water column); converting to tank % needs an empty/full calibration (register scale/offset or the config service later). Register map also vendor-specific — probe at bring-up. |

### RS485 bus wiring & bring-up notes

- One twisted pair daisy-chained: adapter Data+ (orange) → all sensor A/+ lines; Data− (yellow) → all B/− lines. **Common ground** between the 12 V sensor supply and the adapter GND (black). Sensors are powered from the 12 V rail, *not* the adapter's 5 V pin.
- ⚠ **Address collision:** the ZTS-3000 probes ship at Modbus address 1. With multiple sensors on one bus, connect them **one at a time** and re-address each before adding the next — the address lives in holding register `0x07D0` (verified), and `cmd/probe-address` does the read-verify-write safely. The diagnostics scan endpoint (`POST /api/v1/diagnostics/modbus/scan`) finds whatever is responding.
- Bus defaults: **4800 baud, 8N1** — the ZTS-3000 factory default (verified; the baud code lives in register `0x07D1`, where `1` = 4800). The whole bus runs one baud, so the reservoir level transmitter must match — set it to 4800 at bring-up if it doesn't already answer there.

## Numato 8-Channel USB Relay — Protocol Notes

- Enumerates as USB CDC ACM serial (`/dev/ttyACM*`; use `/dev/serial/by-id/` for stability).
- ASCII commands (baud ignored): `relay on N`, `relay off N`, `relay read N`, `relay readall`, `relay writeall xx`, `ver`, `id get`.
- **Channels are 0-indexed (0–7) on the wire.** Spec numbering is 1–8 → driver maps N → N-1. Implemented in `numato_usb.go`.
- Device **echoes commands** before responding; responses end with a `>` prompt.
- `relay read` returns commanded coil state, not measured contact state.
- Power-up default: all relays OFF (fail-safe with loads on NO contacts).
- Host crash/USB disconnect: relays **hold last state** → wire pump/valves to **normally-open** contacts; consider a hardware pump run-timer for defense in depth.

## Relay Budget (updated 2026-07-05)

| Ch | Load |
| --- | --- |
| 1 | Barina grow lights (120 VAC, rack-wide) |
| 2 | SEAFLO pump |
| 3–5 | Solenoids, Shelves 1–3 |
| 6 | Solenoid, Shelf 4 (future) |
| 7 | Circulation fans (all, ganged) |
| 8 | Spare |

Expansion to 6 shelves exceeds this board — plan a second Numato module (the relay driver abstraction already supports multiple controllers conceptually; needs a driver-registry extension).

## Remaining Open Questions

1. ~~Soil probe register map~~ — **done.** ZTS-3000 reverse-engineered 2026-07-10 (map in the Peripherals table). The **reservoir level transmitter** register map and baud are still unconfirmed; probe at bring-up (scan + trial reads) and set it to 4800 baud to share the bus.
2. SEAFLO pump model / current rating — to confirm direct relay switching is safe.
3. Barina light wattage — mains-side contact rating check.
4. Piconium: onboard Wi-Fi or USB adapter? Battery-backed RTC?
5. Which kit was ordered — 002 (N3710/6GB) or 003 (E3845/8GB)?
