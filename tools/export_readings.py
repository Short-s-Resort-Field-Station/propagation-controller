#!/usr/bin/env python3
"""Pull the rack's readings and watering cycles to CSV for the two-week review.

Usage:  python3 tools/export_readings.py [out_dir] [--base URL] [--days N]
Default base is the public page (GETs are unauthenticated); --base
http://localhost/propagate/api/v1 works on the Piconium itself. Writes one CSV
of hourly aggregates per sensor key, one of raw readings, and one of cycles.
"""
import csv, json, sys, urllib.request, urllib.parse, datetime, os

BASE = "https://shortsfieldstation.org/propagate/api/v1"
KEYS = [f"soil_zone_{n}_{m}" for n in (1, 2, 3) for m in ("moisture", "temperature", "ec")] + ["reservoir_level_level_cm"]

def get(base, path):
    with urllib.request.urlopen(base + path, timeout=30) as r:
        return json.load(r)

def main():
    args = sys.argv[1:]
    base, days, out = BASE, 14, None
    while args:
        a = args.pop(0)
        if a == "--base": base = args.pop(0)
        elif a == "--days": days = int(args.pop(0))
        else: out = a
    out = out or f"readings-export-{datetime.date.today()}"
    os.makedirs(out, exist_ok=True)
    frm = urllib.parse.quote((datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ"))
    with open(os.path.join(out, "hourly.csv"), "w", newline="") as fh, open(os.path.join(out, "raw.csv"), "w", newline="") as fr:
        wh = csv.writer(fh); wr = csv.writer(fr)
        wh.writerow(["sensor_key", "bucket", "min", "avg", "max", "count"]); wr.writerow(["sensor_key", "read_at", "value", "quality"])
        for k in KEYS:
            for a in get(base, f"/sensors/{k}/readings?interval=hour&from={frm}&limit=720").get("aggregates", []):
                wh.writerow([k, a.get("bucket") or a.get("bucket_start"), a.get("min"), a.get("avg"), a.get("max"), a.get("count")])
            for r in get(base, f"/sensors/{k}/readings?from={frm}&limit=100000").get("readings", []):
                wr.writerow([k, r.get("read_at"), r.get("value"), r.get("quality")])
    cyc = get(base, "/irrigation/cycles?limit=1000").get("cycles", [])
    with open(os.path.join(out, "cycles.csv"), "w", newline="") as fc:
        cols = ["zone_key", "started_at", "ended_at", "trigger_type", "requested_duration_seconds", "actual_duration_seconds",
                "soil_moisture_start", "soil_moisture_end", "reservoir_level_start", "reservoir_level_end", "result", "abort_reason"]
        w = csv.writer(fc); w.writerow(cols)
        for c in cyc: w.writerow([c.get(x) for x in cols])
    print(f"wrote {out}/hourly.csv raw.csv cycles.csv ({len(cyc)} cycles)")

if __name__ == "__main__":
    main()
