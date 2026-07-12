import React from 'react';
import { PG } from './data.jsx';

// pgui-flows.jsx — the "missing flows" for the Propagation Unit:
// shelf assignment · auth (rack lock + session) · offline states ·
// alerts + escalation · event history + export · config-safety dialogs.
// All paper/instrument styling lives in pgui.css (.fx-* / .asg-* / .al-* / .rk-lock ...).

const fmtH = h => String(h).padStart(2,'0') + ':00';

// ---- small stepper (accepts value or functional updater via set) ----
// change 3: optional `step` (e.g. 0.1 for pH); integer behaviour when step=1.
function Stepper({ v, set, min, max, unit, step=1 }){
  const snap = (x)=> Math.round(x/step)*step;
  const fmt = (x)=> step<1 ? (Math.round(x*10)/10).toFixed(1) : x;
  return (
    <div className="fx-step">
      <button onClick={()=>set(x=>Math.max(min, snap(x-step)))} aria-label="decrease">&minus;</button>
      <div className="v">{fmt(v)}<small>{unit}</small></div>
      <button onClick={()=>set(x=>Math.min(max, snap(x+step)))} aria-label="increase">&#43;</button>
    </div>
  );
}

// change 3 · design pass — compact typed number field. Replaces the modal's
// stepper wall: calmer, fits two-up, type or use arrow keys. Clamps on blur so
// intermediate typing ("6"→"60") isn't fought; empty is tolerated until blur.
function NumField({ v, set, min, max, step=1, unit }){
  return (
    <span className="asg-range">
      <input type="number" className="asg-num" value={v} min={min} max={max} step={step}
        onChange={e=>{ const r=e.target.value; if(r===''){ set(''); return; } const n=step<1?parseFloat(r):parseInt(r,10); if(!isNaN(n)) set(n); }}
        onBlur={()=>set(x=>{ const n=Number(x); return isNaN(n)?min:Math.max(min,Math.min(max,n)); })}/>
      {unit && <span className="asg-unit">{unit}</span>}
    </span>
  );
}
// Low–high pair, e.g. a moisture / pH / temperature target band.
function RangeField({ lo, hi, setLo, setHi, min, max, step=1, unit, ok=true }){
  return (
    <div>
      <div className="asg-range">
        <NumField v={lo} set={setLo} min={min} max={max} step={step}/>
        <span className="asg-dash">to</span>
        <NumField v={hi} set={setHi} min={min} max={max} step={step}/>
        {unit && <span className="asg-unit">{unit}</span>}
      </div>
      {!ok && <div className="fx-warn">Low must be below high</div>}
    </div>
  );
}

// ---- shared PIN pad (paper + dark variants), renders its own dots ----
function PinPad({ value, onKey, dark }){
  const keys = ['1','2','3','4','5','6','7','8','9','clr','0','del'];
  return (
    <div className={dark ? 'rk-pin' : 'pin'}>
      <div className="dots">{[0,1,2,3].map(i => <i key={i} className={i < value.length ? 'on' : ''}></i>)}</div>
      {keys.map(k => k==='clr'
        ? <button key={k} onClick={()=>onKey('clr')} style={{fontSize:'12px',letterSpacing:'0.08em'}}>CLR</button>
        : k==='del'
        ? <button key={k} onClick={()=>onKey('del')} style={{fontSize:'17px'}}>&#9003;</button>
        : <button key={k} onClick={()=>onKey(k)}>{k}</button>)}
    </div>
  );
}

/* ============ FLOW 1 · SHELF ASSIGNMENT / QUICK EDIT ============ */

// PhotoRail — photoperiod visual. Windows may wrap past midnight.
function PhotoRail({ onH, hours }){
  const startPct=(onH/24)*100;
  const w1=Math.min(hours, 24-onH)/24*100;
  const wrap=Math.max(0, hours-(24-onH))/24*100;
  const sunPos=(((onH+hours/2)%24)/24)*100;
  return (
    <div className="photo-rail">
      <div className="photo-track">
        <div className="lit" style={{left:startPct+'%',width:w1+'%'}}></div>
        {wrap>0 && <div className="lit" style={{left:0,width:wrap+'%'}}></div>}
        <div className="sun" style={{left:sunPos+'%'}}></div>
      </div>
      <div className="photo-scale"><span>00:00</span><span>06:00</span><span>12:00</span><span>18:00</span><span>24:00</span></div>
    </div>
  );
}

// change 3 · design pass: propagation-stage presets; free text allowed too.
const STAGE_PRESETS = ['Seeds','Seedlings','Cuttings','Trimmings','Suckers','Saplings'];

// Assignment and quick-edit share this modal, laid out in labeled sections:
// Specimen & Profile · Watering · Monitoring targets · Lighting. Photoperiod is
// "lights on at + hours of light" so on/off can never add up to more than a day;
// a shelf may carry a second specimen. With a `profiles` bridge ({list,save,
// update}) the modal also picks/saves species profiles and edits the pH/temp
// monitoring happy-bands.
function AssignModal({ shelfN, onAssign, onClose, initial, onClear, onDisable, profiles }){
  const cat = PG.SPECIES;
  const findIdx = (name)=>{ const k=cat.findIndex(s=>s.species===name); return k<0?0:k; };
  const [i, setI] = React.useState(initial && initial.species ? findIdx(initial.species) : 0);
  const [j, setJ] = React.useState(initial && initial.species2 ? findIdx(initial.species2) : -1);
  const sp = cat[i];
  const sp2 = j>=0 ? cat[j] : null;
  const [lo,setLo]   = React.useState(initial?initial.lo:sp.lo);
  const [hi,setHi]   = React.useState(initial?initial.hi:sp.hi);
  const [durMin,setDurMin] = React.useState(initial && initial.durMin ? initial.durMin : 5);
  // live-edit (ebb-flow): drain-back time — valve holds open after the pump stops.
  const [drainMin,setDrainMin] = React.useState(initial && initial.drainMin!=null ? initial.drainMin : 5);
  const [onH,setOnH] = React.useState(initial && initial.onH!=null ? initial.onH : sp.onH);
  const [lightHrs,setLightHrs] = React.useState(
    initial && initial.lightHrs!=null ? initial.lightHrs : Math.max(1,(sp.offH-sp.onH+24)%24));
  // change 3: monitoring-only pH / temperature happy-bands + carried moisture
  // guards (cooldown / daily cap) so a saved profile round-trips fully.
  const [phLo,setPhLo] = React.useState(initial && initial.phLo!=null ? initial.phLo : sp.phLo);
  const [phHi,setPhHi] = React.useState(initial && initial.phHi!=null ? initial.phHi : sp.phHi);
  const [tempLo,setTempLo] = React.useState(initial && initial.tempLo!=null ? initial.tempLo : sp.tempLo);
  const [tempHi,setTempHi] = React.useState(initial && initial.tempHi!=null ? initial.tempHi : sp.tempHi);
  const [coolMin,setCoolMin] = React.useState(initial && initial.coolMin!=null ? initial.coolMin : 240);
  const [maxCycles,setMaxCycles] = React.useState(initial && initial.maxCycles!=null ? initial.maxCycles : 4);
  const [profList,setProfList] = React.useState([]);
  const [profId,setProfId] = React.useState(initial && initial.profileId ? initial.profileId : 0);
  const [stage,setStage] = React.useState(initial && initial.stage ? initial.stage : '');
  const [customStage,setCustomStage] = React.useState(!!(initial && initial.stage && !STAGE_PRESETS.includes(initial.stage)));
  const [profName,setProfName] = React.useState('');
  const [showSave,setShowSave] = React.useState(false);
  const [saveMsg,setSaveMsg] = React.useState('');

  React.useEffect(()=>{
    let alive=true;
    if(profiles && profiles.list && sp.id){
      profiles.list(sp.id).then(list=>{ if(alive) setProfList(list||[]); }).catch(()=>{ if(alive) setProfList([]); });
    } else setProfList([]);
    return ()=>{ alive=false; };
  }, [sp.id, profiles]);

  const applyProfile = (p)=>{
    setProfId(p.id);
    setStage(p.stage||''); setCustomStage(!!(p.stage && !STAGE_PRESETS.includes(p.stage)));
    const s = p.settings||{};
    if(s.moisture){
      if(s.moisture.trigger_below!=null) setLo(Math.round(s.moisture.trigger_below));
      if(s.moisture.recovered_above!=null) setHi(Math.round(s.moisture.recovered_above));
      if(s.moisture.run_duration_seconds!=null) setDurMin(Math.max(1,Math.round(s.moisture.run_duration_seconds/60)));
      if(s.moisture.drain_seconds!=null) setDrainMin(Math.max(0,Math.round(s.moisture.drain_seconds/60)));
      if(s.moisture.minimum_interval_minutes!=null) setCoolMin(s.moisture.minimum_interval_minutes);
      if(s.moisture.max_cycles_per_day!=null) setMaxCycles(s.moisture.max_cycles_per_day);
    }
    if(s.ph){ setPhLo(s.ph.min); setPhHi(s.ph.max); }
    if(s.temp){ setTempLo(s.temp.min); setTempHi(s.temp.max); }
  };
  const onPickProfile = (id)=>{ if(!id){ setProfId(0); return; } const p=profList.find(x=>x.id===id); if(p) applyProfile(p); };
  const onPickStage = (v)=>{ if(v==='__custom'){ setCustomStage(true); } else { setCustomStage(false); setStage(v); } };

  const pick = k => { const s=cat[k]; setI(k); setLo(s.lo); setHi(s.hi); setOnH(s.onH); setLightHrs(Math.max(1,(s.offH-s.onH+24)%24));
    setPhLo(s.phLo); setPhHi(s.phHi); setTempLo(s.tempLo); setTempHi(s.tempHi); setProfId(0); };

  const bandOk = lo < hi, phOk = phLo < phHi, tempOk = tempLo < tempHi;
  const allOk = bandOk && phOk && tempOk;
  const offH = (onH+lightHrs)%24;
  const editing = !!initial;
  const appliedProf = profList.find(p=>p.id===profId);

  const settingsPayload = ()=>({
    moisture:{ trigger_below:lo, recovered_above:hi, run_duration_seconds:durMin*60, drain_seconds:drainMin*60,
      minimum_interval_minutes:coolMin, max_cycles_per_day:maxCycles },
    ph:{ min:phLo, max:phHi }, temp:{ min:tempLo, max:tempHi },
  });
  const refreshProfiles = ()=>{ if(profiles && profiles.list && sp.id) profiles.list(sp.id).then(l=>setProfList(l||[])).catch(()=>{}); };
  const doSaveNew = ()=>{
    if(!profiles || !profiles.save || !allOk) { if(!allOk) setSaveMsg('Fix the ranges first'); return; }
    const name = (profName||'').trim() || `${sp.species}${stage?' · '+stage:''}`;
    profiles.save({ species_id: sp.id, stage, name, settings: settingsPayload() })
      .then(p=>{ setProfId(p.id); setSaveMsg('Saved “'+name+'”'); setShowSave(false); refreshProfiles(); })
      .catch(e=>setSaveMsg(e.message||'save failed'));
  };
  const doUpdate = ()=>{
    if(!profiles || !profiles.update || !appliedProf || !allOk) { if(!allOk) setSaveMsg('Fix the ranges first'); return; }
    profiles.update(appliedProf.id, { name: appliedProf.name, stage, settings: settingsPayload() })
      .then(()=>{ setSaveMsg('Updated “'+appliedProf.name+'”'); setShowSave(false); refreshProfiles(); })
      .catch(e=>setSaveMsg(e.message||'update failed'));
  };

  const submit = () => { if(!allOk) return;
    onAssign(shelfN, { active:true, species:sp.species, tag:sp.tag,
      species2: sp2?sp2.species:'', tag2: sp2?sp2.tag:'',
      moisture:Math.round((lo+hi)/2), lo, hi, durMin, drainMin,
      phLo, phHi, tempLo, tempHi, minIntervalMin:coolMin, maxCycles,
      speciesId: sp.id, profileId: appliedProf?appliedProf.id:0, profileName: appliedProf?appliedProf.name:'', stage,
      light:true, on:fmtH(onH), off:fmtH(offH),
      lastWater:'—', usage:'—', status:'Healthy', stripe:sp.stripe });
  };
  return (
    <div className="fx-back" onClick={onClose}><div className="fx-modal" style={{maxWidth:'640px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">{editing?'Edit Shelf':'Assign Specimen'}</span><span className="tag">Shelf {shelfN}</span><span className="x" onClick={onClose}>&times;</span></div>
      <div className="fx-body">

        {/* ── SPECIMEN & PROFILE ─────────────────────────────── */}
        <div className="asg-sec first">Specimen &amp; Profile</div>
        <div className="fx-2">
          <div className="fx-field">
            <div className="fx-k">Specimen · native catalog</div>
            <select className="fx-select" value={i} onChange={e=>pick(+e.target.value)}>
              {cat.map((s,k)=><option key={k} value={k}>{s.species}</option>)}
            </select>
            <div className="fx-latin"><em style={{textTransform:'capitalize'}}>{sp.tag.toLowerCase()}</em> &middot; {sp.hab}</div>
          </div>
          <div className="fx-field">
            <div className="fx-k">Second specimen · optional</div>
            <select className="fx-select" value={j} onChange={e=>setJ(+e.target.value)}>
              <option value={-1}>None — single specimen</option>
              {cat.map((s,k)=><option key={k} value={k}>{s.species}</option>)}
            </select>
            {sp2 && <div className="fx-latin"><em style={{textTransform:'capitalize'}}>{sp2.tag.toLowerCase()}</em> &middot; shares the shelf's watering &amp; band</div>}
          </div>
        </div>
        <div className="fx-2">
          <div className="fx-field" style={{marginBottom: profiles?18:0}}>
            <div className="fx-k">Propagation stage</div>
            <select className="fx-select" value={customStage ? '__custom' : stage} onChange={e=>onPickStage(e.target.value)}>
              <option value="">— none —</option>
              {STAGE_PRESETS.map(s=><option key={s} value={s}>{s}</option>)}
              <option value="__custom">Custom…</option>
            </select>
            {customStage && <input className="asg-input" style={{marginTop:8}} placeholder="Stage name (e.g. Air-layers)" value={stage} onChange={e=>setStage(e.target.value)}/>}
          </div>
          {profiles && (
            <div className="fx-field" style={{marginBottom:0}}>
              <div className="fx-k">Profile · saved for {sp.species}</div>
              <select className="fx-select" value={profId} onChange={e=>onPickProfile(+e.target.value)}>
                <option value={0}>Custom — settings below</option>
                {profList.map(p=><option key={p.id} value={p.id}>{(p.stage?p.stage+' · ':'')+p.name}</option>)}
              </select>
              <div className="fx-latin">{profList.length ? 'Applying a profile fills the settings below.' : 'No saved profiles yet — dial in settings and save one.'}</div>
            </div>
          )}
        </div>
        {/* change 3: save-as-profile lives right here, always visible */}
        {profiles && (
          !showSave
            ? <div style={{display:'flex',alignItems:'center',gap:12,flexWrap:'wrap',marginTop:12}}>
                <button className="fx-btn" onClick={()=>{ setSaveMsg(''); setProfName(appliedProf?appliedProf.name:`${sp.species}${stage?' · '+stage:''}`); setShowSave(true); }}>Save these settings as a profile ›</button>
                {saveMsg && <span className="fx-latin">{saveMsg}</span>}
              </div>
            : <div className="asg-savebar">
                <span className="lbl">Save {stage?stage+' · ':''}as</span>
                <input className="asg-input" value={profName} onChange={e=>setProfName(e.target.value)} placeholder="Profile name"/>
                <button className="fx-btn primary" onClick={doSaveNew}>Save new</button>
                {appliedProf && <button className="fx-btn" onClick={doUpdate}>Update “{appliedProf.name}”</button>}
                <button className="fx-btn" onClick={()=>setShowSave(false)}>Cancel</button>
                {saveMsg && <span className="fx-warn" style={{width:'100%'}}>{saveMsg}</span>}
              </div>
        )}

        {/* ── WATERING ───────────────────────────────────────── */}
        <div className="asg-sec">Watering</div>
        <div className="asg-grid3">
          <div className="fx-field" style={{marginBottom:0}}>
            <div className="fx-k">Moisture band · %VWC</div>
            <RangeField lo={lo} hi={hi} setLo={setLo} setHi={setHi} min={20} max={95} unit="%VWC" ok={bandOk}/>
          </div>
          <div className="fx-field" style={{marginBottom:0}}>
            <div className="fx-k">Soak</div>
            <NumField v={durMin} set={setDurMin} min={1} max={30} unit="min"/>
          </div>
          <div className="fx-field" style={{marginBottom:0}}>
            {/* live-edit (ebb-flow): drain-back — valve holds open, pump off */}
            <div className="fx-k">Drain-back</div>
            <NumField v={drainMin} set={setDrainMin} min={0} max={30} unit="min"/>
          </div>
        </div>
        <div className="asg-hint" style={{marginTop:12}}>Pump rests above the high mark, triggers below the low. Drain-back holds the valve open after the pump stops so the tray drains back.</div>

        {/* ── MONITORING TARGETS ─────────────────────────────── */}
        <div className="asg-sec">Monitoring targets · happy band</div>
        <div className="fx-2">
          <div className="fx-field" style={{marginBottom:0}}>
            <div className="fx-k">Soil pH</div>
            <RangeField lo={phLo} hi={phHi} setLo={setPhLo} setHi={setPhHi} min={3.5} max={9.5} step={0.1} ok={phOk}/>
          </div>
          <div className="fx-field" style={{marginBottom:0}}>
            <div className="fx-k">Soil temperature</div>
            <RangeField lo={tempLo} hi={tempHi} setLo={setTempLo} setHi={setTempHi} min={40} max={100} unit="°F" ok={tempOk}/>
          </div>
        </div>
        <div className="asg-hint" style={{marginTop:12}}>The unit monitors pH &amp; temperature but doesn’t control them — these bands drive the gauges and the time-in-target tracking.</div>

        {/* ── LIGHTING ───────────────────────────────────────── */}
        <div className="asg-sec">Lighting · rack-wide photoperiod</div>
        <div style={{display:'flex',alignItems:'flex-end',gap:'26px',marginBottom:'14px'}}>
          <div><div className="fx-k" style={{marginBottom:'6px'}}>Lights on at</div><NumField v={onH} set={setOnH} min={0} max={23} unit=":00"/></div>
          <div><div className="fx-k" style={{marginBottom:'6px'}}>Hours of light</div><NumField v={lightHrs} set={setLightHrs} min={1} max={23} unit="h"/></div>
          <div style={{marginLeft:'auto',fontFamily:'Zilla Slab',fontWeight:900,fontSize:'22px'}}>{fmtH(onH)}–{fmtH(offH)}<span style={{fontSize:'12px',color:'var(--char)'}}> on–off</span></div>
        </div>
        <PhotoRail onH={onH} hours={lightHrs}/>
      </div>
      <div className="fx-foot">
        {/* change 2: clearing the bay auto-disables sensors + valve; disable
            pauses without clearing the specimen. Both are logged. */}
        {editing && onClear && <button className="fx-btn danger" onClick={()=>onClear(shelfN)}>Clear Bay</button>}
        {editing && onDisable && <button className="fx-btn" onClick={()=>onDisable(shelfN)}>Disable Shelf</button>}
        <span className="note"></span>
        <button className="fx-btn" onClick={onClose}>Cancel</button>
        <button className="fx-btn primary" onClick={submit} disabled={!allOk}>{editing?'Save Changes':'Assign & Activate'}</button>
      </div>
    </div></div>
  );
}

/* ============ FLOW 2 · AUTH ============ */
// dashboard sign-in
function SignInModal({ onSignIn, onClose }){
  const ops = PG.OPERATORS;
  const [oi,setOi]=React.useState(0);
  const [pin,setPin]=React.useState('');
  const [err,setErr]=React.useState(false);
  const op=ops[oi];
  const key=k=>{ setErr(false); if(k==='del')setPin(p=>p.slice(0,-1)); else if(k==='clr')setPin(''); else if(pin.length<4)setPin(p=>p+k); };
  const submit=()=>{ if(pin===op.pin) onSignIn({name:op.name,role:op.role}); else { setErr(true); setPin(''); } };
  return (
    <div className="fx-back" onClick={onClose}><div className="fx-modal" style={{maxWidth:'420px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">Sign In</span><span className="x" onClick={onClose}>&times;</span></div>
      <div className="fx-body">
        <div className="fx-field"><div className="fx-k">Operator</div>
          <select className="fx-select" value={oi} onChange={e=>{setOi(+e.target.value);setPin('');}}>{ops.map((o,i)=><option key={i} value={i}>{o.name} · {o.role}</option>)}</select>
          <div className="fx-latin">{PG.ROLES[op.role].can}</div></div>
        <div className="fx-field"><div className="fx-k">PIN</div><PinPad value={pin} onKey={key}/>{err && <div className="fx-warn">Incorrect PIN — demo PIN {op.pin}</div>}</div>
      </div>
      <div className="fx-foot"><span className="note">Signed actions are logged.</span><button className="fx-btn" onClick={onClose}>Cancel</button><button className="fx-btn primary" onClick={submit} disabled={pin.length<4}>Sign In</button></div>
    </div></div>
  );
}
// rack lock screen (dark, full-face)
function LockScreen({ clock, onUnlock }){
  const ops=PG.OPERATORS;
  const [oi,setOi]=React.useState(0);
  const [pin,setPin]=React.useState('');
  const op=ops[oi];
  const key=k=>{ if(k==='del'){setPin(p=>p.slice(0,-1));return;} if(k==='clr'){setPin('');return;} if(pin.length>=4)return;
    const np=pin+k; setPin(np); if(np.length===4) setTimeout(()=>{ if(np===op.pin) onUnlock(op); else setPin(''); },160); };
  return (
    <div className="rk-lock">
      <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--lcd-dim)" strokeWidth="1.6"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><circle cx="12" cy="15.5" r="1.4" fill="var(--lcd-dim)" stroke="none"/></svg>
      <div className="ti">Propagation Unit No. 01</div>
      <div className="op">Locked &middot; {clock} &middot; select operator, enter PIN</div>
      <select className="fx-select rk-opsel" value={oi} onChange={e=>{setOi(+e.target.value);setPin('');}}>{ops.map((o,i)=><option key={i} value={i}>{o.name} · {o.role}</option>)}</select>
      <PinPad value={pin} onKey={key} dark/>
      <div className="op" style={{opacity:.6}}>Demo PIN &middot; {op.pin}</div>
    </div>
  );
}

/* ============ FLOW 3 · OFFLINE ============ */
function OfflineBanner({ lastSync='14:32' }){
  return (<div className="ban web"><span className="d"></span>Controller unreachable — showing last-known readings from {lastSync} · retrying…</div>);
}

/* ============ change 2 · MAINTENANCE MODE ============ */
// System-wide banner: irrigation automation is suspended (lighting still runs;
// manual watering asks for confirmation). Stewards/growers get a resume button.
function MaintenanceBanner({ onExit, canManage }){
  return (
    <div className="ban web" style={{background:'var(--ochre-deep)'}}>
      <span className="d"></span>
      Maintenance mode · irrigation automation suspended — lighting still runs, manual watering asks first
      {canManage && onExit &&
        <button className="fx-btn" style={{marginLeft:'auto',padding:'5px 12px'}} onClick={onExit}>Resume automation</button>}
    </div>
  );
}

/* ============ FLOW 4 · ALERTS + ESCALATION ============ */
function AlertBar({ alerts, onOpen }){
  if(!alerts || !alerts.length) return null;
  return (
    <div className="albar">
      {alerts.slice(0,3).map(a=>(
        <div className="alrow" key={a.id}>
          <span className="sev" style={{background:a.sev}}></span>
          <span className="kind">{a.kind}</span>
          <span className="msg">{a.msg}</span>
          <span className="prio">{a.prio}</span>
          <button className="ackb" onClick={onOpen}>Review</button>
        </div>
      ))}
      {alerts.length>3 && <button className="fx-btn" style={{alignSelf:'flex-start'}} onClick={onOpen}>+{alerts.length-3} more · Open alerts</button>}
    </div>
  );
}
function EscRow({ label, sub, on, tog }){
  return (<div className="escrow"><div><div style={{fontFamily:'Newsreader',fontSize:'15px'}}>{label}</div>{sub && <div className="fx-k" style={{margin:'3px 0 0'}}>{sub}</div>}</div><div className={'toggle'+(on?' on':'')} onClick={tog}><i></i></div></div>);
}
function AlertsModal({ alerts, esc, setEsc, onAck, onAckAll, onClose }){
  const tog=k=>setEsc(e=>({...e,[k]:!e[k]}));
  const step=(k,min,max)=>f=>setEsc(e=>({...e,[k]:Math.max(min,Math.min(max, typeof f==='function'?f(e[k]):f))}));
  return (
    <div className="fx-back" onClick={onClose}><div className="fx-modal" style={{maxWidth:'640px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">Alerts &amp; Escalation</span><span className="tag">{alerts.length} active</span><span className="x" onClick={onClose}>&times;</span></div>
      <div className="fx-body">
        <div className="fx-k">Active alerts</div>
        {alerts.length
          ? <div className="albar" style={{marginBottom:'20px'}}>{alerts.map(a=>(
              <div className="alrow" key={a.id}><span className="sev" style={{background:a.sev}}></span><span className="kind">{a.kind}</span><span className="msg">{a.msg}</span><button className="ackb" onClick={()=>onAck(a.id)}>Acknowledge</button></div>))}</div>
          : <div className="fx-latin" style={{marginBottom:'20px'}}>No active alerts — all clear.</div>}
        <div className="fx-k">Escalation policy</div>
        <div className="esc-box">
          <EscRow label="Email the on-call steward" sub="A. Short · steward@shortsresort" on={esc.email} tog={()=>tog('email')}/>
          <EscRow label="SMS text alert" sub="For Action-Required only" on={esc.sms} tog={()=>tog('sms')}/>
        </div>
        <div className="fx-2" style={{marginTop:'16px'}}>
          <div><div className="fx-k">Repeat until acknowledged</div><Stepper v={esc.repeatMin} set={step('repeatMin',5,60)} min={5} max={60} unit="m"/></div>
          <div></div>
        </div>
      </div>
      <div className="fx-foot"><span className="note">Sensor Fault &amp; Reservoir Low repeat until acknowledged.</span><button className="fx-btn" onClick={onClose}>Close</button><button className="fx-btn primary" onClick={onAckAll} disabled={!alerts.length}>Acknowledge all</button></div>
    </div></div>
  );
}

/* ============ FLOW 5 · HISTORY + EXPORT ============ */
function HistoryModal({ onClose }){
  const ranges=['24 Hours','7 Days','30 Days','Season'];
  const [r,setR]=React.useState('7 Days');
  const rows=PG.history(ranges.indexOf(r));
  const csv='timestamp,event,detail,category\n'+rows.map(x=>`"${x.ts}","${x.ev}","${x.em}","${x.cat}"`).join('\n');
  const href='data:text/csv;charset=utf-8,'+encodeURIComponent(csv);
  return (
    <div className="fx-back" onClick={onClose}><div className="fx-modal" style={{maxWidth:'720px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">Event History</span><span className="tag">{rows.length} entries</span><span className="x" onClick={onClose}>&times;</span></div>
      <div className="fx-body">
        <div className="hs-controls">
          <div className="seg">{ranges.map(x=><button key={x} className={x===r?'on':''} onClick={()=>setR(x)}>{x}</button>)}</div>
          <a className="fx-btn" style={{marginLeft:'auto',textDecoration:'none'}} href={href} download={`propagation-unit-01-history-${r.replace(/\s/g,'').toLowerCase()}.csv`}>Export CSV</a>
        </div>
        <div className="hs-list">
          {rows.map((x,k)=>(
            <div className="hs-row" key={k}>
              <span className="ts" style={{color:'#1B181399'}}>{x.ts}</span>
              <span className="ev">{x.ev}<em>{x.em}</em></span>
              <span className="cat"><span className="sev" style={{background:x.sev}}></span>{x.cat}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="fx-foot"><span className="note">Retention · 18 months on-unit; exported nightly to the field-station archive.</span><button className="fx-btn" onClick={onClose}>Close</button></div>
    </div></div>
  );
}

/* ============ FLOW 6 · CONFIRM (config safety / signed actions) ============ */
function Confirm({ title, body, confirmLabel, danger, onConfirm, onCancel }){
  return (
    <div className="fx-back" onClick={onCancel}><div className="fx-modal" style={{maxWidth:'440px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">{title}</span><span className="x" onClick={onCancel}>&times;</span></div>
      <div className="fx-body"><div style={{fontFamily:'Newsreader',fontSize:'16px',lineHeight:1.5}}>{body}</div></div>
      <div className="fx-foot"><span className="note"></span><button className="fx-btn" onClick={onCancel}>Cancel</button><button className={'fx-btn '+(danger?'danger':'primary')} onClick={onConfirm}>{confirmLabel||'Confirm'}</button></div>
    </div></div>
  );
}

export { Stepper, PinPad, PhotoRail, AssignModal, SignInModal, LockScreen, OfflineBanner, MaintenanceBanner, AlertBar, AlertsModal, HistoryModal, Confirm };
