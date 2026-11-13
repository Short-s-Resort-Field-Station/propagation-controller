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

The installer seeds `/etc/propagation-controller/notify.env` from
`deploy/notify.env.example` the first time only and never overwrites it.

## Phone notifications (reservoir lockouts)

`propagationd` pushes "Reservoir empty" / "Reservoir sensor offline" and the
matching all-clear to the transports named in `notify.env`; nothing else is
sent. Both are optional and independent.

- **ntfy push** (no account): install the ntfy app, then on the Piconium

  ```sh
  sudo sh -c 'sed -i "s/^PROPAGATION_NTFY_TOPIC=.*/PROPAGATION_NTFY_TOPIC=pu01-$(openssl rand -hex 8)/" /etc/propagation-controller/notify.env'
  sudo grep NTFY_TOPIC /etc/propagation-controller/notify.env   # subscribe to this topic in the app
  sudo systemctl restart propagationd
  ```

  Treat the topic like a password: anyone who knows it can read the pushes.
- **Twilio SMS**: fill the four `PROPAGATION_TWILIO_*` lines with your own
  account's SID, auth token, sending number and your mobile (E.164, `+1...`),
  then restart. US long codes need A2P 10DLC registration before carriers
  deliver; a trial account texts only its verified numbers.

To test either one without emptying the tank, raise the cutoff above the
current level and put it back:

```sh
curl -s localhost/api/v1/settings | jq .settings.safety      # note the current block
# PUT the same block with "min_reservoir_level": 90, wait 5 s, PUT it back to 20
```

Startup logs the enabled transports as `"notifications": [...]` in the
`startup_complete` event (never the credentials).
