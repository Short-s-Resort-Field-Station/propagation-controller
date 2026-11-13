// useUnitState — the live bridge between propagationd and the design surfaces.
// Snapshot on load + steady polling (the handoff suggests 4–10s reading
// cadence; we poll at 2.5s which also covers relay/cycle changes promptly).

import React from 'react';
import { api, BASE } from './api.js';
import { FANS_RELAY, PUMP_RELAY, buildAutomation } from './adapter.js';
import { PG } from '../design/data.jsx';
import {
  buildShelves, buildRackStatus, buildLog, buildReservoir, buildLockouts,
  buildSpeciesCatalog, buildMaintenance, fmtClock,
} from './adapter.js';

// With SSE connected, pushes drive instant refreshes and polling drops to a
// slow safety net; without it (old proxies, EventSource failure), polling
// carries the load alone.
const POLL_MS = 2500;
const POLL_SLOW_MS = 15000;
const RULES_EVERY = 12; // moisture rules change rarely; refresh ~every 30s
const SSE_DEBOUNCE_MS = 300;
// Audit 3.5: the clock ticks on its own, independent of the network.
const CLOCK_MS = 15000;
// Audit 3.1: how long a rejected-command toast stays up.
export const TOAST_MS = 6000;
// Audit 3.10: dashboard manual lighting overrides lapse on their own.
export const LIGHT_OVERRIDE_MIN = 120;

export function useUnitState(surface /* 'manual_web' | 'manual_touch' */) {
  const [state, setState] = React.useState({
    ready: false,
    connection: 'online',
    shelves: [],
    reservoir: 0,
    // Audit 3.9: false while the level probe is stale/absent.
    reservoirOnline: false,
    reservoirReadAt: '—',
    rackStatus: 'Operational',
    lockouts: [],
    log: [],
    clock: fmtClock(),
    // Audit 3.6: wall-clock of the last successful refresh ("—" until one lands).
    lastSync: '—',
    estop: false,
    maintenance: false,
    // Audit 3.10: the active manual lighting override, or null.
    lightOverride: null,
    // Fan scheduler (2026-11-10): active manual override, or null; relay state.
    fanOverride: null,
    fanOn: false,
    pumpOn: false,
    // Watering automation switch + morning window.
    automation: { enabled: false, window: null },
    // Audit 3.1: the last rejected command, { text, at } or null.
    toast: null,
  });
  const rulesRef = React.useRef({});
  const tickRef = React.useRef(0);
  // Audit 3.5: one refresh at a time, and an older response never overwrites
  // a newer one.
  const inflightRef = React.useRef(false);
  const wantRef = React.useRef(false); // a push arrived mid-refresh → go again
  const seqRef = React.useRef(0);
  const shelvesRef = React.useRef([]);
  const toastTimer = React.useRef(null);

  // Audit 3.1: shared failure path — every rejected mutation lands here.
  const toast = React.useCallback((text) => {
    clearTimeout(toastTimer.current);
    setState((s) => ({ ...s, toast: { text: String(text || 'command rejected'), at: Date.now() } }));
    toastTimer.current = setTimeout(() => {
      setState((s) => (s.toast ? { ...s, toast: null } : s));
    }, TOAST_MS);
  }, []);
  const dismissToast = React.useCallback(() => {
    clearTimeout(toastTimer.current);
    setState((s) => (s.toast ? { ...s, toast: null } : s));
  }, []);

  const refresh = React.useCallback(async () => {
    if (inflightRef.current) { wantRef.current = true; return; }
    inflightRef.current = true;
    const seq = ++seqRef.current;
    try {
      const [health, zonesResp, sensorsResp, lighting, relays, eventsResp, safety, settingsResp, fans] =
        await Promise.all([
          api.health(), api.zones(), api.sensors(), api.lighting(),
          api.relays(), api.events(10), api.safety(),
          api.settings().catch(() => ({ settings: {} })),
          api.fans().catch(() => null),
        ]);
      const maintenance = buildMaintenance(settingsResp);
      const automation = buildAutomation(settingsResp);

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
      if (seq !== seqRef.current) return; // a newer refresh already landed

      const shelves = buildShelves({ zonesResp, sensorsResp, lighting, relays, rules: rulesRef.current, maintenance });
      shelvesRef.current = shelves;
      const reservoir = buildReservoir(sensorsResp);
      const now = fmtClock();
      setState((s) => ({
        ...s,
        ready: true,
        connection: 'online',
        shelves,
        reservoir: reservoir.pct,
        reservoirOnline: reservoir.online,
        reservoirReadAt: reservoir.readAt,
        rackStatus: buildRackStatus({ health, safety, zonesResp, maintenance }),
        lockouts: buildLockouts(safety),
        log: buildLog(eventsResp),
        clock: now,
        lastSync: now,
        estop: !!(safety.emergency_stop && safety.emergency_stop.active),
        maintenance,
        lightOverride: lighting.override
          ? { mode: lighting.override.mode, until: lighting.override.until || null }
          : null,
        fanOverride: fans && fans.override
          ? { mode: fans.override.mode, until: fans.override.until || null }
          : null,
        fanOn: fans ? fans.relay_state === 'on'
          : (relays.relays || []).some((r) => r.channel === FANS_RELAY && r.state === 'on'),
        // The pump relay as the board reads it back (list face footer, 2026-11-13).
        pumpOn: (relays.relays || []).some((r) => r.channel === PUMP_RELAY && r.state === 'on'),
        automation,
      }));
    } catch {
      if (seq === seqRef.current) {
        setState((s) => ({ ...s, connection: 'offline', clock: fmtClock() }));
      }
    } finally {
      inflightRef.current = false;
      if (wantRef.current) { wantRef.current = false; refresh(); }
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
    let es = null;
    const onPush = () => {
      clearTimeout(debounce);
      debounce = setTimeout(refresh, SSE_DEBOUNCE_MS);
    };
    if (typeof EventSource !== 'undefined') {
      es = new EventSource(BASE + '/events/stream');
      es.onopen = () => { sseUp = true; };
      es.onerror = () => { sseUp = false; };
      ['system_event', 'relay_action', 'sensor_reading', 'irrigation_cycle_update']
        .forEach((t) => es.addEventListener(t, onPush));
    }

    // Polling: fast when SSE is down, slow heartbeat when it's up. A tick is
    // skipped while the previous refresh is still in flight (refresh guards).
    let lastPoll = Date.now();
    const id = setInterval(() => {
      const due = sseUp ? POLL_SLOW_MS : POLL_MS;
      if (Date.now() - lastPoll >= due && !inflightRef.current) {
        lastPoll = Date.now();
        refresh();
      }
    }, POLL_MS);

    // Audit 3.5: the clock advances even when every fetch is hanging.
    const clockId = setInterval(() => {
      setState((s) => {
        const now = fmtClock();
        return s.clock === now ? s : { ...s, clock: now };
      });
    }, CLOCK_MS);

    return () => {
      clearInterval(id);
      clearInterval(clockId);
      clearTimeout(debounce);
      clearTimeout(toastTimer.current);
      if (es) es.close();
    };
  }, [refresh]);

  // ---- actions, all attributed to this surface ----
  const actions = React.useMemo(() => {
    // Every action resolves to true on success / false on rejection (after
    // toasting the backend's message) so callers can wait on it without a
    // try/catch and never see an unhandled rejection.
    const run = (label, p) => p
      .then(() => refresh())
      .then(() => true)
      .catch((e) => { toast(`${label}: ${e.message}`); return false; });

    // Audit 3.8: resolve a shelf's zone key from the live shelf list (the
    // design surfaces pass either a shelf object or its display number).
    const zoneKeyFor = (ref) => {
      if (ref && typeof ref === 'object') return ref.zoneKey || 'zone_' + Number(ref.n);
      const s = shelvesRef.current.find((x) => x.n === ref || Number(x.n) === Number(ref));
      return (s && s.zoneKey) || 'zone_' + Number(ref);
    };
    const sensorKeyFor = (ref, zoneKey) => {
      const s = ref && typeof ref === 'object' ? ref
        : shelvesRef.current.find((x) => x.n === ref || Number(x.n) === Number(ref));
      return (s && s.sensorKey) || `soil_${zoneKey}_moisture`;
    };

    return {
      water: (shelf) => shelf ? run('Water', api.runZone(zoneKeyFor(shelf), surface)) : Promise.resolve(false),
      drain: (shelf) => shelf ? run('Drain', api.drainZone(zoneKeyFor(shelf), surface)) : Promise.resolve(false),
      purge: (shelf) => shelf ? run('Purge', api.purgeZone(zoneKeyFor(shelf), surface)) : Promise.resolve(false),
      stop: (shelf) => shelf ? run('Stop', api.stopZone(zoneKeyFor(shelf), surface)) : Promise.resolve(false),
      // opts.expiresMinutes > 0 → the override lapses back to the schedule
      // (dashboard toggle, audit 3.10). mode 'auto' clears any override.
      lights: (on, opts) => run('Lights', api.lightingSet(
        on === 'auto' ? 'auto' : (on ? 'on' : 'off'), surface, opts && opts.expiresMinutes)),
      // Fans go through their scheduler (2026-11-10): a manual on/off is a
      // timed override that lapses back to the fan schedule; 'auto' clears it.
      fans: (on, opts) => run('Fans', api.fansSet(
        on === 'auto' ? 'auto' : (on ? 'on' : 'off'), surface,
        opts && opts.expiresMinutes != null ? opts.expiresMinutes : LIGHT_OVERRIDE_MIN)),
      // Watering automation master switch (settings key "automation"); the
      // backend keeps the stored watering window when only enabled is sent.
      setAutomation: (enabled) => run('Automation', api.putSettings({ automation: { enabled } })),
      // Audit 3.3: emergency stop + reset (dashboard, Grower+).
      emergencyStop: () => run('E-stop', api.emergencyStop(surface)),
      shutdown: () => run('Shut down', api.shutdown(surface)),
      safetyReset: () => run('Reset safety', api.safetyReset(surface)),
      // change 2 (maintenance): system-wide suspend of irrigation automation.
      setMaintenance: (active) => run('Maintenance', api.setMaintenance(active)),
      // change 2 (per-shelf disable): manual override — pause/resume a shelf's
      // sensors + valve without touching its assignment.
      setShelfEnabled: (n, enabled) => run(enabled ? 'Enable' : 'Disable', api.patchZone(zoneKeyFor(n), { enabled })),
      // change 2: clear a shelf's assignment → auto-disable (sensors + valve).
      unassign: (n) => run('Clear bay', api.patchZone(zoneKeyFor(n), {
        species: '', species_latin: '', species2: '', species2_latin: '', enabled: false,
      })),
      assign: async (n, fields) => {
        const zoneKey = zoneKeyFor(n);
        try {
          await api.patchZone(zoneKey, {
            species: fields.species,
            species_latin: fields.tag,
            species2: fields.species2 || '',
            species2_latin: fields.tag2 || '',
            enabled: true,
            // live-edit (ebb-flow): drain-back time — valve holds open after pump stops.
            drain_seconds: (fields.drainMin != null ? fields.drainMin : 5) * 60,
            // Post-drain purge (2026-11-11): valve re-open after the drain, pump off.
            purge_delay_seconds: (fields.purgeAfterMin != null ? fields.purgeAfterMin : 60) * 60,
            purge_seconds: fields.purgeSec != null ? fields.purgeSec : 60,
            // change 3: monitoring-only EC/temp happy-band + applied-profile
            // linkage (0 clears a bound, so an unset range removes any old band).
            ec_min: fields.ecLo ?? 0,
            ec_max: fields.ecHi ?? 0,
            temp_min_f: fields.tempLo ?? 0,
            temp_max_f: fields.tempHi ?? 0,
            species_id: fields.speciesId || '',
            profile_id: fields.profileId || 0,
            profile_name: fields.profileName || '',
            stage: fields.stage || '',
          });
          // Moisture band + soak time → a real moisture-control rule.
          const existing = await api.moistureControl(zoneKey).catch(() => null);
          const rule = existing && existing.trigger_below != null ? existing : null;
          const sensorKey = (rule && rule.sensor_key) || sensorKeyFor(n, zoneKey);
          await api.putMoistureControl(zoneKey, {
            enabled: rule ? rule.enabled : true,
            sensor_key: sensorKey,
            trigger_below: fields.lo,
            recovered_above: fields.hi,
            run_duration_seconds: (fields.durMin || 5) * 60,
            // change 3: a profile may carry cooldown + daily-cap; else keep the
            // existing rule's values (or the defaults).
            minimum_interval_minutes: fields.minIntervalMin != null ? fields.minIntervalMin
              : ((rule && rule.minimum_interval_minutes) || 240),
            max_cycles_per_day: fields.maxCycles != null ? fields.maxCycles
              : ((rule && rule.max_cycles_per_day) || 4),
            sensor_offline_behavior: (rule && rule.sensor_offline_behavior) || 'lockout',
          }).catch((e) => {
            // The assignment stands; the band simply isn't enforced yet.
            toast(`Moisture rule: ${e.message}`);
          });
          // Rules are cached; force them to reload with the next refresh.
          tickRef.current = 0;
          await refresh();
          return true;
        } catch (e) {
          toast(`Assign: ${e.message}`);
          return false;
        }
      },
      dismissToast,
      refresh,
    };
  }, [surface, refresh, toast, dismissToast]);

  return [state, actions];
}
