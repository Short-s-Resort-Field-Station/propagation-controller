// Adapter tests — the UI is a pure function of what these builders return,
// so the audit's truthfulness findings (3.2, 3.4, 3.8, 3.9, 3.11) pin here.
import { describe, it, expect } from 'vitest';
import {
  buildShelves, buildReservoir, buildMaintenance, buildRackStatus, buildLockouts,
  shelfNumber, siblingKey,
} from './adapter.js';

const NOW = Date.now();
const iso = (agoMs) => new Date(NOW - agoMs).toISOString();

function zone(over = {}) {
  return {
    zone_key: 'zone_2', name: 'Shelf 2', enabled: true, soil_sensor_key: 'soil_zone_2_moisture',
    config: { species: 'Bald Cypress', species_latin: 'Taxodium distichum', species_category: 'riparian' },
    latest: { moisture: 0.0, read_at: iso(10e3) },
    ...over,
  };
}

function inputs({ zones = [zone()], engine = {}, sensors = [], rules = {}, maintenance = false, lighting = {}, relays = [] } = {}) {
  return {
    zonesResp: { zones, engine },
    sensorsResp: { sensors },
    lighting: { relay_state: 'off', schedule: { on_time: '06:00', off_time: '19:00' }, ...lighting },
    relays: { relays },
    rules,
    maintenance,
  };
}

describe('shelf keys (3.8)', () => {
  it('derives the shelf number from zone_key, not the array index', () => {
    expect(shelfNumber('zone_7', 0)).toBe('07');
    expect(shelfNumber('zone_12', 0)).toBe('12');
    expect(shelfNumber('bench', 2)).toBe('03');
  });
  it('derives temperature/EC keys from the moisture key', () => {
    expect(siblingKey('soil_zone_2_moisture', 'temperature')).toBe('soil_zone_2_temperature');
    expect(siblingKey('soil_zone_2_moisture', 'ec')).toBe('soil_zone_2_ec');
    expect(siblingKey(null, 'ec')).toBeNull();
  });
  it('maps temperature and EC to the shelf by its own sensor key', () => {
    const sensors = [{ type: 'soil_sensor', measurements: [
      { sensor_key: 'soil_zone_2_temperature', name: 'temperature', value: 20, read_at: iso(1e3) },
      { sensor_key: 'soil_zone_2_ec', name: 'ec', value: 1131.4, read_at: iso(1e3) },
      { sensor_key: 'soil_zone_1_temperature', name: 'temperature', value: 0, read_at: iso(1e3) },
    ] }];
    const [s] = buildShelves(inputs({ sensors }));
    expect(s.n).toBe('02');
    expect(s.zoneKey).toBe('zone_2');
    expect(s.sensorKey).toBe('soil_zone_2_moisture');
    expect(s.temp).toBe(68);
    expect(s.ec).toBe(1131);
  });
});

describe('buildShelves status', () => {
  it('a fresh probe reading 0.0 %VWC is a reading, not a fault', () => {
    const [s] = buildShelves(inputs());
    expect(s.probe).toBe(true);
    expect(s.moisture).toBe(0);
    expect(s.status).toBe('Healthy');
  });
  it('a stale probe is a Sensor Fault', () => {
    const [s] = buildShelves(inputs({ zones: [zone({ latest: { moisture: 44, read_at: iso(6 * 60e3) } })] }));
    expect(s.probe).toBe(false);
    expect(s.status).toBe('Sensor Fault');
  });
  it('Draining comes from the engine phase, with the cycle exposed', () => {
    const engine = { active_cycle: { zone_key: 'zone_2', state: 'draining', duration_seconds: 300 }, queue: [] };
    const [s] = buildShelves(inputs({ engine }));
    expect(s.status).toBe('Draining');
    expect(s.cycle).toEqual({ state: 'draining', duration: 300 });
  });
  it('Watering while flooding, idle shelves carry no cycle', () => {
    const engine = { active_cycle: { zone_key: 'zone_2', state: 'flooding', duration_seconds: 60 }, queue: [] };
    const [a, b] = buildShelves(inputs({ engine, zones: [zone(), zone({ zone_key: 'zone_3', soil_sensor_key: 'soil_zone_3_moisture' })] }));
    expect(a.status).toBe('Watering');
    expect(a.cycle.state).toBe('flooding');
    expect(b.status).toBe('Healthy');
    expect(b.cycle).toBeNull();
  });
  it('Disabled (assigned, paused) vs Inactive (vacant bay)', () => {
    const [dis, vac] = buildShelves(inputs({ zones: [
      zone({ enabled: false }),
      zone({ zone_key: 'zone_3', enabled: false, config: {} }),
    ] }));
    expect(dis.status).toBe('Disabled');
    expect(dis.assigned).toBe(true);
    expect(vac.status).toBe('Inactive');
    expect(vac.assigned).toBe(false);
  });
  it('flags a missing moisture rule so the UI says "Target · not set"', () => {
    const [noRule] = buildShelves(inputs());
    expect(noRule.hasRule).toBe(false);
    const [withRule] = buildShelves(inputs({ rules: { zone_2: { trigger_below: 55, recovered_above: 70 } } }));
    expect(withRule.hasRule).toBe(true);
    expect(withRule.lo).toBe(55);
    expect(withRule.hi).toBe(70);
  });
  it('surfaces the manual lighting override', () => {
    const [s] = buildShelves(inputs({ lighting: { relay_state: 'on', override: { mode: 'on', until: iso(-3600e3), actor: 'manual_web' } } }));
    expect(s.light).toBe(true);
    expect(s.lightOverride.mode).toBe('on');
  });
});

describe('buildRackStatus / lockouts (3.11)', () => {
  const health = { status: 'ok' };
  it('is Locked Out when the safety supervisor holds a lockout', () => {
    const safety = { emergency_stop: { active: false }, lockouts: [{ key: 'pump_max_runtime', reason: 'x' }] };
    expect(buildRackStatus({ health, safety, zonesResp: { engine: {} }, maintenance: false })).toBe('Locked Out');
    expect(buildLockouts(safety)).toEqual(['pump_max_runtime']);
  });
  it('precedence: estop > active cycle > lockout > maintenance', () => {
    const lock = { emergency_stop: { active: false }, lockouts: [{ key: 'reservoir_low' }] };
    expect(buildRackStatus({ health, safety: { ...lock, emergency_stop: { active: true } }, zonesResp: { engine: {} }, maintenance: true })).toBe('Fault');
    expect(buildRackStatus({ health, safety: lock, zonesResp: { engine: { active_cycle: { state: 'flooding' } } }, maintenance: true })).toBe('Watering');
    expect(buildRackStatus({ health, safety: lock, zonesResp: { engine: {} }, maintenance: true })).toBe('Locked Out');
    expect(buildRackStatus({ health, safety: { emergency_stop: { active: false }, lockouts: [] }, zonesResp: { engine: {} }, maintenance: true })).toBe('Maintenance');
    expect(buildRackStatus({ health, safety: { emergency_stop: { active: false }, lockouts: [] }, zonesResp: { engine: {} }, maintenance: false })).toBe('Operational');
  });
});

describe('buildReservoir (3.9)', () => {
  const resv = (m) => ({ sensors: [{ type: 'reservoir_sensor', measurements: m }] });
  it('reads a fresh level measurement', () => {
    const r = buildReservoir(resv([
      { sensor_key: 'reservoir_level_temperature', name: 'temperature', value: 21, read_at: iso(5e3) },
      { sensor_key: 'reservoir_level_level', name: 'level', value: 52.4, read_at: iso(5e3) },
    ]), NOW);
    expect(r).toEqual({ pct: 52, online: true, readAt: expect.stringMatching(/^\d\d:\d\d$/) });
  });
  it('a stale level reading is offline', () => {
    const r = buildReservoir(resv([{ name: 'level', value: 52, read_at: iso(6 * 60e3) }]), NOW);
    expect(r.online).toBe(false);
    expect(r.pct).toBe(0);
  });
  it('no read_at, null value, or no reservoir device → offline', () => {
    expect(buildReservoir(resv([{ name: 'level', value: 52 }]), NOW).online).toBe(false);
    expect(buildReservoir(resv([{ name: 'level', value: null, read_at: iso(1e3) }]), NOW).online).toBe(false);
    expect(buildReservoir({ sensors: [] }, NOW)).toEqual({ pct: 0, online: false, readAt: '—' });
    expect(buildReservoir({}, NOW).online).toBe(false);
  });
  it('ignores non-level measurements on the device', () => {
    expect(buildReservoir(resv([{ name: 'temperature', value: 99, read_at: iso(1e3) }]), NOW).online).toBe(false);
  });
});

describe('buildMaintenance', () => {
  it('reads the parsed or stringified switch', () => {
    expect(buildMaintenance({ settings: { maintenance: { active: true } } })).toBe(true);
    expect(buildMaintenance({ settings: { maintenance: '{"active":true}' } })).toBe(true);
    expect(buildMaintenance({ settings: { maintenance: { active: false } } })).toBe(false);
  });
  it('tolerates absence and garbage', () => {
    expect(buildMaintenance({ settings: {} })).toBe(false);
    expect(buildMaintenance({})).toBe(false);
    expect(buildMaintenance(null)).toBe(false);
    expect(buildMaintenance({ settings: { maintenance: '{nope' } })).toBe(false);
  });
});
