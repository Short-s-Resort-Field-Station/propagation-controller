// pgui-controls.jsx — the Controls tab: the whole rack from one screen.
//
// Three rack-wide panels (watering automation, lighting, fans) and a row of
// per-shelf action buttons (water now / drain / stop). Single source of
// truth: every rack-wide setting lives here and nowhere else; every
// per-shelf setting lives in Shelf Management's edit modal and nowhere else.
// With a `controls` bridge (the live build, main-dashboard.jsx) every field
// loads from the daemon and every button hits the API; without it the tab
// runs on an in-memory demo bridge so the handoff figure still works.
//
// Roles follow the backend (spec §20.2): Grower = manual actions (lights,
// fans, water/drain/stop); Steward = schedules, rules and settings.
import React from 'react';
import { Confirm } from './flows.jsx';

const pad2 = (n) => String(n).padStart(2, '0');
const fmtClockIso = (iso) => { const d = new Date(iso); return isNaN(d) ? '' : pad2(d.getHours()) + ':' + pad2(d.getMinutes()); };
const validHHMM = (v) => /^\d\d:\d\d$/.test(v || '');

// ---------- small parts ----------

function TimeRow({ label, sub, value, onChange, disabled }){
  return (
    <div className="ctrl-row">
      <div className="cl">{label}{sub && <small>{sub}</small>}</div>
      <input className="ctrl-time" type="time" step="60" value={value || ''} disabled={disabled} onChange={e=>onChange(e.target.value)}/>
    </div>
  );
}

// On / Off / Auto for a scheduled relay. `override` is the live override or
// null (= following the schedule).
function ModeButtons({ override, onSet, disabled }){
  const cur = override ? override.mode : 'auto';
  return (
    <div className="seg">
      {['on','off','auto'].map(m=>(
        <button key={m} className={cur===m?'on':''} disabled={disabled} onClick={()=>onSet(m)}>{m}</button>
      ))}
    </div>
  );
}

function SaveRow({ dirty, valid, saving, saved, err, onSave, disabled, label='Save' }){
  return (
    <div className="ctrl-save">
      {err && <span className="fx-warn">{err}</span>}
      {!valid && dirty && <span className="fx-warn">Check the fields</span>}
      <button className="fx-btn primary" disabled={disabled||!dirty||!valid||saving} onClick={onSave}>
        {saving ? 'Saving…' : saved ? 'Saved ✓' : label}
      </button>
    </div>
  );
}

// Load-once-then-edit form state with dirty tracking.
function useForm(initial){
  const [v, setV] = React.useState(initial);
  const [dirty, setDirty] = React.useState(false);
  const [saved, setSaved] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [err, setErr] = React.useState('');
  const load = (next) => { setV(next); setDirty(false); setSaved(false); setErr(''); };
  const set = (k, x) => { setV(p=>({...p,[k]:x})); setDirty(true); setSaved(false); };
  const save = async (fn) => {
    setSaving(true); setErr('');
    try { await fn(v); setDirty(false); setSaved(true); }
    catch(e){ setErr((e && e.message) || 'save failed'); }
    finally { setSaving(false); }
  };
  return { v, set, load, dirty, saved, saving, err, save };
}

// ---------- rack-wide panels ----------

function SchedulePanel({ title, what, form, live, canEdit, canManual, onManual, onSave }){
  const valid = validHHMM(form.v.on) && validHHMM(form.v.off) && form.v.on !== form.v.off;
  const status = live.override
    ? `Forced ${live.override.mode}${live.override.until ? ' until ' + fmtClockIso(live.override.until) : ' until cleared'}`
    : form.v.enabled ? `Following schedule · relay ${live.on ? 'on' : 'off'}` : `Schedule disabled · relay ${live.on ? 'on' : 'off'}`;
  return (
    <div className="panel">
      <div className="ph"><div className="pt">{title}</div><div className="pm">Rack-wide</div></div>
      <div className="pb">
        <div className="ctrl-row">
          <div className="cl">Schedule<small>{form.v.enabled ? 'Runs every day' : 'Off — relay only moves by hand'}</small></div>
          <div className="seg">
            <button className={form.v.enabled?'on':''} disabled={!canEdit} onClick={()=>form.set('enabled',true)}>On</button>
            <button className={!form.v.enabled?'on':''} disabled={!canEdit} onClick={()=>form.set('enabled',false)}>Off</button>
          </div>
        </div>
        <TimeRow label={`${what} on at`} value={form.v.on} onChange={v=>form.set('on',v)} disabled={!canEdit}/>
        <TimeRow label={`${what} off at`} sub={form.v.on && form.v.off && form.v.off < form.v.on ? 'Spans midnight' : undefined} value={form.v.off} onChange={v=>form.set('off',v)} disabled={!canEdit}/>
        <SaveRow dirty={form.dirty} valid={valid} saving={form.saving} saved={form.saved} err={form.err} disabled={!canEdit} onSave={()=>onSave(form)}/>
        <div className="ctrl-row" style={{marginTop:4}}>
          <div className="cl">Manual<small>{status}</small></div>
          <ModeButtons override={live.override} onSet={onManual} disabled={!canManual}/>
        </div>
        <div className="ctrl-note">On / Off hold for two hours, then the schedule takes back over. Auto clears them now.</div>
      </div>
    </div>
  );
}

function AutomationPanel({ form, live, canEdit, onSave, onSetAutomation, onSetMaintenance }){
  const [confirm, setConfirm] = React.useState(null); // 'auto-on' | 'auto-off' | 'maint-on' | 'maint-off'
  const hasWindow = !!(form.v.start || form.v.end);
  const valid = (!hasWindow || (validHHMM(form.v.start) && validHHMM(form.v.end) && form.v.start !== form.v.end))
    && form.v.minReservoir >= 5 && form.v.minReservoir <= 60;
  const auto = live.automation || { enabled:false, window:null };
  const bodies = {
    'auto-on': ['Start watering automation', 'Moisture-triggered soaks will run on every enabled shelf, one shelf at a time, inside the watering window. Make sure every probe is seated in media first.', 'Start'],
    'auto-off': ['Stop watering automation', 'No soak will fire from the moisture rules until it is switched back on. Lights and fans keep their schedules; manual watering still works.', 'Stop'],
    'maint-on': ['Enter maintenance', 'Suspends irrigation automation while you work on the rack. Lighting and fans keep running; you can still water a shelf by hand.', 'Enter maintenance'],
    'maint-off': ['Leave maintenance', 'Irrigation automation resumes according to the automation switch above.', 'Resume'],
  };
  return (
    <div className="panel">
      <div className="ph"><div className="pt">Watering automation</div><div className="pm">Rack-wide</div></div>
      <div className="pb">
        <div className="ctrl-row">
          <div className="cl">Automation<small>{auto.enabled ? 'Running — soaks fire from the moisture rules' : 'Stopped — nothing waters by itself'}</small></div>
          <div className="seg">
            <button className={auto.enabled?'on':''} disabled={!canEdit} onClick={()=>!auto.enabled && setConfirm('auto-on')}>On</button>
            <button className={!auto.enabled?'on':''} disabled={!canEdit} onClick={()=>auto.enabled && setConfirm('auto-off')}>Off</button>
          </div>
        </div>
        <div className="ctrl-row">
          <div className="cl">Maintenance<small>{live.maintenance ? 'On — automation suspended for rack work' : 'Off — normal operation'}</small></div>
          <div className="seg">
            <button className={live.maintenance?'on':''} disabled={!canEdit} onClick={()=>!live.maintenance && setConfirm('maint-on')}>On</button>
            <button className={!live.maintenance?'on':''} disabled={!canEdit} onClick={()=>live.maintenance && setConfirm('maint-off')}>Off</button>
          </div>
        </div>
        <TimeRow label="Watering window opens" sub="Soaks may start from" value={form.v.start} onChange={v=>form.set('start',v)} disabled={!canEdit}/>
        <TimeRow label="Watering window closes" sub={hasWindow ? 'No new soak after this' : 'Blank both for no window'} value={form.v.end} onChange={v=>form.set('end',v)} disabled={!canEdit}/>
        <div className="ctrl-row">
          <div className="cl">Reservoir cutoff<small>Pump locks out below this level</small></div>
          <div style={{display:'flex',alignItems:'center',gap:10}}>
            <input className="ctrl-num" type="number" min={5} max={60} value={form.v.minReservoir} disabled={!canEdit} onChange={e=>form.set('minReservoir',+e.target.value)}/>
            <span className="cv" style={{minWidth:0}}><span>%</span></span>
          </div>
        </div>
        <SaveRow dirty={form.dirty} valid={valid} saving={form.saving} saved={form.saved} err={form.err} disabled={!canEdit} onSave={()=>onSave(form)}/>
        <div className="ctrl-note">One shelf waters at a time; the pump runs for the whole soak, then the tray drains back.{live.lockouts && live.lockouts.length ? ' Lockouts: ' + live.lockouts.join(', ') + '.' : ''}</div>
      </div>
      {confirm && <Confirm title={bodies[confirm][0]} body={bodies[confirm][1]} confirmLabel={bodies[confirm][2]} danger={confirm==='auto-off'}
        onCancel={()=>setConfirm(null)}
        onConfirm={()=>{ const c=confirm; setConfirm(null);
          if(c==='auto-on') onSetAutomation(true); else if(c==='auto-off') onSetAutomation(false);
          else if(c==='maint-on') onSetMaintenance(true); else onSetMaintenance(false); }}/>}
    </div>
  );
}

// ---------- per-shelf actions ----------
// Actions only. A shelf's settings (band, soak, drain, gap, cap, species)
// are edited in one place — Shelf Management's edit modal — never here.

function ShelfActions({ list, canManual, onWater, onDrain, onPurge, onStop }){
  const [confirm, setConfirm] = React.useState(null); // shelf n
  const rows = list.filter(x=>x.active || x.assigned);
  const target = rows.find(x=>x.n===confirm);
  return (
    <div className="panel">
      <div className="ph"><div className="pt">Shelves</div><div className="pm">Water by hand · settings live in Shelf Management</div></div>
      <div className="pb" style={{paddingTop:4,paddingBottom:4}}>
        {rows.length===0 && <div className="ctrl-note">No shelf assigned yet.</div>}
        {rows.map(x=>{
          // 2026-11-13: a queued shelf says so and can be cancelled (Stop dequeues it).
          const now = x.cycle ? `${x.cycle.state} · ${Math.round((x.cycle.duration||0)/60)} min`
            : x.queued ? `${x.queued.kind} queued${x.queued.behind ? ` · after Shelf ${x.queued.behind}` : ''}`
            : (x.probe ? `Probe ${x.moisture}%` : 'No probe reading');
          const waiting = !!x.cycle || !!x.queued;
          const purgeSec = x.raw && x.raw.purgeSec ? x.raw.purgeSec : 0;
          return (
            <div className="ctrl-row" key={x.n}>
              <div className="cl"><b className="ctrl-shelfno">{x.n}</b> {x.species || 'Unassigned'}{x.latin ? <em className="ctrl-sp"> {x.latin}</em> : null}<small>{now}{x.status ? ' · '+x.status : ''}{!x.active ? ' · disabled' : ''}</small></div>
              <div className="ctrl-btns">
                <button className="fx-btn" disabled={!canManual || waiting || !x.active} onClick={()=>setConfirm(x.n)}>Water now</button>
                <button className="fx-btn" disabled={!canManual || waiting || !x.active || !purgeSec} title={purgeSec ? `Valve open ${purgeSec} s, pump off` : 'Purge is off for this shelf'} onClick={()=>onPurge && onPurge(x.n)}>Purge</button>
                <button className="fx-btn" disabled={!canManual || waiting || !x.active} onClick={()=>onDrain(x.n)}>Drain</button>
                <button className="fx-btn danger" disabled={!canManual || !waiting} onClick={()=>onStop(x.n)}>{x.queued && !x.cycle ? 'Cancel' : 'Stop'}</button>
              </div>
            </div>
          );
        })}
      </div>
      {target && <Confirm title={`Water Shelf ${target.n} now`}
        body={`Run a full soak on Shelf ${target.n}${target.durMin ? ` — ${target.durMin} min with the pump on, then ${target.drainMin||0} min drain back` : ''}? It runs now, outside the watering window, one shelf at a time.`}
        confirmLabel="Water now" onCancel={()=>setConfirm(null)} onConfirm={()=>{ const n=target.n; setConfirm(null); onWater(n); }}/>}
    </div>
  );
}

// ---------- demo bridge (standalone figure / preview) ----------

function useDemoControls(){
  const store = React.useRef({
    lights: { enabled:true, on:'07:30', off:'17:05' }, fans: { enabled:true, on:'13:30', off:'15:00' },
    window: { start:'07:45', end:'11:00' }, minReservoir: 20,
    rules: {}, live: { lightOn:true, lightOverride:null, fanOn:false, fanOverride:null, maintenance:false, automation:{enabled:false, window:{start:'07:45',end:'11:00'}}, lockouts:[], reservoir:48 },
  });
  const [, bump] = React.useState(0);
  const s = store.current;
  return React.useMemo(()=>({
    list: [{n:'01',species:'Bald Cypress',active:true,probe:true,moisture:20,status:'Healthy',durMin:17,drainMin:10},{n:'02',species:'Buttonbush',active:true,probe:true,moisture:18,status:'Monitoring',durMin:17,drainMin:10},{n:'03',species:'Texas Mountain Laurel',active:true,probe:true,moisture:19,status:'Healthy',durMin:17,drainMin:10}],
    live: s.live,
    loadRack: async ()=>({ lights:s.lights, fans:s.fans, window:s.window, minReservoir:s.minReservoir }),
    applyRack: async (p)=>{ Object.assign(s, p); if(p.window!==undefined) s.live.automation={...s.live.automation, window:p.window}; bump(x=>x+1); },
    manualLight: (m)=>{ s.live.lightOverride = m==='auto'?null:{mode:m,until:null}; s.live.lightOn = m==='on'; bump(x=>x+1); },
    manualFan: (m)=>{ s.live.fanOverride = m==='auto'?null:{mode:m,until:null}; s.live.fanOn = m==='on'; bump(x=>x+1); },
    setAutomation: (e)=>{ s.live.automation={...s.live.automation, enabled:e}; bump(x=>x+1); },
    setMaintenance: (e)=>{ s.live.maintenance=e; bump(x=>x+1); },
    water: ()=>{}, drain: ()=>{}, purge: ()=>{}, stop: ()=>{},
  }), [s, bump]);
}

// ---------- the tab ----------

function ControlsTab({ role, controls }){
  const demo = useDemoControls();
  const c = controls || demo;
  const readonly = !role || role==='Viewer';
  const canManual = !readonly;                 // Grower+
  const canEdit = role==='Steward';            // schedules, rules, settings
  const live = c.live || {};

  const lights = useForm({ enabled:true, on:'07:30', off:'17:05' });
  const fans = useForm({ enabled:true, on:'13:30', off:'15:00' });
  const auto = useForm({ start:'', end:'', minReservoir:20 });
  const shelfList = c.list || [];

  // Rack-wide values load once (and again after a save), never while editing.
  const loadRack = React.useCallback(()=>{
    c.loadRack().then(r=>{
      if(!r) return;
      if(r.lights) lights.load({ enabled: r.lights.enabled !== false, on: r.lights.on || '', off: r.lights.off || '' });
      if(r.fans) fans.load({ enabled: r.fans.enabled !== false, on: r.fans.on || '', off: r.fans.off || '' });
      auto.load({ start: r.window ? r.window.start : '', end: r.window ? r.window.end : '', minReservoir: r.minReservoir != null ? r.minReservoir : 20 });
    }).catch(()=>{});
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [c]);
  React.useEffect(()=>{ loadRack(); }, [loadRack]);

  const saveSched = (which) => (form) => form.save(v => c.applyRack({ [which]: { enabled: v.enabled, on: v.on, off: v.off } }));
  const saveAuto = (form) => form.save(v => c.applyRack({
    window: (v.start && v.end) ? { start: v.start, end: v.end } : { start:'', end:'' },
    minReservoir: v.minReservoir,
  }));

  return (
    <div>
      <div className="db-h">
        <div className="ttl">Rack Controls</div>
        <div className="sub">Rack-wide schedules and automation · water a shelf by hand</div>
      </div>
      {readonly && <div className="ro-note">Read-only — sign in as a Grower to run things by hand, or as a Steward to change schedules and settings.</div>}
      {!readonly && !canEdit && <div className="ro-note">Grower: manual controls are live. Schedules and settings need a Steward.</div>}
      <div className="ctrl-grid3">
        <AutomationPanel form={auto} live={live} canEdit={canEdit} onSave={saveAuto}
          onSetAutomation={c.setAutomation} onSetMaintenance={c.setMaintenance}/>
        <SchedulePanel title="Lighting" what="Lights" form={lights} live={{on:live.lightOn, override:live.lightOverride}}
          canEdit={canEdit} canManual={canManual} onManual={c.manualLight} onSave={saveSched('lights')}/>
        <SchedulePanel title="Circulation fans" what="Fans" form={fans} live={{on:live.fanOn, override:live.fanOverride}}
          canEdit={canEdit} canManual={canManual} onManual={c.manualFan} onSave={saveSched('fans')}/>
      </div>
      <ShelfActions list={shelfList} canManual={canManual} onWater={c.water} onDrain={c.drain} onPurge={c.purge} onStop={c.stop}/>
    </div>
  );
}

export { ControlsTab };
