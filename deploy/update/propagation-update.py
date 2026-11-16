#!/usr/bin/env python3
"""Self-updater for Propagation Unit 01 (2026-11-15).

Runs from a root systemd timer every ten minutes. Looks at the newest GitHub
release (built and tested by .github/workflows/release.yml on every push to
main), compares its tag to the build stamp the running daemon reports, and if
they differ: waits for an idle rack, downloads the assets, verifies SHA256SUMS,
keeps the previous binaries as *.prev, runs install.sh, and checks health
afterwards. Anything unexpected logs and exits; the next tick tries again.

Standard library only: the unit has no curl and no git.
"""
import hashlib, io, json, os, re, shutil, subprocess, sys, tarfile, tempfile, time, urllib.request

REPO = "Short-s-Resort-Field-Station/propagation-controller"
API = f"https://api.github.com/repos/{REPO}/releases/latest"
LOCAL = "http://localhost/api/v1"
OPT = "/opt/propagation-controller"
ASSETS = ("propagationd-linux-amd64", "propagation-watchdog-linux-amd64", "deploy.tar.gz", "SHA256SUMS")
TAG_RE = re.compile(r"^build-([0-9a-f]{7,40})$")
LOCK = "/run/propagation-update.lock"


def log(msg):
    print(msg, flush=True)


def fetch(url, timeout=60):
    req = urllib.request.Request(url, headers={"User-Agent": "propagation-update/1", "Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.read()


def running_build():
    try:
        h = json.loads(fetch(f"{LOCAL}/health", timeout=10))
        return h.get("build") or "", h.get("status")
    except Exception as e:  # daemon down: still allowed to update (install.sh restarts it)
        log(f"local health unavailable: {e}")
        return "", None


def rack_idle():
    try:
        z = json.loads(fetch(f"{LOCAL}/irrigation/zones", timeout=10))
    except Exception as e:
        log(f"cannot read zones ({e}); treating as idle")
        return True
    busy = [x["zone_key"] for x in z.get("zones", []) if x.get("state") in ("running", "queued")]
    if busy:
        log(f"rack busy ({', '.join(busy)}); deferring")
    return not busy


def main():
    check_only = "--check" in sys.argv
    if os.path.exists(LOCK) and time.time() - os.path.getmtime(LOCK) < 1800:
        log("another update is in progress; exiting")
        return 0
    rel = json.loads(fetch(API))
    tag = rel.get("tag_name", "")
    m = TAG_RE.match(tag)
    if not m:
        log(f"latest release tag {tag!r} is not a build tag; nothing to do")
        return 0
    want = m.group(1)
    have, status = running_build()
    have_sha = have.split("-")[0]
    log(f"release {tag}; running build {have or '(unknown)'} (health {status})")
    if have_sha and (have_sha.startswith(want) or want.startswith(have_sha)):
        log("already on this build")
        return 0
    if check_only:
        log("would update")
        return 0
    if not rack_idle():
        return 0
    assets = {a["name"]: a["browser_download_url"] for a in rel.get("assets", [])}
    missing = [n for n in ASSETS if n not in assets]
    if missing:
        log(f"release is missing assets {missing}; skipping")
        return 0
    open(LOCK, "w").write(str(os.getpid()))
    try:
        with tempfile.TemporaryDirectory(prefix="propagation-update-") as tmp:
            for name in ASSETS:
                data = fetch(assets[name], timeout=300)
                open(os.path.join(tmp, name), "wb").write(data)
            sums = {}
            for line in open(os.path.join(tmp, "SHA256SUMS")):
                parts = line.split()
                if len(parts) == 2:
                    sums[parts[1].lstrip("*")] = parts[0]
            for name in ASSETS[:-1]:
                digest = hashlib.sha256(open(os.path.join(tmp, name), "rb").read()).hexdigest()
                if sums.get(name) != digest:
                    log(f"checksum mismatch for {name}; aborting")
                    return 1
            with tarfile.open(os.path.join(tmp, "deploy.tar.gz")) as tf:
                tf.extractall(tmp, filter="data")
            deploy = os.path.join(tmp, "deploy")
            for name in ("propagationd", "propagation-watchdog"):
                cur = os.path.join(OPT, name)
                if os.path.exists(cur):
                    shutil.copy2(cur, cur + ".prev")
            os.chmod(os.path.join(deploy, "install.sh"), 0o755)
            log(f"installing {tag}")
            r = subprocess.run(["sh", "install.sh", os.path.join(tmp, "propagationd-linux-amd64"),
                                os.path.join(tmp, "propagation-watchdog-linux-amd64")], cwd=deploy,
                               capture_output=True, text=True)
            log(r.stdout.strip())
            if r.returncode != 0:
                log(f"install.sh failed ({r.returncode}): {r.stderr.strip()}")
                return 1
        # Post-install: the new build must answer healthy within 90 s, else roll back.
        for _ in range(18):
            time.sleep(5)
            build, status = running_build()
            if build.split("-")[0] and (build.startswith(want) or want.startswith(build.split("-")[0])) and status == "ok":
                log(f"update complete: running {build}")
                return 0
        log("new build did not come up healthy; rolling back")
        for name in ("propagationd", "propagation-watchdog"):
            prev = os.path.join(OPT, name) + ".prev"
            if os.path.exists(prev):
                shutil.copy2(prev, os.path.join(OPT, name))
        subprocess.run(["systemctl", "restart", "propagationd"])
        return 1
    finally:
        try:
            os.remove(LOCK)
        except OSError:
            pass


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as e:
        log(f"update check failed: {e}")
        sys.exit(0)  # a transient network error must not mark the timer unit failed
