// Surface B — the paper control center, served at "/".
import React from 'react';
import ReactDOM from 'react-dom/client';
import './design/pgui.css';
import './design/rack3.css';
import './design/console.css';
import { PG } from './design/data.jsx';
import { Dashboard } from './design/dashboard.jsx';
import { useUnitState } from './live/useUnitState.js';
import { getEnvSeries } from './live/series.js';
import { api, onUnauthorized, exportUrl } from './live/api.js';
import { LIGHT_OVERRIDE_MIN } from './live/useUnitState.js';
import { LiveSignInModal, SetupModal, OperatorsModal, toDesignSession } from './live/auth-ui.jsx';
import { usePush } from './live/push-ui.jsx';
import { buildLog } from './live/adapter.js';

// Live Controls-tab bridge (design/controls.jsx): rack-wide schedules and
// settings plus the per-shelf action buttons. Per-shelf settings live only
// in Shelf Management's edit modal (single source of truth).
// Stable identity so the Control box panel's poll is not restarted every render.
const loadBox = () => api.box();

function makeControls(state, actions) {
  const list = state.shelves.map((s) => ({
    n: s.n, zoneKey: s.zoneKey, species: s.species, latin: s.latin, active: s.active, assigned: s.assigned,
    status: s.status, cycle: s.cycle, moisture: s.moisture, probe: s.probe,
    durMin: (s.raw && s.raw.durMin) || 0, drainMin: (s.raw && s.raw.drainMin) || 0,
    // 2026-11-15 review: the Controls rows read these for Purge and Cancel.
    queued: s.queued || null, raw: s.raw || null,
  }));
  const sched = (sch) => (sch ? { enabled: !!sch.enabled, on: sch.on_time, off: sch.off_time, timezone: sch.timezone, days: sch.days } : null);

  return {
    list,
    loadBox,
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
        // 2026-11-15 review: never default the current settings — a failed
        // read used to PUT automation.enabled=false and report "Saved".
        const settings = await api.settings();
        const body = {};
        if (window !== undefined) {
          // A failed read throws above. A unit that has never had automation
          // toggled has no "automation" key yet: that is "off", not an error
          // (it used to block saving a watering window on a fresh unit).
          if (!settings.settings) throw new Error('current automation settings unavailable; nothing saved');
          const auto = settings.settings.automation || { enabled: false };
          body.automation = { enabled: !!auto.enabled, watering_window: window };
        }
        if (minReservoir !== undefined) {
          const safety = (settings.settings || {}).safety;
          if (!safety) throw new Error('current safety settings unavailable; nothing saved');
          body.safety = { ...safety, min_reservoir_level: minReservoir };
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
    purge: (n) => actions.purge(n),
    stop: (n) => actions.stop(n),
  };
}

// One alert row for a relay held off its schedule: what, which way, by which
// screen, since when, and when (if ever) it lets go on its own.
const hhmm = (iso) => { const d = new Date(iso); return isNaN(d) ? '' : String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'); };
function overrideNotice(id, what, o, clock) {
  if (!o) return [];
  const where = o.actor === 'manual_touch' ? ' from the rack panel' : o.actor === 'manual_web' ? ' from the dashboard' : '';
  const since = o.since && hhmm(o.since) ? ` since ${hhmm(o.since)}` : '';
  const ends = o.until && hhmm(o.until) ? `Back to schedule at ${hhmm(o.until)}.` : 'Stays that way until switched to Auto.';
  return [{ id, ts: clock, shelf: '—', kind: 'Manual Override', sev: 'var(--ochre)', prio: 'ATTENTION',
    msg: `${what} forced ${o.mode}${where}${since}. ${ends}` }];
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
  const push = usePush(session);
  const authBridge = React.useMemo(() => ({
    SignInModal: LiveSignInModal,
    canManage,
    openOperators: () => setShowOperators(true),
  }), [canManage]);

  // audit 3.9: no "Reservoir Low" from a stale probe — a sensor-offline alert instead.
  const allAlerts = PG.deriveAlerts(state.shelves, state.reservoirOnline ? state.reservoir : 100, state.clock, state.reservoirFloor)
    .concat(state.ready && !state.reservoirOnline
      ? [{ id: 'RESOFF', ts: state.clock, shelf: '—', kind: 'Sensor Fault', sev: 'var(--oxblood)',
          prio: 'ACTION REQUIRED', msg: 'Reservoir level sensor not reporting — refill guard is blind.' }]
      : [])
    // 2026-11-17: a forced relay is said out loud. Lights sat forced On from the
    // panel all evening and only the panel showed it.
    .concat(overrideNotice('OVL', 'Lights', state.lightOverride, state.clock))
    .concat(overrideNotice('OVF', 'Fans', state.fanOverride, state.clock))
  ;
  // Batches tab: the live API plus the shelves a batch can sit on.
  const shelfKeys = state.shelves.map((s) => s.zoneKey + ':' + s.n).join(',');
  const batchStore = React.useMemo(() => ({
    list: api.batches, get: api.batch, create: api.createBatch, patch: api.patchBatch, log: api.logBatch,
    exportUrls: { batches: exportUrl('batches'), log: exportUrl('log') },
    shelves: state.shelves.map((s) => ({ key: s.zoneKey, name: 'Shelf ' + Number(s.n) })),
  }), [shelfKeys]);
  const alerts = allAlerts.filter((a) => !acked[a.id]);
  // 2026-11-17: an acknowledgement covers the alert that was showing. Once
  // that condition clears, forget it, so the next dry spell alerts again
  // (acks used to last until the page was reloaded).
  const liveIds = allAlerts.map((a) => a.id).sort().join(',');
  React.useEffect(() => {
    setAcked((a) => {
      const keep = {};
      liveIds.split(',').forEach((id) => { if (id && a[id]) keep[id] = true; });
      return Object.keys(keep).length === Object.keys(a).length ? a : keep;
    });
  }, [liveIds]);

  // 2026-11-15 review: key on what the charts depend on, not on the shelves
  // array identity (new every refresh) — otherwise each refresh restarted the
  // series chain and on a slow link the Environmental tab never rendered.
  const shelvesRef = React.useRef(state.shelves);
  shelvesRef.current = state.shelves;
  const seriesKey = state.shelves.map((s) => `${s.zoneKey}:${s.sensorKey}:${s.active ? 1 : 0}:${s.lo}-${s.hi}`).join('|');
  const getSeries = React.useCallback(
    (period) => getEnvSeries(period, shelvesRef.current),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [seriesKey],
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
      <div style={{ fontFamily: 'JetBrains Mono, monospace', padding: 40, color: '#D3CCB3',
        background: '#2E4A46', minHeight: '100vh', letterSpacing: '0.1em' }}>
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
        latched={state.latched}
        reservoirFloor={state.reservoirFloor}
        loadHistory={async (hours) => buildLog(await api.eventsSince(new Date(Date.now() - hours * 3600e3).toISOString(), 500))}
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
        push={push}
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
        batchStore={batchStore}
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

document.body.classList.add('console');
document.body.style.background = '#2E4A46';
document.body.style.margin = '0';
ReactDOM.createRoot(document.getElementById('root')).render(<App />);
