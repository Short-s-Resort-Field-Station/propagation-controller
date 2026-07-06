# Propagation Controller

Backend + UI for **Propagation Unit 01** — a local-first embedded Linux control
appliance for automated plant propagation. See [`docs/SPEC.md`](docs/SPEC.md)
for the full architecture spec and [`docs/HARDWARE.md`](docs/HARDWARE.md) for
the hardware inventory.

## Status: Phases 0–7 complete (of 8 phases)

Phase 7 (data retention & aggregation):

- Hourly aggregates computed continuously (5-min refresh over a 48 h window,
  idempotent upserts); raw readings pruned daily after 30 days (configurable
  via settings key `retention`), always aggregated before deletion
- `GET /api/v1/sensors/{key}/readings?interval=hour` serves the long-term series
- **Environmental tab now charts real hourly aggregates** (dynamic y-bounds,
  per-shelf legend, honest "collecting data" state) and watering minutes from
  real cycle history
- **Controls tab writes through**: sliders load each zone's actual moisture
  rule + the rack photoperiod; Review & Apply PUTs moisture-control and the
  lighting schedule (single-shelf or all-shelves scope); manual-override
  toggle drives the real lighting service
- Dashboard tabs are deep-linkable (`/#environmental`, `/#controls`, `/#shelves`)

**The production frontend is live** — built from the design handoff's actual
components (see `web/design-src/` for the extracted originals):

- `/` — Surface B, the paper control center: Dashboard / Shelf Management /
  Environmental / Controls tabs, live stat cells, field-notebook event log,
  reservoir gauge, alerts + acknowledge flow, specimen assignment (fed by the
  real species register), sign-in modal
- `/ui/touch` — Surface A · Round 2, the sculptural instrument face: fixed
  880×528 canvas auto-scaled to the 800×480 panel, needle-dial shelf plates,
  live status chip (Operational/Watering/Fault), shelf detail with Water Now,
  rack-wide Lighting and Fans On/Off (wired to the real relays)
- Live bridge: `web/src/live/` polls the REST API at 2.5 s and maps responses
  onto the handoff's §7 data contract; every action carries its surface's
  actor (`manual_web` / `manual_touch`)
- Self-hosted fonts (Zilla Slab, Newsreader, JetBrains Mono) — no CDN, works
  fully offline
- `make web` builds the frontend and embeds it in the binary; without it the
  Phase 0 shells still serve

Backend:

Working now:

- `propagationd` Go daemon with SQLite (WAL) and embedded migrations (full v1.0 schema)
- Health API, relay API with durable action logging and **server-enforced timeouts**
- Numato 8-channel USB relay driver (echo handling, 1→0 channel mapping) + simulation mock
- **Modbus RTU bus** (goburrow) over the FTDI RS485 adapter + simulated bus with drifting values
- **Generic JSON-configured Modbus sensor driver** — new sensors are config, not code
- Per-sensor polling loops with immediate first poll, batched reads into SQLite,
  offline detection (N consecutive failures) with status-change events, recovery events
- Sensors API: list/latest/history/force-poll + **config hot-reload** (PATCH restarts the poller, no service restart)
- Diagnostics: bus health + **address scan** (`POST /api/v1/diagnostics/modbus/scan`)
- **Safety supervisor** (spec §12): every relay-on command is authorized; off is never blocked
  - Emergency stop (web / touch / API) — forces pump+valves off, latches until manual reset,
    re-enforces the safe state every second while latched
  - Pump max-runtime enforcement → forced off + manual-clear lockout
  - Reservoir low / sensor-stale lockouts (auto-set, auto-clear on recovery)
  - Safety API: status, emergency-stop, reset, lockouts, lockout clear
  - Limits editable at runtime via `PUT /api/v1/settings` (`safety` key, hot-applied)
- Settings API (GET/PUT key-value JSON)
- **Irrigation engine** (spec §9): 3 shelf zones + 1 disabled for expansion, shared manifold model
  - **Strict single-zone rule: a FIFO queue serializes all watering — two shelves can never flood at once**
  - Cycle state machine: precheck → open valve → start pump → flood → stop pump → (drain) → close valve
  - Valve opens before pump starts; pump stops before valve closes (never pump a closed manifold)
  - Optional per-zone moisture target ends flooding early; duration is the hard cap
  - Relay backstop timers on pump and valve — a dead process can't leave water running
  - Manual runs (web/touch/API) with per-zone default duration; stop aborts active or dequeues
  - Full cycle history: trigger, durations, reservoir/moisture start+end, result, abort reason
  - Safety-integrated: precheck + mid-flood lockout/e-stop watch aborts within 250 ms
- **Lighting service** (spec §10): rack-wide photoperiod schedule (default 06:00–19:30),
  manual on/off overrides with optional expiry, edge-triggered control loop that
  **restores the expected state at startup** and never fights manual relay commands
- **Circulation fans**: all ganged on relay 7, simple on/off toggle (touch + API)
- **Species catalog**: 21 specimens from the propagation manual at
  `GET /api/v1/species` (provisional — regenerated when Austin's new document
  replaces the A–U plate system); shelves take species assignments via zone PATCH
- **Automation service** (spec addendum + §11): both triggers only ever Enqueue,
  so the one-shelf-at-a-time rule holds for automation too
  - Moisture rules per zone: trigger-below threshold, hysteresis (recovered-above),
    minimum interval, max cycles per rolling 24 h, sensor-offline lockout;
    `GET/PUT /api/v1/irrigation/zones/{key}/moisture-control`
  - Scheduled irrigation: time-of-day rules in the schedules table with full CRUD
    (`/api/v1/schedules`); missed runs are skipped after downtime (spec §11.2)
  - Global automation kill switch: settings key `automation` `{"enabled": false}`
  - Blocked-trigger events are de-duplicated (one event per condition, not per tick)
- Safe startup/shutdown state: pump + valves forced off
- System event log (database + structured process log)
- Embedded web dashboard (`/`) with live sensor tiles and 800×480 touch shell (`/ui/touch`)
- systemd units + installer skeleton

**Authentication & remote access (Phase 8):**

- Real accounts: bcrypt passwords, hashed session tokens, HttpOnly cookies
- Roles: Steward (admin) · Grower (operator) · Viewer — **all reads are
  public** (followers watch, can't touch); control needs Grower, config needs
  Steward; the rack touchscreen is trusted via loopback (no sign-in at the unit)
- First-boot setup wizard creates the steward account (local network only)
- Operator management UI (add/disable/re-role/reset password, last-admin guard)
- `docs/remote-access.md`: serving `unit01.shortsfieldstation.org` from the
  Piconium via Cloudflare Tunnel (or Caddy)

**Live streaming & the watchdog (Phase 8, complete):**

- SSE at `GET /api/v1/events/stream` (spec §16): system events, relay actions,
  sensor readings, and irrigation cycle updates push live; the UIs refresh
  instantly on push and fall back to polling if the stream drops
- `propagation-watchdog` binary: health-checks propagationd, restarts it on
  sustained failure, and if restarts don't recover it, stops the service,
  takes the relay port, and forces pump/valves/fans off (escalation ladder
  unit-tested; resurrection verified end-to-end)

Remaining before hardware: optional rack PIN lock screen. Hardware-dependent:
real Numato/Modbus bring-up, kiosk deployment, hardware-in-the-loop safety
tests, pump flow calibration.

## Development (no hardware needed)

```sh
make run-sim        # simulated relay controller, pretty logs, ./data/propagation.db
open http://localhost:8080          # web dashboard
open http://localhost:8080/ui/touch # touch shell
```

```sh
make test           # unit tests (driver protocol, manager, storage)
make linux          # static x86_64 binary for the Piconium
```

## Try the API

```sh
curl -s localhost:8080/api/v1/health | jq
curl -s -X POST localhost:8080/api/v1/relays/2/set \
  -H 'Content-Type: application/json' \
  -d '{"state":"on","reason":"pump test","timeout_seconds":10}'
curl -s localhost:8080/api/v1/relay-actions | jq
curl -s -X POST localhost:8080/api/v1/relays/all-off
```

## Layout

Follows spec §5. Notable deviations:

- Migrations live in `internal/storage/migrations/` (Go embed requires them
  inside the package tree); the full v1.0 schema ships as `001`.
- Phase 0 web shell is embedded static HTML under `internal/api/static/`;
  the React app (spec §2.3) arrives in Phase 5 under `web/`.

## Hardware deployment

See `docs/deployment.md` for the Debian 13 install plan (NVMe root), kiosk
setup, udev/serial notes, and the `deploy/` scripts.
