// Environmental-tab series built from the hourly aggregates API (Phase 7).

import { api } from './api.js';
import { cToF, siblingKey } from './adapter.js';

const HOURS = { '24 Hours': 24, '7 Days': 168, '30 Days': 720, 'Season': 2160 };
const COLORS = ['var(--river)', 'var(--ochre)', 'var(--sage)', 'var(--oxblood)'];

// LineChart needs >= 2 points to draw a path.
const pad = (data) => (data.length === 1 ? [data[0], data[0]] : data);

function bounds(datasets, defLo, defHi) {
  const all = datasets.flatMap((d) => d.data);
  if (!all.length) return [defLo, defHi];
  const lo = Math.min(...all), hi = Math.max(...all);
  const margin = Math.max(2, (hi - lo) * 0.25);
  return [Math.floor(lo - margin), Math.ceil(hi + margin)];
}

export async function getEnvSeries(period, shelves) {
  const hours = HOURS[period] || 168;
  const fromMs = Date.now() - hours * 3600e3;
  const from = new Date(fromMs).toISOString();

  const moist = [];
  const legend = [];
  // Audit 3.8: every series is keyed off the shelf's own probe key
  // (soil_sensor_key), never the array index / shelf number.
  const active = shelves.filter((x) => x.active && x.sensorKey);
  for (const s of active) {
    try {
      const r = await api.aggregates(s.sensorKey, from);
      const data = (r.aggregates || []).map((a) => Math.round(a.avg_value * 10) / 10);
      if (data.length) {
        const color = COLORS[moist.length % COLORS.length];
        moist.push({ color, fill: moist.length === 0, data: pad(data) });
        legend.push({ color, label: `Shelf ${s.n}` });
      }
    } catch { /* no probe / no data for this shelf */ }
  }

  // Root-zone temperature per shelf (each probe has its own channel since the
  // 2026-11-09 build); adherence scores each shelf against its own trace.
  const temp = [];
  const tempLegend = [];
  const tempByShelf = {}; // change 3: raw °F buckets per shelf, reused for adherence
  for (const s of active) {
    try {
      const r = await api.aggregates(siblingKey(s.sensorKey, 'temperature'), from);
      const data = (r.aggregates || []).map((a) => cToF(a.avg_value));
      if (data.length) {
        tempByShelf[s.n] = data;
        const color = COLORS[(temp.length + 3) % COLORS.length];
        temp.push({ color, fill: temp.length === 0, data: pad(data) });
        tempLegend.push({ color, label: `Shelf ${s.n}` });
      }
    } catch { /* no temperature data for this shelf */ }
  }

  // Substrate EC per shelf (µS/cm) — the ZTS-3000 probes measure conductivity
  // directly (register 2); there is no pH channel on this hardware.
  const ec = [];
  const ecLegend = [];
  const ecByShelf = {}; // change 3: raw EC buckets per shelf, reused for adherence
  for (const s of active) {
    try {
      const r = await api.aggregates(siblingKey(s.sensorKey, 'ec'), from);
      const data = (r.aggregates || []).map((a) => Math.round(a.avg_value));
      if (data.length) {
        ecByShelf[s.n] = data;
        const color = COLORS[(ec.length + 2) % COLORS.length];
        ec.push({ color, fill: ec.length === 0, data: pad(data) });
        ecLegend.push({ color, label: `Shelf ${s.n}` });
      }
    } catch { /* no EC data for this shelf */ }
  }

  // change 3: time-in-target — share of hourly buckets inside each shelf's
  // happy-band, over the selected period (monitoring only). Each shelf is
  // scored against its own EC and temperature traces.
  const inBand = (arr, band) => {
    if (!band || !arr || !arr.length) return null;
    const [b0, b1] = band;
    return { pct: Math.round((arr.filter((v) => v >= b0 && v <= b1).length / arr.length) * 100), n: arr.length };
  };
  const adherence = shelves.filter((x) => x.active).map((s) => ({
    n: s.n,
    ecBand: s.ecBand || null, ec: inBand(ecByShelf[s.n], s.ecBand),
    tempBand: s.tempBand || null, temp: inBand(tempByShelf[s.n], s.tempBand),
  })).filter((a) => a.ec || a.temp);

  // Watering events: minutes of actual watering per bucket across the range.
  let water = new Array(14).fill(0);
  try {
    const r = await api.cycles(500);
    for (const c of r.cycles || []) {
      const t = new Date(c.started_at).getTime();
      if (t < fromMs || c.result === 'locked_out') continue;
      const idx = Math.min(13, Math.floor((t - fromMs) / (hours * 3600e3 / 14)));
      water[idx] += (c.actual_duration_seconds || 0) / 60;
    }
    water = water.map((v) => Math.round(v * 10) / 10);
  } catch { /* cycles unavailable */ }

  return {
    moist, temp, tempLegend, water, legend, ec, ecLegend, adherence,
    moistY: bounds(moist, 20, 80),
    tempY: bounds(temp, 68, 82),
    ecY: ecBounds(ec),
  };
}

// EC wants bounds that hug the data (propagation substrate sits in the
// hundreds of µS/cm; a 0–3000 window would flatten the trace).
function ecBounds(datasets) {
  const all = datasets.flatMap((d) => d.data);
  if (!all.length) return [0, 3000];
  const lo = Math.min(...all), hi = Math.max(...all);
  const margin = Math.max(100, (hi - lo) * 0.5);
  return [Math.max(0, Math.round((lo - margin) / 50) * 50), Math.round((hi + margin) / 50) * 50];
}
