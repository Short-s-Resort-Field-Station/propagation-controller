// useUnitState — the live bridge between propagationd and the design surfaces.
// Snapshot on load + steady polling (the handoff suggests 4–10s reading
// cadence; we poll at 2.5s which also covers relay/cycle changes promptly).

import React from 'react';
import { api } from './api.js';
import { PG } from '../design/data.jsx';
import {
  buildShelves, buildRackStatus, buildLog, buildReservoir,
  buildSpeciesCatalog, fmtClock,
} from './adapter.js';

// With SSE connected, pushes drive instant refreshes and polling drops to a
// slow safety net; without it (old proxies, EventSource failure), polling
// carries the load alone.
const POLL_MS = 2500;
const POLL_SLOW_MS = 15000;
const RULES_EVERY = 12; // moisture rules change rarely; refresh ~every 30s
const SSE_DEBOUNCE_MS = 300;

export function useUnitState(surface /* 'manual_web' | 'manual_touch' */) {
  const [state, setState] = React.useState({
    ready: false,
    connection: 'online',
    shelves: [],
    reservoir: 0,
    rackStatus: 'Operational',
    log: [],
    clock: fmtClock(),
    estop: false,
  });
  const rulesRef = React.useRef({});
  const tickRef = React.useRef(0);

  const refresh = React.useCallback(async () => {
    try {
      const [health, zonesResp, sensorsResp, lighting, relays, eventsResp, safety] =
        await Promise.all([
          api.health(), api.zones(), api.sensors(), api.lighting(),
          api.relays(), api.events(10), api.safety(),
        ]);

      if (tickRef.current % RULES_EVERY === 0) {
        const rules = {};
        await Promise.all((zonesResp.zones || []).map(async (z) => {
          try {
            const r = await api.moistureControl(z.zone_key);
            if (r && r.trigger_below != null) rules[z.zone_key] = r;
          } catch { /* no rule configured */ }
        }));
        rulesRef.current = rules;
      }
      tickRef.current++;

      setState({
        ready: true,
        connection: 'online',
        shelves: buildShelves({ zonesResp, sensorsResp, lighting, relays, rules: rulesRef.current }),
        reservoir: buildReservoir(sensorsResp),
        rackStatus: buildRackStatus({ health, safety, zonesResp }),
        log: buildLog(eventsResp),
        clock: fmtClock(),
        estop: !!(safety.emergency_stop && safety.emergency_stop.active),
      });
    } catch {
      setState((s) => ({ ...s, connection: 'offline', clock: fmtClock() }));
    }
  }, []);

  React.useEffect(() => {
    // Feed the real species register into the design catalog (AssignModal
    // reads PG.SPECIES).
    api.species()
      .then((r) => { PG.SPECIES = buildSpeciesCatalog(r); })
      .catch(() => { /* keep design defaults if unreachable */ });
    refresh();

    // SSE: any push schedules a debounced refresh (bursts collapse to one).
    let sseUp = false;
    let debounce = null;
    const onPush = () => {
      clearTimeout(debounce);
      debounce = setTimeout(refresh, SSE_DEBOUNCE_MS);
    };
    const es = new EventSource('/api/v1/events/stream');
    es.onopen = () => { sseUp = true; };
    es.onerror = () => { sseUp = false; };
    ['system_event', 'relay_action', 'sensor_reading', 'irrigation_cycle_update']
      .forEach((t) => es.addEventListener(t, onPush));

    // Polling: fast when SSE is down, slow heartbeat when it's up.
    let lastPoll = Date.now();
    const id = setInterval(() => {
      const due = sseUp ? POLL_SLOW_MS : POLL_MS;
      if (Date.now() - lastPoll >= due) {
        lastPoll = Date.now();
        refresh();
      }
    }, POLL_MS);

    return () => {
      clearInterval(id);
      clearTimeout(debounce);
      es.close();
    };
  }, [refresh]);

  // ---- actions, all attributed to this surface ----
  const actions = React.useMemo(() => ({
    water: (shelf) => shelf && api.runZone(shelf.zoneKey || 'zone_' + Number(shelf.n), surface)
      .then(refresh).catch((e) => console.error('water:', e.message)),
    stop: (shelf) => shelf && api.stopZone(shelf.zoneKey || 'zone_' + Number(shelf.n), surface)
      .then(refresh).catch((e) => console.error('stop:', e.message)),
    lights: (on) => api.lightingSet(on ? 'on' : 'off', surface)
      .then(refresh).catch((e) => console.error('lights:', e.message)),
    fans: (on) => api.setRelay(7, on ? 'on' : 'off', surface, 'circulation fans')
      .then(refresh).catch((e) => console.error('fans:', e.message)),
    assign: async (n, fields) => {
      const zoneKey = 'zone_' + Number(n);
      try {
        await api.patchZone(zoneKey, {
          species: fields.species,
          species_latin: fields.tag,
          species2: fields.species2 || '',
          species2_latin: fields.tag2 || '',
          enabled: true,
        });
        // Moisture band + soak time → a real moisture-control rule.
        const existing = await api.moistureControl(zoneKey).catch(() => null);
        const rule = existing && existing.trigger_below != null ? existing : null;
        const sensorKey = (rule && rule.sensor_key) || `soil_zone_${Number(n)}_moisture`;
        await api.putMoistureControl(zoneKey, {
          enabled: rule ? rule.enabled : true,
          sensor_key: sensorKey,
          trigger_below: fields.lo,
          recovered_above: fields.hi,
          run_duration_seconds: (fields.durMin || 5) * 60,
          minimum_interval_minutes: (rule && rule.minimum_interval_minutes) || 240,
          max_cycles_per_day: (rule && rule.max_cycles_per_day) || 4,
          sensor_offline_behavior: (rule && rule.sensor_offline_behavior) || 'lockout',
        }).catch(() => { /* zone may have no probe yet; assignment still stands */ });
        // Photoperiod from the modal is rack-wide (one lighting circuit).
        if (fields.on && fields.off && fields.on !== '—') {
          const sch = await api.lightingSchedule().catch(() => null);
          await api.putLightingSchedule({
            enabled: true,
            timezone: (sch && sch.timezone) || 'America/Chicago',
            on_time: fields.on,
            off_time: fields.off,
            days: (sch && sch.days) || [],
          }).catch(() => {});
        }
        refresh();
      } catch (e) {
        console.error('assign:', e.message);
      }
    },
  }), [surface, refresh]);

  return [state, actions];
}
