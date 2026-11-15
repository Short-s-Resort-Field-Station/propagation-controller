#!/bin/sh
# Propagation Controller installer (run as root on the Piconium).
set -eu

BIN_SRC="${1:-./propagationd-linux-amd64}"

echo "== Propagation Controller install =="

id -u propagation >/dev/null 2>&1 || useradd --system --home /var/lib/propagation-controller --shell /usr/sbin/nologin propagation
usermod -aG dialout propagation

mkdir -p /opt/propagation-controller /var/lib/propagation-controller /etc/propagation-controller
install -m 755 "$BIN_SRC" /opt/propagation-controller/propagationd
WATCHDOG_SRC="${2:-./propagation-watchdog-linux-amd64}"
[ -f "$WATCHDOG_SRC" ] && install -m 755 "$WATCHDOG_SRC" /opt/propagation-controller/propagation-watchdog
chown -R propagation:propagation /var/lib/propagation-controller

install -m 644 systemd/propagationd.service /etc/systemd/system/
[ -f /opt/propagation-controller/propagation-watchdog ] && install -m 644 systemd/propagation-watchdog.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable propagationd
systemctl restart propagationd   # restart, not enable --now: an upgrade must swap the running binary
[ -f /etc/systemd/system/propagation-watchdog.service ] && systemctl enable --now propagation-watchdog

echo "== Done. Health: curl http://localhost/api/v1/health =="
