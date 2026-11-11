// pgui-controls.jsx — the Controls tab: the whole rack from one screen.
//
// Moisture bounds are the probe's full 0–100 % — a cactus shelf is allowed
// to sit at zero; the only rule is re-arm above trigger (the backend's).
// Three rack-wide panels (watering automation, lighting, fans) and one
// per-shelf panel (moisture rule + soak/drain + manual water/drain/stop).
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

function Slider({ label, sub, value, min, max, step = 1, unit, onChange, disabled }){
  return (
    <div className="ctrl-row">
      <div className="cl">{label}{sub && <small>{sub}</small>}</div>
      <div style={{display:'flex',alignItems:'center',gap:16}}>
        <input className="rng" type="range" min={min} max={max} step={step} value={value} disabled={disabled} onChange={e=>onChange(+e.target.value)}/>
        <div className="cv">{value}<span>{unit}</span></div>
      </div>
    </div>
  );
}

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

// ---------- per-shelf panel ----------

function ShelfPanel({ list, shelf, setShelf, form, canEdit, canManual, onApply, onWater, onDrain, onStop }){
  const [scope, setScope] = React.useState('this');
  const [confirm, setConfirm] = React.useState(null); // 'apply' | 'water'
  const cur = list.find(x=>x.n===shelf) || {};
  const v = form.v;
  const valid = v.rearm > v.trigger && v.durMin >= 1 && v.coolH >= 1 && v.maxCycles >= 1;
  const cycle = cur.cycle ? `${cur.cycle.state} · ${Math.round((cur.cycle.duration||0)/60)} min` : null;
  const soakTotal = v.durMin + v.drainMin;
  return (
    <div className="panel">
      <div className="ph">
        <div className="pt">Shelf {shelf}{cur.species ? <span className="ctrl-sp"> · {cur.species}</span> : null}</div>
        <div className="seg ctrl-shelfseg">
          {list.map(x=>(<button key={x.n} className={x.n===shelf?'on':''} onClick={()=>setShelf(x.n)}>{x.n}</button>))}
        </div>
      </div>
      <div className="pb">
        <div className="ctrl-grid2">
          <div>
            <Slider label="Water below" sub="Soak triggers under this probe reading" value={v.trigger} min={0} max={100} unit="%" onChange={x=>form.set('trigger',x)} disabled={!canEdit}/>
            <Slider label="Re-arm above" sub="Shelf may trigger again once it reads this" value={v.rearm} min={0} max={100} unit="%" onChange={x=>form.set('rearm',x)} disabled={!canEdit}/>
            <Slider label="Soak" sub="Pump runs the whole time" value={v.durMin} min={1} max={30} unit="min" onChange={x=>form.set('durMin',x)} disabled={!canEdit}/>
            <Slider label="Drain back" sub="Valve held open after the pump stops" value={v.drainMin} min={0} max={60} unit="min" onChange={x=>form.set('drainMin',x)} disabled={!canEdit}/>
          </div>
          <div>
            <Slider label="Minimum gap" sub="Rest between soaks" value={v.coolH} min={1} max={72} unit="h" onChange={x=>form.set('coolH',x)} disabled={!canEdit}/>
            <Slider label="Soaks per day" sub="Rolling 24 h cap" value={v.maxCycles} min={1} max={6} unit="/day" onChange={x=>form.set('maxCycles',x)} disabled={!canEdit}/>
            <div className="ctrl-row"><div className="cl">Full cycle<small>Soak + drain, one shelf at a time</small></div><div className="cv">{soakTotal}<span>min</span></div></div>
            <div className="ctrl-row">
              <div className="cl">Now<small>{cycle || (cur.probe ? `Probe ${cur.moisture}% · ${cur.status || ''}` : (cur.status || 'No probe reading'))}</small></div>
              <div className="ctrl-btns">
                <button className="fx-btn" disabled={!canManual || !!cur.cycle} onClick={()=>setConfirm('water')}>Water now</button>
                <button className="fx-btn" disabled={!canManual || !!cur.cycle} onClick={()=>onDrain(shelf)}>Drain</button>
                <button className="fx-btn danger" disabled={!canManual || !cur.cycle} onClick={()=>onStop(shelf)}>Stop</button>
              </div>
            </div>
          </div>
        </div>
        {v.rearm <= v.trigger && <div className="fx-warn">Re-arm must be above the trigger</div>}
        <div className="apply-bar">
          <span className="mono ctrl-k">Apply to</span>
          <div className="seg">
            <button className={scope==='this'?'on':''} onClick={()=>setScope('this')} disabled={!canEdit}>Shelf {shelf}</button>
            <button className={scope==='all'?'on':''} onClick={()=>setScope('all')} disabled={!canEdit}>All shelves</button>
          </div>
          {form.err && <span className="fx-warn">{form.err}</span>}
          <button className="fx-btn primary" style={{marginLeft:'auto'}} disabled={!canEdit||!form.dirty||!valid||form.saving} onClick={()=>setConfirm('apply')}>
            {form.saving ? 'Saving…' : form.saved ? 'Saved ✓' : 'Review & Apply'}
          </button>
        </div>
      </div>
      {confirm==='apply' && <Confirm title="Apply shelf rule"
        body={`Apply to ${scope==='all' ? 'all shelves' : 'Shelf '+shelf}: water below ${v.trigger}%, re-arm above ${v.rearm}%, ${v.durMin} min soak with the pump running, ${v.drainMin} min drain back, at least ${v.coolH} h between soaks, at most ${v.maxCycles} per day.`}
        confirmLabel="Apply" onCancel={()=>setConfirm(null)} onConfirm={()=>{ setConfirm(null); onApply(scope, shelf, form); }}/>}
      {confirm==='water' && <Confirm title={`Water Shelf ${shelf} now`}
        body={`Run a full ${v.durMin} min soak on Shelf ${shelf} with the pump on, then drain back for ${v.drainMin} min? It runs now, outside the watering window.`}
        confirmLabel="Water now" onCancel={()=>setConfirm(null)} onConfirm={()=>{ setConfirm(null); onWater(shelf); }}/>}
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
    list: [{n:'01',species:'Bald Cypress',active:true,probe:true,moisture:20,status:'Healthy'},{n:'02',species:'Buttonbush',active:true,probe:true,moisture:18,status:'Monitoring'},{n:'03',species:'Texas Mountain Laurel',active:true,probe:true,moisture:19,status:'Healthy'}],
    live: s.live,
    loadShelf: async (n)=> s.rules[n] || { trigger:16, rearm:24, durMin:17, drainMin:10, coolH:12, maxCycles:1 },
    applyShelf: async (scope, n, irr)=>{ (scope==='all'?['01','02','03']:[n]).forEach(k=>{ s.rules[k]={...irr}; }); },
    loadRack: async ()=>({ lights:s.lights, fans:s.fans, window:s.window, minReservoir:s.minReservoir }),
    applyRack: async (p)=>{ Object.assign(s, p); if(p.window!==undefined) s.live.automation={...s.live.automation, window:p.window}; bump(x=>x+1); },
    manualLight: (m)=>{ s.live.lightOverride = m==='auto'?null:{mode:m,until:null}; s.live.lightOn = m==='on'; bump(x=>x+1); },
    manualFan: (m)=>{ s.live.fanOverride = m==='auto'?null:{mode:m,until:null}; s.live.fanOn = m==='on'; bump(x=>x+1); },
    setAutomation: (e)=>{ s.live.automation={...s.live.automation, enabled:e}; bump(x=>x+1); },
    setMaintenance: (e)=>{ s.live.maintenance=e; bump(x=>x+1); },
    water: ()=>{}, drain: ()=>{}, stop: ()=>{},
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
  const shelfList = c.list && c.list.length ? c.list : [{n:'01'},{n:'02'},{n:'03'}];
  const [shelf, setShelf] = React.useState(shelfList[0].n);
  const rule = useForm({ trigger:16, rearm:24, durMin:17, drainMin:10, coolH:12, maxCycles:1 });

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

  React.useEffect(()=>{
    let alive = true;
    c.loadShelf(shelf).then(p=>{ if(alive && p) rule.load(p); }).catch(()=>{});
    return ()=>{ alive=false; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shelf, c]);

  const saveSched = (which) => (form) => form.save(v => c.applyRack({ [which]: { enabled: v.enabled, on: v.on, off: v.off } }));
  const saveAuto = (form) => form.save(v => c.applyRack({
    window: (v.start && v.end) ? { start: v.start, end: v.end } : { start:'', end:'' },
    minReservoir: v.minReservoir,
  }));
  const applyShelf = (scope, n, form) => form.save(v => c.applyShelf(scope, n, v));

  return (
    <div>
      <div className="db-h">
        <div className="ttl">Rack Controls</div>
        <div className="sub">Schedules, automation and every shelf, from here</div>
      </div>
      {readonly && <div className="ro-note">Read-only — sign in as a Grower to run things by hand, or as a Steward to change schedules and rules.</div>}
      {!readonly && !canEdit && <div className="ro-note">Grower: manual controls are live. Schedules, rules and settings need a Steward.</div>}
      <div className="ctrl-grid3">
        <AutomationPanel form={auto} live={live} canEdit={canEdit} onSave={saveAuto}
          onSetAutomation={c.setAutomation} onSetMaintenance={c.setMaintenance}/>
        <SchedulePanel title="Lighting" what="Lights" form={lights} live={{on:live.lightOn, override:live.lightOverride}}
          canEdit={canEdit} canManual={canManual} onManual={c.manualLight} onSave={saveSched('lights')}/>
        <SchedulePanel title="Circulation fans" what="Fans" form={fans} live={{on:live.fanOn, override:live.fanOverride}}
          canEdit={canEdit} canManual={canManual} onManual={c.manualFan} onSave={saveSched('fans')}/>
      </div>
      <ShelfPanel list={shelfList} shelf={shelf} setShelf={setShelf} form={rule} canEdit={canEdit} canManual={canManual}
        onApply={applyShelf} onWater={c.water} onDrain={c.drain} onStop={c.stop}/>
    </div>
  );
}

export { ControlsTab };
