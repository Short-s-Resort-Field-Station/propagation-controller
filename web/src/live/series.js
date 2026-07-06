// Environmental-tab series built from the hourly aggregates API (Phase 7).

import { api } from './api.js';
import { cToF } from './adapter.js';

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
  for (const s of shelves.filter((x) => x.active)) {
    const key = `soil_zone_${Number(s.n)}_moisture`;
    try {
      const r = await api.aggregates(key, from);
      const data = (r.aggregates || []).map((a) => Math.round(a.avg_value * 10) / 10);
      if (data.length) {
        const color = COLORS[moist.length % COLORS.length];
        moist.push({ color, fill: moist.length === 0, data: pad(data) });
        legend.push({ color, label: `Shelf ${s.n}` });
      }
    } catch { /* no probe / no data for this shelf */ }
  }

  let temp = [];
  try {
    const r = await api.aggregates('soil_zone_1_temperature', from);
    const data = (r.aggregates || []).map((a) => cToF(a.avg_value));
    if (data.length) temp = [{ color: 'var(--oxblood)', fill: true, data: pad(data) }];
  } catch { /* no data yet */ }

  // pH per shelf (the soil probes measure it — spec listed pH as future,
  // the hardware gives it to us now).
  const ph = [];
  const phLegend = [];
  for (const s of shelves.filter((x) => x.active)) {
    const key = `soil_zone_${Number(s.n)}_ph`;
    try {
      const r = await api.aggregates(key, from);
      const data = (r.aggregates || []).map((a) => Math.round(a.avg_value * 100) / 100);
      if (data.length) {
        const color = COLORS[(ph.length + 2) % COLORS.length];
        ph.push({ color, fill: ph.length === 0, data: pad(data) });
        phLegend.push({ color, label: `Shelf ${s.n}` });
      }
    } catch { /* no pH data for this shelf */ }
  }

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
    moist, temp, water, legend, ph, phLegend,
    moistY: bounds(moist, 20, 80),
    tempY: bounds(temp, 68, 82),
    phY: phBounds(ph),
  };
}

// pH wants tighter, sensible bounds (a whole-number window looks wrong).
function phBounds(datasets) {
  const all = datasets.flatMap((d) => d.data);
  if (!all.length) return [5.5, 7.5];
  const lo = Math.min(...all), hi = Math.max(...all);
  const margin = Math.max(0.3, (hi - lo) * 0.5);
  return [Math.round((lo - margin) * 10) / 10, Math.round((hi + margin) * 10) / 10];
}
