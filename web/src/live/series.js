// Environmental-tab series built from the readings API (Phase 7; timeline
// rebuild 2026-11-11). Every metric is a set of time-stamped points per shelf
// — { t, v, lo, hi } — so the charts draw a true date axis: 5-minute buckets
// of raw readings for the 24-hour view, hourly aggregates beyond that.

import { api } from './api.js';
import { cToF, siblingKey } from './adapter.js';

export const WINDOWS = {
  '24 Hours': { hours: 24, interval: '5m' },
  '7 Days': { hours: 168, interval: 'hour' },
  '30 Days': { hours: 720, interval: 'hour' },
  'Season': { hours: 2160, interval: 'hour' },
};
const COLORS = ['var(--river)', 'var(--ochre)', 'var(--sage)', 'var(--oxblood)'];

const toPoints = (aggs, fn) => (aggs || [])
  .filter((a) => a.avg_value != null)
  .map((a) => ({
    t: new Date(a.hour_start).getTime(),
    v: fn(a.avg_value),
    lo: fn(a.min_value != null ? a.min_value : a.avg_value),
    hi: fn(a.max_value != null ? a.max_value : a.avg_value),
  }));

// Y range that hugs the data (band included) with a margin, snapped to `step`.
export function yBounds(series, defLo, defHi, step = 1, floor = null) {
  // Include each shelf's target band so the ideal range is always on screen.
  const los = series.flatMap((s) => s.points.map((p) => p.lo).concat(s.target ? [s.target[0]] : []));
  const his = series.flatMap((s) => s.points.map((p) => p.hi).concat(s.target ? [s.target[1]] : []));
  if (!los.length) return [defLo, defHi];
  const lo = Math.min(...los), hi = Math.max(...his);
  const margin = Math.max(step * 2, (hi - lo) * 0.2);
  let a = Math.floor((lo - margin) / step) * step;
  let b = Math.ceil((hi + margin) / step) * step;
  if (floor != null) a = Math.max(floor, a);
  if (b - a < step * 4) b = a + step * 4;
  return [a, b];
}

// One metric across the active shelves. Audit 3.8: every series is keyed off
// the shelf's own probe key (soil_sensor_key), never the shelf number.
// `target(shelf)` returns the shelf's ideal [lo, hi] for this metric (or null)
// — drawn as a band through the chart (Austin, 2026-11-11).
async function metric(active, channel, fn, from, interval, colorOffset, target = () => null) {
  const series = [];
  const byShelf = {};
  for (const s of active) {
    try {
      const key = channel ? siblingKey(s.sensorKey, channel) : s.sensorKey;
      const r = await api.series(key, from, interval);
      const points = toPoints(r.aggregates, fn);
      if (points.length) {
        byShelf[s.n] = points.map((p) => p.v);
        series.push({ n: s.n, color: COLORS[(series.length + colorOffset) % COLORS.length], label: `Shelf ${s.n}`, points, target: target(s) });
      }
    } catch { /* no probe / no data for this shelf */ }
  }
  return { series, byShelf };
}

export async function getEnvSeries(period, shelves) {
  const win = WINDOWS[period] || WINDOWS['7 Days'];
  const toMs = Date.now();
  const fromMs = toMs - win.hours * 3600e3;
  const from = new Date(fromMs).toISOString();
  const active = shelves.filter((x) => x.active && x.sensorKey);

  const band = (b) => (Array.isArray(b) && b.length === 2 && b[0] != null && b[1] != null && b[1] > b[0] ? [b[0], b[1]] : null);
  // Moisture's ideal = the shelf's rule: water below `lo`, re-armed above `hi`.
  const moist = await metric(active, null, (v) => Math.round(v * 10) / 10, from, win.interval, 0,
    (s) => (s.hasRule === false ? null : band([s.lo, s.hi])));
  // Root-zone temperature per shelf (each probe has its own channel since the
  // 2026-11-09 build); adherence scores each shelf against its own trace.
  const temp = await metric(active, 'temperature', cToF, from, win.interval, 3, (s) => band(s.tempBand));
  // Substrate EC (µS/cm) — the ZTS-3000 probes measure conductivity directly.
  const ec = await metric(active, 'ec', (v) => Math.round(v), from, win.interval, 2, (s) => band(s.ecBand));

  // change 3: time-in-target — share of buckets inside each shelf's
  // happy-band over the selected period (monitoring only).
  const inBand = (arr, band) => {
    if (!band || !arr || !arr.length) return null;
    const [b0, b1] = band;
    return { pct: Math.round((arr.filter((v) => v >= b0 && v <= b1).length / arr.length) * 100), n: arr.length };
  };
  const adherence = shelves.filter((x) => x.active).map((s) => ({
    n: s.n,
    ecBand: s.ecBand || null, ec: inBand(ec.byShelf[s.n], s.ecBand),
    tempBand: s.tempBand || null, temp: inBand(temp.byShelf[s.n], s.tempBand),
  })).filter((a) => a.ec || a.temp);

  // Watering events: minutes of actual pump time per bucket across the range,
  // plus every pump run per zone for the timeline markers (Austin,
  // 2026-11-11: mark when the pump ran). Drains and purges never run the
  // pump (actual 0) and are left off the charts.
  let water = new Array(14).fill(0);
  const runsByZone = {};
  try {
    const r = await api.cycles(2000);
    for (const c of r.cycles || []) {
      const t = new Date(c.started_at).getTime();
      // A watering still in progress has no actual duration yet: count it
      // from its start so the drop shows up as soon as the pump starts.
      const secs = c.actual_duration_seconds != null ? c.actual_duration_seconds
        : (c.result === 'running' && c.trigger_source !== 'drain' && c.trigger_source !== 'purge' ? Math.max(1, (Date.now() - t) / 1000) : 0);
      if (t < fromMs || c.result === 'locked_out' || secs <= 0) continue;
      const idx = Math.min(13, Math.floor((t - fromMs) / (win.hours * 3600e3 / 14)));
      water[idx] += secs / 60;
      (runsByZone[c.zone_key] = runsByZone[c.zone_key] || []).push({
        t0: t, t1: t + secs * 1000, mins: Math.round(secs / 6) / 10,
        source: c.trigger_source || c.trigger_type || '', result: c.result || '',
      });
    }
    water = water.map((v) => Math.round(v * 10) / 10);
  } catch { /* cycles unavailable */ }

  // One section per shelf (Austin, 2026-11-11: keep the data readable) —
  // each shelf's three traces with Y bounds that hug that shelf alone.
  const perShelf = active.map((s) => {
    const pick = (m) => m.series.filter((x) => x.n === s.n);
    const mo = pick(moist), te = pick(temp), e = pick(ec);
    return {
      n: s.n,
      runs: (runsByZone[s.zoneKey] || []).sort((a, b) => a.t0 - b.t0),
      moist: { series: mo, y: yBounds(mo, 0, 60, 5, 0) },
      temp: { series: te, y: yBounds(te, 60, 90, 2) },
      ec: { series: e, y: yBounds(e, 0, 1000, 50, 0) },
    };
  }).filter((p) => p.moist.series.length || p.temp.series.length || p.ec.series.length);

  return {
    range: { fromMs, toMs },
    perShelf,
    moist: moist.series, temp: temp.series, ec: ec.series,
    moistY: yBounds(moist.series, 0, 60, 5, 0),
    tempY: yBounds(temp.series, 60, 90, 2),
    ecY: yBounds(ec.series, 0, 1000, 50, 0),
    water, adherence,
  };
}
