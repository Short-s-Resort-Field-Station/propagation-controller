// Maps propagationd API responses onto the design handoff's state shapes
// (§7 Data Model & Contract). The UI is a pure function of what this builds.

const CAT_STRIPE = {
  canopy: 'var(--sage-deep)', nurse: 'var(--ochre)', riparian: 'var(--river)',
  wildlife: 'var(--ochre)', specialty: 'var(--oxblood)', shrub: 'var(--sage)',
  vine: 'var(--sage)',
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

// Build the design's shelf objects from zones + sensors + lighting + rules.
export function buildShelves({ zonesResp, sensorsResp, lighting, relays, rules }) {
  const engine = zonesResp.engine || {};
  const activeZone = engine.active_cycle ? engine.active_cycle.zone_key : null;
  const queuedZones = new Set((engine.queue || []).map((q) => q.zone_key));

  // Rack-wide readings (one soil probe in v1 serves temp for all shelves)
  // plus per-shelf pH keyed by sensor key.
  let soilTempF = null;
  const phByKey = {};
  for (const dev of sensorsResp.sensors || []) {
    for (const m of dev.measurements || []) {
      if (m.name === 'temperature' && m.value != null) soilTempF = cToF(m.value);
      if (m.name === 'ph' && m.value != null) phByKey[m.sensor_key] = Math.round(m.value * 10) / 10;
    }
  }

  const fansOn = (relays.relays || []).some((r) => r.channel === 7 && r.state === 'on');
  const lightOn = lighting.relay_state === 'on';
  const sch = lighting.schedule || {};

  return (zonesResp.zones || []).map((z, i) => {
    const n = String(i + 1).padStart(2, '0');
    const cfg = z.config || {};
    const rule = rules[z.zone_key];
    const lo = rule ? Math.round(rule.trigger_below) : 0;
    const hi = rule ? Math.round(rule.recovered_above ?? Math.min(rule.trigger_below + 10, 95)) : 0;
    const moisture = z.latest ? Math.round(z.latest.moisture) : 0;
    const sensorFresh = z.latest && Date.now() - new Date(z.latest.read_at).getTime() < 5 * 60e3;

    // Moisture judgments (Dry/Monitoring) only apply when the shelf actually
    // has a probe reporting — a probe-less shelf is not "dry", it's unmetered.
    let status = 'Healthy';
    if (!z.enabled) status = 'Inactive';
    else if (z.zone_key === activeZone) status = 'Watering';
    else if (z.soil_sensor_key && !sensorFresh) status = 'Sensor Fault';
    else if (rule && z.soil_sensor_key && sensorFresh && moisture < lo) status = 'Dry';
    else if (rule && z.soil_sensor_key && sensorFresh && moisture < lo + 3) status = 'Monitoring';
    else if (queuedZones.has(z.zone_key)) status = 'Monitoring';

    const last = z.last_cycle;
    const species1 = cfg.species || (z.enabled ? z.name : 'Unassigned');
    const species = cfg.species2 ? `${species1} + ${cfg.species2}` : species1;
    const tag1 = (cfg.species_latin || (z.enabled ? '' : 'EMPTY BAY')).toUpperCase();
    const tag = cfg.species2_latin ? `${tag1} · ${cfg.species2_latin.toUpperCase()}` : tag1;
    const cat = (cfg.species_category || '').toLowerCase();

    return {
      n,
      zoneKey: z.zone_key,
      active: z.enabled,
      species,
      tag,
      // Raw fields for the quick-edit modal (display strings above are joined).
      raw: {
        species1: cfg.species || '',
        species2: cfg.species2 || '',
        durMin: rule ? Math.max(1, Math.round(rule.run_duration_seconds / 60)) : 0,
      },
      moisture: z.enabled ? moisture : 0,
      temp: z.enabled && soilTempF != null ? soilTempF : 0,
      ph: z.enabled ? (phByKey[`soil_zone_${i + 1}_ph`] ?? null) : null,
      lo: lo || 40,
      hi: hi || 70,
      light: lightOn,
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

export function buildRackStatus({ health, safety, zonesResp }) {
  if (safety.emergency_stop && safety.emergency_stop.active) return 'Fault';
  if (health.status !== 'ok') return 'Fault';
  if (zonesResp.engine && zonesResp.engine.active_cycle) return 'Watering';
  return 'Operational';
}

export function buildLog(eventsResp) {
  return (eventsResp.events || []).map((e) => ({
    ts: fmtTs(e.occurred_at),
    ev: e.message,
    em: (e.event_type || '').replace(/_/g, ' ').toUpperCase() + (e.actor ? ' · ' + e.actor.toUpperCase() : ''),
    sev: eventColor(e),
  }));
}

export function buildReservoir(sensorsResp) {
  for (const dev of sensorsResp.sensors || []) {
    if (dev.type !== 'reservoir_sensor') continue;
    for (const m of dev.measurements || []) {
      if (m.value != null) return Math.round(m.value);
    }
  }
  return 0;
}

// Map the backend species register onto the AssignModal's catalog shape.
export function buildSpeciesCatalog(speciesResp) {
  return (speciesResp.species || []).map((s) => ({
    species: s.common_name,
    tag: s.latin_name.toUpperCase(),
    hab: s.category.charAt(0).toUpperCase() + s.category.slice(1) +
      ' · ' + s.method + ' · difficulty ' + s.difficulty + '/5',
    category: s.category,
    lo: 40, hi: 70, temp: 75, onH: 6, offH: 19, fan: 1,
    stripe: CAT_STRIPE[s.category] || 'var(--sage)',
  }));
}
