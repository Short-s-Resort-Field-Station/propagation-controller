# BRINGUP — hardware install playbook (for the on-device session)

**Audience:** the Claude session running *on the Piconium itself* (or over SSH
to it) doing first hardware bring-up, with Austin at the bench. Written
2026-07-11 by the session that reverse-engineered the soil probe and shipped
the EC metric. Read `docs/HANDOFF.md` for project context, `docs/SENSORS.md`
for verified probe facts, `docs/deployment.md` for the OS steps this document
sequences.

## Mission

Take the repo from "verified in simulation" to "running the physical rack":
propagationd + watchdog as services, real relay + Modbus drivers, kiosk
touchscreen, and every safety layer demonstrated against real hardware.

## Ground rules (Austin's working style — non-negotiable)

- **Verify every claim live.** Exercise it over HTTP, read the actual logs,
  screenshot the UI. Never report a step done without evidence.
- **Raise a concern once, then defer** if Austin reaffirms.
- **Ask hardware questions before assuming** — he answers fast and precisely.
- Safety engineering is the product. If a safety layer misbehaves, stop and
  fix it before moving on; never bypass one to make progress.

## Hardware facts you'd otherwise rediscover the hard way

1. **Soil probe talks 4800 baud, 8N1, slave address 1.** The daemon default
   (`-modbus-baud`) is already 4800. If the probe seems absent, suspect
   wiring before software: probe yellow → adapter A/Data+, probe blue →
   B/Data−, and the probe MUST have V+ (brown) powered — an unpowered or
   A/B-swapped probe is perfectly silent. Full map + bench behaviour:
   `docs/SENSORS.md`.
2. **The probe has no pH channel.** The product intentionally shows
   Substrate EC (µS/cm). Don't "fix" anything back to pH.
3. **EC readings of exactly 20000 are clipped** (sensor ceiling) and arrive
   with quality `clipped` — that value means "off-scale", not a measurement.
4. **Additional probes (shelves 02–04) all ship at address 1.** Readdress
   each alone on the bus with `bin/probe-address -to <N> -yes` BEFORE
   connecting it alongside the others (`docs/SENSORS.md` §multi-probe).
5. **Numato relay** enumerates as CDC ACM at `/dev/serial/by-id/*Numato*`;
   the FTDI RS485 adapter autodetects via `/dev/serial/by-id/*FTDI*`. Pin
   both explicitly in the unit file env once known — by-id paths are stable,
   ttyUSB numbering is not.
6. **Kit 003 (Atom E3845 / Bay Trail) needs `intel_idle.max_cstate=1`** on
   the kernel cmdline or the board freezes under load. Kit 002 (N3710)
   needs nothing.
7. The service binds **:80 via CAP_NET_BIND_SERVICE** (no root, no reverse
   proxy). Kiosk is cage + chromium at `http://localhost/ui/touch`.

## What the 2026-07-12 OS install already established (don't re-learn these)

The base OS is INSTALLED and these facts are verified on the physical machine:

- **It's Kit 002** — Pentium N3710 (Braswell), 4 GB RAM, 500 GB WD Blue SATA
  M.2 (`/dev/sda`), 14.3 GB eMMC. The Bay Trail `intel_idle.max_cstate=1`
  workaround is NOT needed on this CPU.
- Debian 13, **BIOS/legacy boot**, GRUB on `/dev/sda`, root `/dev/sda1` +
  swap `sda5`. Hostname `srfsprop1`, static-ish at 192.168.1.250 (DHCP),
  user `anelsen` (sudo), SSH key auth from Austin's Mac (`ssh piconium`).
- **The debian-installer defaults to the eMMC at partitioning.** If the OS
  is ever reinstalled: explicitly select the SATA SSD by size/model, and
  send GRUB to `/dev/sda`. (Round 1 landed on the eMMC this way.)
- The **eMMC was wiped clean 2026-07-12** (old install + its GRUB). Nothing
  boots from it; leave it empty or use it as scratch.
- **Phantom eDP:** the board exposes `eDP-1` that falsely reports a
  connected panel, which steals the console/KMS output and blanks the real
  HDMI display. Fixed with `video=eDP-1:d` in GRUB_CMDLINE_LINUX_DEFAULT —
  do not remove it, the kiosk depends on output landing on HDMI.
- **Rescue-mode chroot has no DNS**: `echo "nameserver 1.1.1.1" >
  /etc/resolv.conf` inside the chroot before any apt operation.
- The graphical installer black-screens on this GPU; always use the text
  installer.

## Kiosk (installed 2026-07-12 overnight — ONE open item: display output)

The touchscreen kiosk is **built, deployed, enabled, and reboot-verified** at
the software level:

- Accidental GNOME desktop purged; boot target is `multi-user.target` (no
  display manager). Network stack (NetworkManager) and SSH deliberately
  `apt-mark manual`'d first so the purge couldn't strand remote access.
- `cage seatd chromium grim` installed. `kiosk` user (groups video/input/
  render). Launch script + service in the repo at `deploy/kiosk/`
  (`propagation-kiosk.service`, `kiosk-launch.sh` — chromium `--kiosk
  --ozone-platform=wayland --app=http://localhost/ui/touch`).
- After a clean reboot, all four services (propagationd, watchdog, seatd,
  kiosk) come up automatically and chromium loads `/ui/touch` — confirmed via
  its remote-debug endpoint. The web app itself is fine.

**OPEN ITEM — on-screen output not yet visually confirmed.** Both `grim` and
chromium's own `Page.captureScreenshot` block with no frame, and cage's debug
run gives the root cause:

```
[ERROR] backend/drm/util.c: Failed to parse EDID
[ERROR] connector HDMI-A-2: drmModeMoveCursor failed: Invalid argument
```

The **bench monitor's EDID does not parse**, so wlroots/cage can't negotiate a
video mode and never presents a frame — the screen stays dark even though
chromium is rendering. This is the likely root cause of the *entire* night's
display saga (installer black-screens included). It is a display-negotiation
problem, not a software-config one.

**Next session, with eyes on the panel:** connect the **actual Waveshare 5″
800×480 touchscreen** (the real target — its EDID may parse cleanly where the
bench monitor's doesn't) and look. If it's still dark, the fix is to supply a
forced EDID to the kernel: build a 800×480 (or 1024×768) EDID blob and boot
with `drm.edid_firmware=HDMI-A-1:edid/<blob>.bin` plus the existing
`video=HDMI-A-1:...e` force-enable. Do NOT chase this further on the bench
monitor — verify against the panel that ships with the unit. Console text
(fbcon) DOES paint at 1024×768, so the hardware path works; only the
EDID-dependent Wayland modeset is failing.

## Sequence

Follow `docs/deployment.md` for each step's detail; this is the order and
the verification gate for each.

1. **OS + users + udev** (deployment.md §§1–3). Gate: both serial by-id
   paths visible, `propagation` user in `dialout`.
2. **Binaries**: `make linux` on the build machine → scp `propagationd`,
   `propagation-watchdog`, `probe-address` + `deploy/` → `install.sh`.
   Gate: `systemctl status` green for both services in **simulation** first.
3. **Probe smoke test, before real mode**: with the daemon stopped, run the
   readdressing tool as a dry run — `bin/probe-address -to 2` (no `-yes`)
   reports the probe it found (moisture, temp, EC) without writing
   anything. That one line proves port, baud, wiring, and address in
   isolation from the daemon.
4. **Real mode**: drop `-simulation`. Gate: `/api/v1/sensors` shows
   `soil_zone_1_*` and `reservoir_level_level` with quality `good`, values
   plausible (probe in air: moisture ≈ 0, EC 0, temp ≈ room).
5. **Relay bring-up with the pump DRY-DISCONNECTED** (deployment.md order):
   exercise each channel from the Controls screen, listen for the clicks,
   then wire loads one at a time.
6. **Calibration** (deployment.md §sensors): reservoir level against a
   ruler; soil probe against pots of the actual substrate. Update device
   config JSON — no rebuild needed.
7. **Safety demonstration** (the finish line, from HANDOFF §8): pull the
   reservoir sensor mid-cycle, yank the probe, kill propagationd and watch
   the watchdog seize the relay port and force safe state. Each must be
   observed on real hardware, not assumed from the sim.
8. **Kiosk + remote access** (deployment.md §touchscreen,
   `docs/remote-access.md` for the Cloudflare tunnel).

## Known state of the codebase at handoff

- All 12 Go test packages green; both UIs screenshot-verified 2026-07-11.
- Austin's dev database (not shipped) was migrated pH→EC; a fresh install
  seeds EC correctly from scratch — no migration concerns on the device.
- `web/` needs Node only at build time; the Piconium gets static binaries.
- Watchdog restart-ladder and port-seize were tested in simulation with a
  real port seize on the bench Mac; the on-device demonstration (step 7) is
  the remaining proof.

## If the probe misbehaves on the rack

Bench-verified diagnostics, fastest first: (a) `probe-address` dry run —
proves the whole serial path in one command; (b) swap A/B at the terminal
block; (c) check V+ — these probes brown-out silently below ~4.5 V; (d) probe
answers but readings frozen? poll interval vs. another master — never run two
readers on one bus. The raw register dump procedure and full map are in
`docs/SENSORS.md` if genuinely stuck.
