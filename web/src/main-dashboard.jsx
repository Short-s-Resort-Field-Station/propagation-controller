// Surface B — the paper control center, served at "/".
import React from 'react';
import ReactDOM from 'react-dom/client';
import './design/pgui.css';
import { PG } from './design/data.jsx';
import { Dashboard } from './design/dashboard.jsx';
import { useUnitState } from './live/useUnitState.js';
import { getEnvSeries } from './live/series.js';
import { api } from './live/api.js';
import { LiveSignInModal, SetupModal, OperatorsModal, toDesignSession } from './live/auth-ui.jsx';

const pad2 = (n) => String(n).padStart(2, '0');

// Live Controls-tab bridge: loads real per-zone moisture rules + the rack
// photoperiod; Apply writes them back through the API.
function makeControls(state, actions) {
  const list = state.shelves.filter((s) => s.active)
    .map((s) => ({ n: s.n, zoneKey: s.zoneKey }));
  const zoneFor = (n) => (list.find((x) => x.n === n) || {}).zoneKey || 'zone_' + Number(n);

  return {
    list,
    lightOn: state.shelves.length ? state.shelves[0].light : false,
    manualLight: (on) => actions.lights(on),
    load: async (n) => {
      const zoneKey = zoneFor(n);
      const [mc, sch] = await Promise.all([
        api.moistureControl(zoneKey).catch(() => null),
        api.lightingSchedule().catch(() => null),
      ]);
      const rule = mc && mc.trigger_below != null ? mc : null;
      const on = sch ? parseInt(sch.on_time, 10) : 6;
      const off = sch ? parseInt(sch.off_time, 10) : 19;
      return {
        target: rule ? Math.round(rule.recovered_above ?? rule.trigger_below + 6) : 70,
        min: rule ? Math.round(rule.trigger_below) : 58,
        durMin: rule ? Math.max(1, Math.round(rule.run_duration_seconds / 60)) : 5,
        cool: rule ? Math.max(1, Math.round(rule.minimum_interval_minutes / 60)) : 4,
        on,
        hours: ((off - on + 24) % 24) || 13,
      };
    },
    apply: async (scope, n, irr, photo) => {
      const targets = scope === 'all' ? list.map((x) => x.zoneKey) : [zoneFor(n)];
      for (const zoneKey of targets) {
        const existing = await api.moistureControl(zoneKey).catch(() => null);
        const rule = existing && existing.trigger_below != null ? existing : null;
        await api.putMoistureControl(zoneKey, {
          enabled: rule ? rule.enabled : true,
          sensor_key: (rule && rule.sensor_key) || `soil_${zoneKey}_moisture`,
          trigger_below: irr.min,
          recovered_above: irr.target,
          run_duration_seconds: irr.durMin * 60,
          minimum_interval_minutes: irr.cool * 60,
          max_cycles_per_day: (rule && rule.max_cycles_per_day) || 4,
          sensor_offline_behavior: (rule && rule.sensor_offline_behavior) || 'lockout',
        });
      }
      const sch = await api.lightingSchedule().catch(() => null);
      await api.putLightingSchedule({
        enabled: true,
        timezone: (sch && sch.timezone) || 'America/Chicago',
        on_time: pad2(photo.on) + ':00',
        off_time: pad2((photo.on + photo.hours) % 24) + ':00',
        days: (sch && sch.days) || [],
      });
    },
  };
}

function App() {
  const [state, actions] = useUnitState('manual_web');
  const [session, setSession] = React.useState(undefined); // undefined = loading
  const [setupRequired, setSetupRequired] = React.useState(false);
  const [showOperators, setShowOperators] = React.useState(false);
  const [acked, setAcked] = React.useState({});

  React.useEffect(() => {
    api.session()
      .then((r) => { setSession(toDesignSession(r.user)); setSetupRequired(r.setup_required); })
      .catch(() => setSession(null));
  }, []);

  const signOut = () => { api.logout().catch(() => {}); setSession(null); };
  const canManage = !!(session && session.role === 'Steward');
  const authBridge = React.useMemo(() => ({
    SignInModal: LiveSignInModal,
    canManage,
    openOperators: () => setShowOperators(true),
  }), [canManage]);

  const alerts = PG.deriveAlerts(state.shelves, state.reservoir, state.clock)
    .filter((a) => !acked[a.id]);

  const getSeries = React.useCallback(
    (period) => getEnvSeries(period, state.shelves),
    [state.shelves],
  );
  const controls = React.useMemo(
    () => makeControls(state, actions),
    [state.shelves, actions],
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
        rackStatus={state.rackStatus}
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
