// Maps propagationd API responses onto the design handoff's state shapes
// (§7 Data Model & Contract). The UI is a pure function of what this builds.

// Relay channel (spec 1-indexed) carrying the ganged circulation fans — as
// built: Numato silkscreen 3. Keep in sync with devices.FansChannel.
// Pump relay (software channel 7, silkscreen 6) — the footer's truth-light reads it.
export const PUMP_RELAY = 7;
export const FANS_RELAY = 4;

const CAT_STRIPE = {
  canopy: 'var(--sage-deep)', nurse: 'var(--ochre)', riparian: 'var(--river)',
  wildlife: 'var(--ochre)', specialty: 'var(--oxblood)', shrub: 'var(--sage)',
  vine: 'var(--sage)', grass: 'var(--river)',
};

export const cToF = (c) => Math.round(c * 9 / 5 + 32);

export function fmtClock(d = new Date()) {
  return String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
}

export function fmtDay(d = new Date()) {
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)} / ${p(d.getDate())} / ${String(d.getFullYear()).slice(2)}`;
}

function fmtTs(iso) {
  const d = new Date(iso);
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}/${p(d.getDate())}/${String(d.getFullYear()).slice(2)} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

const EVENT_SEV = {
  error: 'var(--oxblood)',
  warning: 'var(--ochre)',
  info: 'var(--sage)',
};

function eventColor(e) {
  if (e.level !== 'info') return EVENT_SEV[e.level] || 'var(--sage)';
  if (/irrigation|moisture|water/.test(e.event_type)) return 'var(--river)';
  if (/lighting/.test(e.event_type)) return 'var(--ochre)';
  return 'var(--sage)';
}

// Audit 3.8: zone_key "zone_7" → "07"; falls back to list position only when
// the key carries no number.
export function shelfNumber(zoneKey, index = 0) {
  const m = /zone_(\d+)/.exec(zoneKey || '');
  return String(m ? Number(m[1]) : index + 1).padStart(2, '0');
}

// Audit 3.8: a shelf's temperature/EC keys are siblings of its moisture key
// (soil_zone_2_moisture → soil_zone_2_temperature), never the array index.
export function siblingKey(sensorKey, name) {
  if (!sensorKey) return null;
  return sensorKey.replace(/_moisture$/, '_' + name);
}

// Build the design's shelf objects from zones + sensors + lighting + rules.
export function buildShelves({ zonesResp, sensorsResp, lighting, relays, rules, maintenance }) {
  const engine = zonesResp.engine || {};
  const activeZone = engine.active_cycle ? engine.active_cycle.zone_key : null;
  // live-edit (ebb-flow): the active cycle's fine-grained phase, so the fill
  // (pump on) and drain-back (pump off, valve open) phases read distinctly.
  const activeState = engine.active_cycle ? engine.active_cycle.state : null;
  const queuedZones = new Set((engine.queue || []).map((q) => q.zone_key));
  // Queue detail (Austin, 2026-11-13): a shelf waiting behind another's cycle
  // shows as Queued with a Cancel on its detail screen — yesterday a request
  // queued by accident could not be undone from the panel.
  const queueInfo = {};
  (engine.queue || []).forEach((q, idx) => {
    queueInfo[q.zone_key] = {
      pos: idx + 1 + (engine.active_cycle ? 1 : 0),
      kind: q.drain_only ? (q.trigger_source === 'purge' ? 'Purge' : 'Drain') : 'Water',
      behind: idx === 0 ? (engine.active_cycle ? engine.active_cycle.zone_key : null) : (engine.queue[idx - 1].zone_key),
    };
  });
  // Post-drain purges the engine has armed (valve re-open, pump off) — shown
  // so a hand purge isn't fired on a shelf that is about to purge itself.
  const purgeDueByZone = {};
  for (const p of engine.pending_purges || []) purgeDueByZone[p.zone_key] = { at: p.due_at, seconds: p.seconds };

  // Per-shelf root-zone temperature (°F) and substrate EC (µS/cm), keyed by
  // sensor key — every shelf has its own probe as of the 2026-11-09 build.
  const tempFByKey = {};
  const ecByKey = {};
  for (const dev of sensorsResp.sensors || []) {
    for (const m of dev.measurements || []) {
      if (m.name === 'temperature' && m.value != null) tempFByKey[m.sensor_key] = cToF(m.value);
      if (m.name === 'ec' && m.value != null) ecByKey[m.sensor_key] = Math.round(m.value);
    }
  }

  const fansOn = (relays.relays || []).some((r) => r.channel === FANS_RELAY && r.state === 'on');
  const lightOn = lighting.relay_state === 'on';
  const sch = lighting.schedule || {};
  // Audit 3.10: a manual override is shown wherever the lights are toggled.
  const lightOverride = lighting.override
    ? { mode: lighting.override.mode, until: lighting.override.until || null, actor: lighting.override.actor || '' }
    : null;
  // Audit 3.2: the active cycle's real phase + duration, for the kiosk label.
  const cycle = engine.active_cycle
    ? { state: engine.active_cycle.state, duration: engine.active_cycle.duration_seconds || 0 }
    : null;

  return (zonesResp.zones || []).map((z, i) => {
    // Audit 3.8: shelf number + probe keys come from the zone itself, not the
    // array index — zones may be deleted or renamed.
    const n = shelfNumber(z.zone_key, i);
    const sensorKey = z.soil_sensor_key || null;
    const cfg = z.config || {};
    const rule = rules[z.zone_key];
    const lo = rule ? Math.round(rule.trigger_below) : 0;
    const hi = rule ? Math.round(rule.recovered_above ?? Math.min(rule.trigger_below + 10, 95)) : 0;
    const moisture = z.latest ? Math.round(z.latest.moisture) : 0;
    const sensorFresh = z.latest && Date.now() - new Date(z.latest.read_at).getTime() < 5 * 60e3;

    // A shelf with a specimen assigned but disabled is *paused* (manual
    // override or its assignment is mid-change), distinct from a vacant bay.
    const assigned = !!cfg.species;

    // Moisture judgments (Dry/Monitoring) only apply when the shelf actually
    // has a probe reporting — a probe-less shelf is not "dry", it's unmetered.
    let status = 'Healthy';
    if (!z.enabled) status = assigned ? 'Disabled' : 'Inactive';
    else if (z.zone_key === activeZone) status = activeState === 'draining' ? 'Draining' : 'Watering';
    else if (z.soil_sensor_key && !sensorFresh) status = 'Sensor Fault';
    else if (rule && z.soil_sensor_key && sensorFresh && moisture < lo) status = 'Dry';
    else if (rule && z.soil_sensor_key && sensorFresh && moisture < lo + 3) status = 'Monitoring';
    else if (queuedZones.has(z.zone_key)) status = 'Queued';

    // change 2 (maintenance): while maintenance mode is on, calm the per-shelf
    // moisture/probe alarms (pulling probes or moving trays shouldn't raise
    // Sensor Fault / Dry / Monitoring). The readings still display; only the
    // alarm is suppressed. Watering/Disabled/Inactive are left untouched.
    if (maintenance && (status === 'Sensor Fault' || status === 'Dry' || status === 'Monitoring')) {
      status = 'Maintenance';
    }

    const last = z.last_cycle;
    const species1 = cfg.species || (z.enabled ? z.name : 'Unassigned');
    const species = cfg.species2 ? `${species1} + ${cfg.species2}` : species1;
    const tag1 = (cfg.species_latin || (z.enabled ? '' : 'EMPTY BAY')).toUpperCase();
    const tag = cfg.species2_latin ? `${tag1} · ${cfg.species2_latin.toUpperCase()}` : tag1;
    // Latin names as written (italicised by the UIs, after the common name).
    const latin = [cfg.species_latin, cfg.species2_latin].filter(Boolean).join(' · ');
    const cat = (cfg.species_category || '').toLowerCase();

    return {
      n,
      zoneKey: z.zone_key,
      sensorKey,
      active: z.enabled,
      // Audit 3.2/3.4: this shelf's live cycle phase (null when idle).
      cycle: z.zone_key === activeZone ? cycle : null,
      // Queue position/kind while this shelf waits its turn (null otherwise).
      queued: queueInfo[z.zone_key] ? {
        ...queueInfo[z.zone_key],
        behind: queueInfo[z.zone_key].behind ? shelfNumber(queueInfo[z.zone_key].behind, -1) : null,
      } : null,
      // Armed post-drain purge for this shelf: { at: ISO, seconds } or null.
      purgeDue: purgeDueByZone[z.zone_key] || null,
      // Audit 3.11: whether a moisture rule exists — without one there is no
      // target band to draw ("Target · not set").
      hasRule: !!rule,
      // change 2: a paused shelf (assigned but disabled) vs a vacant bay.
      assigned,
      species,
      tag,
      latin,
      // change 3 (species profiles): monitoring-only happy-band ranges + the
      // applied-profile linkage, surfaced for the gauges, adherence tracking,
      // and the edit/save-profile flow.
      ecBand: (cfg.ec_min != null && cfg.ec_max != null) ? [cfg.ec_min, cfg.ec_max] : null,
      tempBand: (cfg.temp_min_f != null && cfg.temp_max_f != null) ? [cfg.temp_min_f, cfg.temp_max_f] : null,
      speciesId: cfg.species_id || '',
      profileId: cfg.profile_id || 0,
      profileName: cfg.profile_name || '',
      stage: cfg.stage || '',
      // Raw fields for the quick-edit modal (display strings above are joined).
      raw: {
        species1: cfg.species || '',
        species2: cfg.species2 || '',
        durMin: rule ? Math.max(1, Math.round(rule.run_duration_seconds / 60)) : 0,
        // live-edit (ebb-flow): drain-back time (valve holds open), minutes.
        drainMin: Math.max(0, Math.round((cfg.drain_seconds ?? 300) / 60)),
        // Post-drain purge (2026-11-11): minutes after the drain, seconds open.
        purgeAfterMin: Math.max(0, Math.round((cfg.purge_delay_seconds ?? 0) / 60)),
        purgeSec: Math.max(0, Math.round(cfg.purge_seconds ?? 0)),
        // change 3: EC/temp target ranges for the edit modal (happy band).
        ecLo: cfg.ec_min ?? null, ecHi: cfg.ec_max ?? null,
        tempLo: cfg.temp_min_f ?? null, tempHi: cfg.temp_max_f ?? null,
        speciesId: cfg.species_id || '', profileId: cfg.profile_id || 0, stage: cfg.stage || '',
        // change 3: moisture guards (for round-tripping a shelf → saved profile).
        coolMin: rule ? rule.minimum_interval_minutes : null,
        maxCycles: rule ? rule.max_cycles_per_day : null,
      },
      // A probe is "present" when its sensor is reporting, regardless of the
      // value — a probe in air legitimately reads 0.0 %VWC (bring-up 2026-11-09).
      probe: !!(z.enabled && z.soil_sensor_key && sensorFresh),
      moisture: z.enabled ? moisture : 0,
      temp: z.enabled ? (tempFByKey[siblingKey(sensorKey, 'temperature')] ?? 0) : 0,
      ec: z.enabled ? (ecByKey[siblingKey(sensorKey, 'ec')] ?? null) : null,
      lo: lo || 40,
      hi: hi || 70,
      light: lightOn,
      lightOverride,
      on: sch.on_time || '—',
      off: sch.off_time || '—',
      fan: fansOn,
      lastWater: last ? fmtClock(new Date(last.started_at)) : '—',
      lastWaterDate: last ? (() => {
        const d = new Date(last.started_at);
        const p = (v) => String(v).padStart(2, '0');
        return `${p(d.getMonth() + 1)}/${p(d.getDate())}/${String(d.getFullYear()).slice(2)}`;
      })() : '',
      usage: last && last.actual_duration_seconds != null
        ? `${Math.round((last.actual_duration_seconds / 60) * 10) / 10} min`
        : '—',
      status,
      stripe: CAT_STRIPE[cat] || 'var(--river)',
    };
  });
}

export function buildRackStatus({ health, safety, zonesResp, maintenance }) {
  if (safety.emergency_stop && safety.emergency_stop.active) return 'Fault';
  if (health.status !== 'ok') return 'Fault';
  if (zonesResp.engine && zonesResp.engine.active_cycle) {
    // live-edit (ebb-flow): distinguish the drain-back phase in the rack header.
    // A cycle in progress (e.g. a manual water during maintenance) still reads
    // as Watering/Draining; the maintenance banner shows the mode separately.
    return zonesResp.engine.active_cycle.state === 'draining' ? 'Draining' : 'Watering';
  }
  // Audit 3.11: a safety lockout means Water Now is guaranteed to 409 — say
  // so in the header instead of "Operational".
  if (buildLockouts(safety).length) return 'Locked Out';
  // change 2 (maintenance): the rack header reads Maintenance while suspended,
  // unless a safety fault or an active cycle takes precedence above.
  if (maintenance) return 'Maintenance';
  return 'Operational';
}

// Audit 3.11: active safety lockout keys (e.g. "pump_max_runtime",
// "reservoir_low"), shown in the rack header / dashboard status cell.
export function buildLockouts(safety) {
  return ((safety && safety.lockouts) || []).map((l) => l.key || String(l)).filter(Boolean);
}

// change 2 (maintenance): read the maintenance switch from GET /settings.
// The settings endpoint returns each value as raw JSON, which arrives here
// already parsed; tolerate a stringified value too.
export function buildMaintenance(settingsResp) {
  const m = settingsResp && settingsResp.settings ? settingsResp.settings.maintenance : null;
  if (m == null) return false;
  try {
    const obj = typeof m === 'string' ? JSON.parse(m) : m;
    return !!(obj && obj.active);
  } catch {
    return false;
  }
}

// Watering automation switch + optional morning window from the "automation"
// setting (2026-11-10). A missing/malformed value reads as off, like the daemon.
export function buildAutomation(settingsResp) {
  const a = settingsResp && settingsResp.settings ? settingsResp.settings.automation : null;
  if (a == null) return { enabled: false, window: null };
  try {
    const obj = typeof a === 'string' ? JSON.parse(a) : a;
    const w = obj && obj.watering_window && obj.watering_window.start && obj.watering_window.end
      ? { start: obj.watering_window.start, end: obj.watering_window.end } : null;
    return { enabled: !!(obj && obj.enabled), window: w };
  } catch {
    return { enabled: false, window: null };
  }
}

export function buildLog(eventsResp) {
  return (eventsResp.events || []).map((e) => ({
    ts: fmtTs(e.occurred_at),
    ev: e.message,
    em: (e.event_type || '').replace(/_/g, ' ').toUpperCase() + (e.actor ? ' · ' + e.actor.toUpperCase() : ''),
    sev: eventColor(e),
  }));
}

// Audit 3.9: the reservoir percentage is only trusted while its level
// measurement is fresh (< 5 min). A stale/absent probe reports online:false so
// the surfaces draw "— %" and "sensor offline" instead of a confident number.
export const RESERVOIR_STALE_MS = 5 * 60e3;
export function buildReservoir(sensorsResp, now = Date.now()) {
  for (const dev of (sensorsResp && sensorsResp.sensors) || []) {
    if (dev.type !== 'reservoir_sensor') continue;
    for (const m of dev.measurements || []) {
      if (m.name !== 'level') continue;
      const readAt = m.read_at ? new Date(m.read_at).getTime() : NaN;
      const fresh = m.value != null && Number.isFinite(readAt) && now - readAt < RESERVOIR_STALE_MS;
      return {
        pct: fresh ? Math.max(0, Math.min(100, Math.round(m.value))) : 0,
        online: fresh,
        readAt: Number.isFinite(readAt) ? fmtClock(new Date(readAt)) : '—',
      };
    }
  }
  return { pct: 0, online: false, readAt: '—' };
}

// Map the backend species register onto the AssignModal's catalog shape.
export function buildSpeciesCatalog(speciesResp) {
  return (speciesResp.species || []).map((s) => ({
    id: s.id, // change 3: carry the botanical ID so profiles can key off it
    species: s.common_name,
    tag: s.latin_name.toUpperCase(),
    latin: s.latin_name,
    hab: s.category.charAt(0).toUpperCase() + s.category.slice(1) +
      ' · ' + s.method + ' · difficulty ' + s.difficulty + '/5',
    category: s.category,
    lo: 40, hi: 70, temp: 75, onH: 6, offH: 19, fan: 1,
    // change 3: default happy-band ranges (monitoring only) until a profile
    // or manual entry overrides them. EC in µS/cm — propagation cuttings/
    // seedlings generally want a low-salts substrate (~500–1500).
    ecLo: 500, ecHi: 1500, tempLo: 68, tempHi: 78,
    stripe: CAT_STRIPE[s.category] || 'var(--sage)',
  }));
}
