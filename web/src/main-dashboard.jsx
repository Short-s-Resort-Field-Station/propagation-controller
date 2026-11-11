// Surface B — the paper control center, served at "/".
import React from 'react';
import ReactDOM from 'react-dom/client';
import './design/pgui.css';
import { PG } from './design/data.jsx';
import { Dashboard } from './design/dashboard.jsx';
import { useUnitState } from './live/useUnitState.js';
import { getEnvSeries } from './live/series.js';
import { api, onUnauthorized } from './live/api.js';
import { LIGHT_OVERRIDE_MIN } from './live/useUnitState.js';
import { LiveSignInModal, SetupModal, OperatorsModal, toDesignSession } from './live/auth-ui.jsx';

// Live Controls-tab bridge (design/controls.jsx): rack-wide schedules and
// settings, per-shelf rules, and the manual buttons — all through the API.
function makeControls(state, actions) {
  const list = state.shelves.map((s) => ({
    n: s.n, zoneKey: s.zoneKey, species: s.species, latin: s.latin, active: s.active,
    status: s.status, cycle: s.cycle, moisture: s.moisture, probe: s.probe,
  }));
  const zoneFor = (n) => (list.find((x) => x.n === n) || {}).zoneKey || 'zone_' + Number(n);
  const sched = (sch) => (sch ? { enabled: !!sch.enabled, on: sch.on_time, off: sch.off_time, timezone: sch.timezone, days: sch.days } : null);

  return {
    list,
    live: {
      lightOn: state.shelves.length ? state.shelves[0].light : false,
      lightOverride: state.lightOverride,
      fanOn: state.fanOn,
      fanOverride: state.fanOverride,
      maintenance: state.maintenance,
      automation: state.automation,
      lockouts: state.lockouts,
      reservoir: state.reservoir,
    },
    loadShelf: async (n) => {
      const zoneKey = zoneFor(n);
      const [mc, zonesResp] = await Promise.all([
        api.moistureControl(zoneKey).catch(() => null),
        api.zones().catch(() => null),
      ]);
      const rule = mc && mc.trigger_below != null ? mc : null;
      const zone = zonesResp && (zonesResp.zones || []).find((z) => z.zone_key === zoneKey);
      const cfg = (zone && zone.config) || {};
      return {
        trigger: rule ? Math.round(rule.trigger_below) : 16,
        rearm: rule ? Math.round(rule.recovered_above ?? rule.trigger_below + 8) : 24,
        durMin: rule ? Math.max(1, Math.round(rule.run_duration_seconds / 60)) : Math.max(1, Math.round((cfg.default_run_seconds || 1020) / 60)),
        drainMin: Math.max(0, Math.round((cfg.drain_seconds ?? 600) / 60)),
        coolH: rule ? Math.max(1, Math.round(rule.minimum_interval_minutes / 60)) : 12,
        maxCycles: rule && rule.max_cycles_per_day != null ? rule.max_cycles_per_day : 1,
      };
    },
    applyShelf: async (scope, n, irr) => {
      const targets = scope === 'all' ? list.map((x) => x.zoneKey) : [zoneFor(n)];
      for (const zoneKey of targets) {
        const existing = await api.moistureControl(zoneKey).catch(() => null);
        const rule = existing && existing.trigger_below != null ? existing : null;
        await api.patchZone(zoneKey, { drain_seconds: irr.drainMin * 60, default_run_seconds: irr.durMin * 60 });
        await api.putMoistureControl(zoneKey, {
          enabled: rule ? rule.enabled : true,
          sensor_key: (rule && rule.sensor_key) || `soil_${zoneKey}_moisture`,
          trigger_below: irr.trigger,
          recovered_above: irr.rearm,
          run_duration_seconds: irr.durMin * 60,
          minimum_interval_minutes: irr.coolH * 60,
          max_cycles_per_day: irr.maxCycles,
          sensor_offline_behavior: (rule && rule.sensor_offline_behavior) || 'lockout',
        });
      }
      await actions.refresh();
    },
    loadRack: async () => {
      const [lights, fans, settings] = await Promise.all([
        api.lightingSchedule().catch(() => null),
        api.fansSchedule().catch(() => null),
        api.settings().catch(() => ({ settings: {} })),
      ]);
      const safety = (settings.settings || {}).safety || {};
      const auto = (settings.settings || {}).automation || {};
      const w = auto.watering_window && auto.watering_window.start ? auto.watering_window : null;
      return { lights: sched(lights), fans: sched(fans), window: w, minReservoir: safety.min_reservoir_level ?? 20, safety };
    },
    applyRack: async ({ lights, fans, window, minReservoir }) => {
      const put = async (get, set, v) => {
        const cur = await get().catch(() => null);
        await set({ enabled: v.enabled, timezone: (cur && cur.timezone) || 'America/Chicago', on_time: v.on, off_time: v.off, days: (cur && cur.days) || [] });
      };
      if (lights) await put(api.lightingSchedule, api.putLightingSchedule, lights);
      if (fans) await put(api.fansSchedule, api.putFansSchedule, fans);
      if (window !== undefined || minReservoir !== undefined) {
        const settings = await api.settings().catch(() => ({ settings: {} }));
        const body = {};
        if (window !== undefined) {
          const auto = (settings.settings || {}).automation || {};
          body.automation = { enabled: !!auto.enabled, watering_window: window };
        }
        if (minReservoir !== undefined) {
          const safety = (settings.settings || {}).safety;
          if (safety) body.safety = { ...safety, min_reservoir_level: minReservoir };
        }
        await api.putSettings(body);
      }
      await actions.refresh();
    },
    manualLight: (mode) => actions.lights(mode === 'auto' ? 'auto' : mode === 'on', { expiresMinutes: LIGHT_OVERRIDE_MIN }),
    manualFan: (mode) => actions.fans(mode === 'auto' ? 'auto' : mode === 'on', { expiresMinutes: LIGHT_OVERRIDE_MIN }),
    setAutomation: (enabled) => actions.setAutomation(enabled),
    setMaintenance: (active) => actions.setMaintenance(active),
    water: (n) => actions.water(n),
    drain: (n) => actions.drain(n),
    stop: (n) => actions.stop(n),
  };
}

function App() {
  const [state, actions] = useUnitState('manual_web');
  const [session, setSession] = React.useState(undefined); // undefined = loading
  const [setupRequired, setSetupRequired] = React.useState(false);
  const [showOperators, setShowOperators] = React.useState(false);
  const [acked, setAcked] = React.useState({});
  const [signInRequest, setSignInRequest] = React.useState(0);

  React.useEffect(() => {
    api.session()
      .then((r) => { setSession(toDesignSession(r.user)); setSetupRequired(r.setup_required); })
      .catch(() => setSession(null));
    // audit 3.10/3.11: any 401 (expired cookie, revoked account) clears the
    // session and reopens sign-in; the toast carries the backend's message.
    onUnauthorized(() => {
      setSession((s) => (s ? null : s));
      setSignInRequest((n) => n + 1);
    });
    return () => onUnauthorized(null);
  }, []);

  const signOut = () => { api.logout().catch(() => {}); setSession(null); };
  const canManage = !!(session && session.role === 'Steward');
  const authBridge = React.useMemo(() => ({
    SignInModal: LiveSignInModal,
    canManage,
    openOperators: () => setShowOperators(true),
  }), [canManage]);

  // audit 3.9: no "Reservoir Low" from a stale probe — a sensor-offline alert instead.
  const alerts = PG.deriveAlerts(state.shelves, state.reservoirOnline ? state.reservoir : 100, state.clock)
    .concat(state.ready && !state.reservoirOnline
      ? [{ id: 'RESOFF', ts: state.clock, shelf: '—', kind: 'Sensor Fault', sev: 'var(--oxblood)',
          prio: 'ACTION REQUIRED', msg: 'Reservoir level sensor not reporting — refill guard is blind.' }]
      : [])
    .filter((a) => !acked[a.id]);

  const getSeries = React.useCallback(
    (period) => getEnvSeries(period, state.shelves),
    [state.shelves],
  );
  const controls = React.useMemo(
    () => makeControls(state, actions),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [state.shelves, state.lightOverride, state.fanOverride, state.fanOn, state.maintenance,
      state.automation, state.lockouts, state.reservoir, actions],
  );
  // change 3: species-profiles bridge for the assign/edit modal.
  const profiles = React.useMemo(() => ({
    list: (speciesId) => api.speciesProfiles(speciesId).then((r) => r.profiles || []),
    save: (body) => api.createProfile(body),
    update: (id, body) => api.updateProfile(id, body),
  }), []);

  if (!state.ready || session === undefined) {
    return (
      <div style={{ fontFamily: 'JetBrains Mono, monospace', padding: 40, color: '#1B1813',
        background: '#ECE3CE', minHeight: '100vh', letterSpacing: '0.1em' }}>
        CONNECTING TO PROPAGATION UNIT 01…
      </div>
    );
  }

  if (setupRequired) {
    return (
      <div className="live-db">
        <SetupModal onDone={(user) => { setSession(user); setSetupRequired(false); }} />
      </div>
    );
  }

  return (
    <div className="live-db">
      <Dashboard
        shelves={state.shelves}
        reservoir={state.reservoir}
        reservoirOnline={state.reservoirOnline}
        rackStatus={state.rackStatus}
        lockouts={state.lockouts}
        lastSync={state.lastSync}
        toast={state.toast}
        onDismissToast={actions.dismissToast}
        estop={state.estop}
        onEmergencyStop={actions.emergencyStop}
        onSafetyReset={actions.safetyReset}
        signInRequest={signInRequest}
        log={state.log}
        session={session}
        onSignIn={setSession}
        onSignOut={signOut}
        auth={authBridge}
        alerts={alerts}
        onAck={(id) => setAcked((a) => ({ ...a, [id]: true }))}
        onAckAll={() => setAcked((a) => {
          const n = { ...a };
          alerts.forEach((x) => { n[x.id] = true; });
          return n;
        })}
        connection={state.connection}
        assignments={{}}
        onAssign={actions.assign}
        getSeries={getSeries}
        controls={controls}
        maintenance={state.maintenance}
        onSetMaintenance={actions.setMaintenance}
        onSetEnabled={actions.setShelfEnabled}
        onUnassign={actions.unassign}
        profiles={profiles}
      />
      {showOperators && <OperatorsModal onClose={() => setShowOperators(false)}
        selfName={session ? session.name : ''} />}
    </div>
  );
}

document.body.style.background = '#ECE3CE';
document.body.style.margin = '0';
ReactDOM.createRoot(document.getElementById('root')).render(<App />);
