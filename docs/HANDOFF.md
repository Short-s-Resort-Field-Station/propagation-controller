# HANDOFF — Propagation Unit 01

**To the next Claude working with Austin on this project.** This document is
your orientation. It was written by Claude Fable 5 on 2026-07-06, at the point
where the software is feature-complete and verified, and what remains is
hardware bring-up plus a couple of loose ends. Read this fully before
touching anything.

Also read: the persistent memory at
`~/.claude/projects/-Users-austinnelsen/memory/` — `MEMORY.md` indexes it,
`project_propagation_unit1.md` is the running log of this project with
per-session detail. And `docs/SPEC.md` (the contract for everything) +
`docs/HARDWARE.md` (physical inventory and wiring notes).

---

## 1. Who you're working with

Austin Nelsen (austin@nativeplanet.io). Experienced builder (Urbit apps,
embedded tinkering), pragmatic, gives design feedback in batches of concrete
notes ("the reservoir numbers bump into the bottom line", "watering needs up
to 30 minutes"). Working style he expects:

- **Verify everything live.** Don't claim something works — run it in
  simulation, exercise it over HTTP, screenshot the UI headlessly, show him.
  He has caught nothing yet because every claim was tested first. Keep it so.
- **Raise a concern once, then defer.** If he reaffirms a decision, stop
  relitigating (this is a standing preference from other projects).
- **Ask hardware questions before assuming.** He'll answer fast and precisely.
- He keeps a live simulation running in two browser windows (dashboard +
  touch app window). After UI changes, rebuild and reload his windows for him
  (AppleScript recipe in §7). Tell him when you've done it.
- Safety engineering is the product. Every layer that protects the pump and
  plants is intentional and load-bearing.

## 2. What this is

A local-first embedded-Linux appliance that automates a 4-shelf plant
propagation rack: irrigation (pump + per-shelf solenoids), lighting, fans,
soil/reservoir sensing, logging, safety. One Go daemon (`propagationd`),
SQLite, two UIs served from the same binary:

- `/` — "paper" web control center (Grafana-meets-field-notebook aesthetic)
- `/ui/touch` — 880×528 "instrument face" for the 5-inch rack touchscreen

**One backend.** Both surfaces are pure views of the same state (SSE push +
polling fallback). This was explicitly confirmed to Austin and matters to him.

The physical unit ("Piconium" by Embedded Now, x86_64, 4 GB RAM, NVMe) hasn't
been assembled yet. Everything so far runs in **simulation mode**
(`make run-sim`) with mock relay/Modbus drivers producing realistic drifting
values.

## 3. State of completion (verified, not aspirational)

All 12 Go test packages pass. Everything below was also exercised end-to-end
over HTTP and, where visual, screenshot-verified in headless Chrome.

| Area | Status |
| --- | --- |
| Relay control (Numato USB driver + mock), audit-logged, server-side timeouts | Done |
| Modbus RTU bus (FTDI) + JSON-configurable generic sensor driver + polling | Done (register maps are PLACEHOLDERS — see §8) |
| Safety supervisor: e-stop latch, pump max-runtime, reservoir low/stale lockouts | Done |
| Irrigation engine: **strict one-shelf-at-a-time FIFO queue** (Austin's hard rule), state machine, moisture-target early stop, cycle history | Done |
| Lighting: rack-wide photoperiod schedule, overrides, restore-after-reboot | Done |
| Automation: moisture-trigger rules (hysteresis, min-interval, max/day, offline lockout) + scheduled watering + global kill switch | Done |
| Retention: hourly aggregates (kept forever), 30-day raw pruning (aggregate-before-delete) | Done |
| Both UIs: built from the actual design-handoff components, live data, all of Austin's review notes implemented | Done |
| Auth: bcrypt+sessions, roles admin(Steward)/operator(Grower)/viewer, **public read-only for anonymous** (his "followers watch, can't touch"), first-boot setup wizard, operator management UI | Done |
| SSE live push + polling fallback | Done |
| Watchdog binary (restart ladder → seize relay port → force safe state) | Done, resurrection tested |
| systemd units, installer, deployment + remote-access docs | Done |
| Cross-compiled Linux binaries in `bin/` (propagationd + watchdog, static, no CGo) | Done |

## 4. Immediate loose ends (do these first)

1. **GitHub push.** The repo is committed locally (single commit, 153 files).
   Austin wants it in a free-standing **organization** with `dalhec-banler`
   as a member. He accidentally created the org while logged in as a Native
   Planet account; the fix in flight: invite `dalhec-banler` as org Owner
   from that browser session, accept, then:
   ```sh
   gh config set git_protocol https   # his SSH keys are broken — HTTPS only
   gh repo create ORG_NAME/propagation-controller --private --source . --push
   ```
   Note: `gh auth status` says the account is "abnels" but the token actually
   resolves to `dalhec-banler` (account was renamed; keyring label is stale).
   Verify with `gh api user --jq .login`.
2. **Optional rack PIN lock screen.** Design exists (`LockScreen` in
   `web/src/design/flows.jsx`, dark PIN pad). Currently unused because the
   rack is trusted via loopback. Only build if Austin asks.
3. **Species catalog**: regenerate from Austin's manual if he revises it
   again (`internal/species/catalog.go`; the manual PDF is in the parent
   folder, extraction recipe in memory).

## 5. Architecture tour (repo = `~/Desktop/Propogation Unit 1 Software/propagation-controller/`)

```
cmd/propagationd/          main wiring; startup order matters (spec §12.2)
cmd/propagation-watchdog/  independent supervisor binary
internal/
  api/                     chi router, handlers, auth middleware, SSE, embedded UI
    webdist/               generated by `make web` (gitignored, .gitkeep placeholder)
    static/                Phase-0 fallback shells (kept; served if webdist empty)
  auth/                    bcrypt, sessions, roles, last-admin guard
  automation/              moisture rules + schedule evaluation → Enqueue only
  bus/                     SSE fanout hub (non-blocking, nil-safe)
  config/                  flags/env (simulation, ports, listen addr)
  devices/
    manager.go             ONLY path to relays: audit log + timeout timers
    relay/                 Numato driver (echo handling, 1→0 index map) + mock
    modbus/                RTU bus (goburrow) + drifting MockBus
    sensors/               generic JSON-configured driver + pollers + hot reload
  irrigation/              FIFO queue engine + cycle state machine
  lighting/                edge-triggered photoperiod service
  retention/               hourly aggregation + raw pruning
  safety/                  supervisor: Authorize() gates every relay-on
  storage/                 sqlite repos + embedded migrations (001, 002)
  watchdog/                testable escalation ladder (Decider)
web/
  design-src/              PRISTINE extracted design components — never edit
  src/design/              same components converted to ES modules (edits marked)
  src/live/                api client, adapter (backend→design shapes), SSE hook,
                           auth UI, overrides.css (ALL visual tweaks live here)
  src/main-dashboard.jsx   "/" entry     src/main-rack.jsx  "/ui/touch" entry
deploy/                    systemd units + install.sh
docs/                      SPEC, HARDWARE, deployment, remote-access, this file
```

Control flow rule: **nothing touches hardware except `devices.Manager`**, and
nothing turns a relay on without passing `safety.Supervisor.Authorize()`.
Irrigation NEVER runs two zones at once — every water request (manual, web,
schedule, moisture trigger) goes through `irrigation.Engine.Enqueue`. There
is a test that fails if two valves are ever open together. Do not weaken it.

## 6. Decisions with reasons (don't accidentally undo these)

- **Valve opens before pump; pump stops before valve closes.** Never pump
  against a closed manifold. Austin approved this ordering deviation.
- **Drain phase defaults to 0 s** (his plumbing has no drain-back), but stays
  configurable per zone.
- **Lighting is rack-wide** (one relay). Per-shelf cuts happen at a physical
  switch. UI labels say "· Rack". Photoperiod UI = "lights on at + hours of
  light" (off time computed) so on/off can never exceed 24 h — his request.
- **Fans: all ganged on relay 7, single on/off** (single-speed 12 V USB desk
  fans, stripped plugs). The design's 0–10 stepper was superseded.
- **Relay budget is FULL at 6 shelves** (1 lights, 2 pump, 3–6 valves, 7 fans,
  8 spare). Expansion needs a second Numato board.
- **No buzzer. Ever.** Removed from alerts UI.
- **Reservoir = 7 gal = 26.5 L** (`PG.TANK_L`). `PG.EST_DAILY_L = 4` is a
  placeholder — calibrate real pump flow at hardware bring-up and fix both
  the days-remaining estimate and the shelf "usage" display (currently
  minutes of pump time, honest but crude).
- **Watering duration 1–30 min** end-to-end (big pots soak 20 min). Zone caps
  1800 s; safety pump backstop 1860 s (self-migrates from old 300 s configs).
- **Auth trust model**: all GETs public; loopback WITHOUT
  X-Forwarded-For/CF headers = implicit "Rack Console" admin (the kiosk);
  proxies always add XFF so internet traffic can't impersonate the rack.
  First-boot setup only accepted from loopback/RFC1918 without proxy headers.
- **Probe-less shelves are "unmetered", not "Dry"** — no moisture judgments
  without a fresh sensor reading; display shows "NO PROBE", never a fake 0
  or an em-dash (Zilla 900 em-dashes render as heavy bars — Austin's "black
  lines" complaint).
- **Design aesthetic is contractual.** The handoff's tokens/fonts/vocabulary
  are fixed. All visual adjustments go in `web/src/live/overrides.css` with
  comments; `web/design-src/` stays pristine. The legibility pass (~+30%
  type for the 186 PPI 5" panel) lives there in one marked block.

## 7. Dev workflow recipes

```sh
cd "~/Desktop/Propogation Unit 1 Software/propagation-controller"
go test ./...                 # 12 packages, all must pass
make run-sim                  # daemon on :8080 (Austin's live sim uses :8090)
make web                      # npm build → internal/api/webdist → go build
make linux                    # both static x86_64 binaries into bin/
```

Austin's live sim (keep it running for him; his data dir is `./data`):
```sh
pkill -f propagationd; (./bin/propagationd -simulation -listen :8090 -data-dir ./data > /tmp/propd-user.log 2>&1 &)
```

Reload his browser windows after UI rebuilds:
```sh
osascript -e 'tell application "Google Chrome"
  repeat with w in windows
    repeat with t in tabs of w
      if URL of t contains "localhost:8090" then tell t to reload
    end repeat
  end repeat
end tell'
```

Headless screenshot verification (macOS headless Chrome has a ~87 px
viewport offset — add it to the height):
```sh
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless --disable-gpu --screenshot=/tmp/x.png \
  --window-size=800,567 --hide-scrollbars --virtual-time-budget=8000 \
  "http://localhost:8090/ui/touch"          # 800×480-equivalent rack view
```
Deep links for testing: `/#environmental` `/#controls` `/#shelves` (dashboard
tabs), `/ui/touch#shelf-1` (detail), `/ui/touch#theme-light` (limestone).

## 8. Hardware bring-up (the finish line)

Follow `docs/deployment.md` (OS) and `docs/remote-access.md` (domain). Order:

1. **Debian 13 minimal onto the NVMe.** If the board is the Atom E3845 (Kit
   003): add `intel_idle.max_cstate=1` to the kernel cmdline (Bay Trail
   freeze erratum). N3710 (Kit 002) needs nothing special.
2. `make linux`, scp both binaries + `deploy/`, run `install.sh`. The service
   binds :80 via CAP_NET_BIND_SERVICE.
3. **Numato relay**: shows as `/dev/serial/by-id/*Numato*` (CDC ACM). The
   driver autodetects; pin with `-relay-port` in the unit file env. Remember:
   wire span loads to **normally-open** contacts (de-energized = off); relays
   HOLD STATE if the host dies — that's why the watchdog exists.
4. **RS485 chain**: FTDI adapter (`/dev/serial/by-id/usb-FTDI*`), 9600 8N1.
   ⚠ All generic sensors ship at Modbus address 1 — connect ONE AT A TIME
   and re-address (vendor register, often 0x07D0; probe with
   `POST /api/v1/diagnostics/modbus/scan`). Jumper the adapter's Term1↔Term2
   for 120 Ω termination. Common ground between 12 V rail and RS485 GND.
5. **Register maps**: the seeded configs (moisture@0 ×0.1, temp@1 int16 ×0.1,
   pH@3 ×0.1; level@0 ×0.1) are guesses at the common generic layout. Probe
   with force-poll + hot-reload (`PATCH /api/v1/sensors/{key}/config` — no
   restart needed) until values look sane. Level sensor reports water column;
   calibrate empty/full → % via scale/offset.
6. **Kiosk**: cage + chromium at `http://localhost/ui/touch` (unit file
   in deploy/). The Waveshare 800×480 needs no drivers.
7. **Hardware-in-the-loop safety tests**: run the spec §24.5 list — pump
   timeout for real, reservoir lockout with the actual sensor, relay
   controller unplug mid-cycle, reboot during irrigation, watchdog force-safe
   with a real port seize.
8. **Calibrate pump flow** (time a measured fill) → update `PG.EST_DAILY_L`
   and consider converting the usage display to real liters.
9. **cloudflared** for `unit01.shortsfieldstation.org` (exact commands in
   remote-access.md). Austin owns the domain; apex is reserved for his
   separate main-site project. He must run `cloudflared tunnel login` (his
   Cloudflare account).
10. First boot on real hardware: dashboard shows the Unit Setup screen →
    Austin creates his steward account (must be done from the LAN).

## 9. Sharp edges (each of these cost real debugging time)

- **modernc/sqlite stores `time.Time` as Go's String() format**
  (`"2026-07-06 10:05:00 +0000 UTC"`). SQLite's `strftime` can't parse it.
  Aggregation buckets by `substr(read_at,1,13)`; scans of DATETIME columns
  should Scan into `any` and type-switch. Don't "clean this up" casually —
  every stored row is in that format, and lexicographic range comparisons
  depend on all-UTC uniformity.
- **`go:embed` can't cross package boundaries** — migrations live in
  `internal/storage/migrations/`, webdist needs its `.gitkeep`.
- **The design bundle has a CSS class collision**: `.rng` is both the
  Controls slider (a 200×3 px ink bar) and the detail-card range text.
  Fixed in overrides.css (`.rk2-card div.rng`). Watch for similar collisions
  if you extract more design files.
- **Design portable HTMLs are packed**: the real files are in a script tag as
  JSON `{uuid: {mime, compressed(gzip+base64), data}}`; the main document is
  a JSON-quoted DOCTYPE string in another script tag. Extraction script
  pattern is in the memory log. Extracted copies already live in
  `web/design-src/`.
- **`pkill -f` self-match**: if a watchdog/test argv contains the target
  pattern as an argument, pkill kills the caller. Bit me in the harness.
- **Stale daemon on :8090**: failed command chains can leave an old binary
  serving. If behavior looks stale after a rebuild, `pkill -f propagationd`
  and relaunch before doubting your change.
- **Numato protocol**: echoes every command, `>` prompt terminator, channels
  0-indexed on the wire (driver maps from 1-based), `relay read` returns
  commanded not measured state, all-off at power-on.
- **Missed-schedule policy is "skip"** — implemented by starting schedule
  evaluation from process start time. Don't "fix" catch-up behavior.
- **Moisture rule counting**: max-cycles/day counts only `moisture_threshold`
  cycles over a rolling 24 h; min-interval counts ANY watering (manual too).
  Both intentional.

## 10. API cheat sheet

Base `/api/v1`. All GETs public. Mutations: operator = relay set/all-off,
lighting on/off/override, zone run/stop, sensor poll, diagnostics, e-stop;
admin = everything else (zone PATCH, moisture-control, schedules, settings,
sensor config, safety reset/lockout clear, users). Sessions via
`propagation_session` HttpOnly cookie (30 d).

Key endpoints: `/health` · `/relays/{ch}/set` · `/irrigation/zones` ·
`/irrigation/zones/{key}/run|stop|moisture-control` · `/lighting/status|schedule` ·
`/sensors/{key}/readings?interval=hour` · `/safety/status|emergency-stop|reset` ·
`/schedules` · `/events` · `/events/stream` (SSE) · `/auth/login|logout|session|setup` ·
`/users` · `/species` · `/diagnostics/modbus/scan`.

## 11. Open hardware questions (ask Austin when relevant)

- Which Piconium kit arrived: 002 (N3710/6 GB eMMC) or 003 (E3845/8 GB)?
- SEAFLO pump model/current (if >5 A continuous, interpose an automotive
  relay; Numato contacts ~7 A) — and add flyback diodes across pump + valves.
- Barina light wattage (mains contact check).
- Battery-backed RTC on the board? (Schedules survive without NTP?)
- Sensor datasheets, if the sellers ever provide them (else bench-probe).

## 12. Tone for the finish

The system is genuinely done and tested on the software side; resist
rewriting working subsystems. The remaining work is careful, physical, and
incremental: one sensor at a time, one safety test at a time, with the
simulator as the reference behavior. Austin will be at the bench with you —
he's fast, decisive, and enjoys this. Keep receipts (screenshots, curl
output, test runs) for every claim, keep the memory log updated per session,
and keep water off the floor.

— Fable, 2026-07-06
