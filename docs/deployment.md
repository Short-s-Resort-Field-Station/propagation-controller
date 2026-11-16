# Deployment — Piconium (Propagation Unit 01)

## OS choice

**Debian 13 "trixie", amd64, minimal netinst install — installed to the NVMe drive.**

Why: stable with a long support window, Wi-Fi firmware bundled in the
installer, `cage`/`chromium` in main repos, systemd-native, boring in the good
way. Both Piconium kits (Pentium N3710 / Atom E3845) are plain x86_64.

Install choices:

- Root filesystem on the **NVMe** (the 6–8 GB eMMC is free for anything or
  nothing; NVMe removes all space pressure and takes the write wear instead of
  the soldered eMMC).
- No desktop task. Add: `sudo openssh-server chromium cage seatd curl`
- `systemd-timesyncd` for NTP (unit has internet via Wi-Fi).
- If the board is the **Atom E3845 (Bay Trail)**: add `intel_idle.max_cstate=1`
  to `GRUB_CMDLINE_LINUX_DEFAULT` — Bay Trail has a known c-state erratum that
  causes random hard freezes otherwise.

## Users and permissions

- `propagation` system user runs the daemon; member of `dialout` for the
  Numato (`/dev/ttyACM*`) and RS485 (`/dev/ttyUSB*`) serial devices.
- `kiosk` user runs cage+chromium (needs a seat — install `seatd` or use
  `graphical.target` with logind).

## Serial device stability

Two USB serial devices will be present (Numato relay + RS485 adapter). Do not
use `/dev/ttyACM0`/`/dev/ttyUSB0` in config — enumeration order can change.
Use `/dev/serial/by-id/...` paths; the daemon's autodetection prefers them.
At first bring-up, run `ls -l /dev/serial/by-id/` and pin the relay port via
`-relay-port` (or `PROPAGATION_RELAY_PORT`).

The RS485 adapter (Xiusiyt RS485WE1800BT-1) is expected to be a CH340/CH341;
verify with `lsusb` — the `ch341-uart` driver is in the mainline kernel.

## Touchscreen

Waveshare 5" 800×480: HDMI video + USB HID touch. Works out of the box under
cage (Wayland) — no calibration files or vendor drivers. The touch UI is
designed for exactly 800×480.

## Services

- `propagationd.service` — the controller (listens on :80 via
  `CAP_NET_BIND_SERVICE`; switch `PROPAGATION_LISTEN` to `:8080` if you'd
  rather not bind 80).
- `propagation-kiosk.service` — cage + chromium at `http://localhost/ui/touch`.
- `propagation-watchdog.service` — placeholder until Phase 8. Design note:
  the watchdog must never open the relay serial port while propagationd owns
  it; its force-safe-state path is (1) stop propagationd, (2) open port,
  (3) `relay writeall 00`.

## SQLite on flash

The daemon opens the DB with WAL + `synchronous=NORMAL` + busy timeout — crash
safe with modest write amplification. Database lives in
`/var/lib/propagation-controller` (NVMe). Keep journald size-capped
(`SystemMaxUse=200M` in `/etc/systemd/journald.conf`) since the daemon logs
JSON to stderr.

## Install

```sh
make linux
scp bin/propagationd-linux-amd64 bin/propagation-watchdog-linux-amd64 piconium:
scp -rp deploy piconium:
ssh -t piconium 'chmod +x deploy/install.sh && cd deploy && sudo ./install.sh ../propagationd-linux-amd64 ../propagation-watchdog-linux-amd64'
```

## Phone notifications (Web Push)

The dashboard is an installable PWA and `propagationd` pushes to it directly
over Web Push: no third-party app, no account, nothing to configure on the
unit. On first start the daemon writes a VAPID key pair to
`<data-dir>/vapid.json` (0600); each browser that opts in is a row in
`push_subscriptions` with its own switches.

To enable on a phone: sign in at shortsfieldstation.org/propagate, open
**Alerts**, switch on **Notifications on this device**, allow the browser
prompt, then pick the categories: Watering, Shelf below its band, Reservoir,
Sensors & faults (off by default). **Send test** proves the path. On an
iPhone the dashboard must be added to the Home Screen first and opened from
there. Android pushes ride Google's push service, so a phone without Play
Services receives nothing.

Pushes need the unit's outbound HTTPS, the same path the tunnel uses.
`startup_complete` logs `"notifications": "web_push"`.

## Tunnel-only firewall

`deploy/firewall/nftables.conf` accepts SSH from the LAN and nothing else
inbound; the dashboard is reachable only through the Cloudflare tunnel and
on the unit itself. To (re)apply:

```sh
sudo install -m 644 ~/deploy/firewall/nftables.conf /etc/nftables.conf
sudo nft -f /etc/nftables.conf
sudo systemctl enable nftables
```

To open the dashboard to the LAN again temporarily, add `80` back to the
`tcp dport` set and re-run `nft -f`.

## Auto-install from GitHub

Every push to `main` runs `.github/workflows/release.yml`: web tests, `make
web`, Go tests, `make linux` with the build stamp, then a GitHub release
tagged `build-<sha>` carrying the two binaries, `deploy.tar.gz` and
`SHA256SUMS`. A red test suite publishes nothing.

On the unit, `propagation-update.timer` (root, 5 min after boot, then every 10
min) runs `/opt/propagation-controller/propagation-update.py`: it compares the
latest release tag with the running build from `/health`, waits until no
shelf is running or queued, downloads and verifies the assets, keeps the old
binaries as `*.prev`, runs `install.sh`, and rolls back if the new build is
not healthy within 90 s. The kiosk reloads itself on the new build stamp.

```sh
sudo python3 /opt/propagation-controller/propagation-update.py --check   # what it would do
journalctl -u propagation-update -n 30                                  # recent runs
sudo systemctl disable --now propagation-update.timer                   # opt out
```

A manual `scp` + `install.sh` still works and the updater will leave a
matching build alone.

