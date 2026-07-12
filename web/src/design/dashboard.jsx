import React from 'react';
import { PG, ForestMark, LineChart, EventBars } from './data.jsx';
import { AssignModal, SignInModal, Confirm, AlertsModal, HistoryModal, AlertBar, OfflineBanner, MaintenanceBanner, PhotoRail } from './flows.jsx';

// pgui-dashboard.jsx — the web dashboard (paper). Tabs:
// Dashboard · Shelf Management · Environmental · Controls

const DB_TABS = ['Dashboard','Shelf Management','Environmental','Controls'];

// ---------- DASHBOARD (overview + log) ----------
function OverviewTab({ shelves, reservoir, rackStatus, log, alerts, onOpenAlerts, onOpenHistory }){
  const active = shelves.filter(s=>s.active);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;
  const issues = shelves.filter(s=>s.active && (s.status==='Monitoring'||s.status==='Dry'||s.status==='Sensor Fault'));
  const vacant = shelves.length - active.length;
  const watering = shelves.filter(s=>s.status==='Watering').length;
  const cells = [
    { k:'Rack Status', v:rackStatus, dot:rs.col, n: rackStatus==='Operational'?'All subsystems nominal':'Check diagnostics' },
    { k:'Reservoir', v:reservoir, unit:'%', n:`≈ ${PG.tankDays(reservoir)} days remaining` },
    { k:'Active Shelves', v:active.length, unit:' / '+shelves.length, n: vacant? vacant+(vacant===1?' bay':' bays')+' unassigned':'All bays assigned' },
    { k:'Watering Now', v:watering, n: watering? 'Cycle in progress':'Pump at rest' },
    { k:'Active Alerts', v:issues.length, n: issues.length? issues[0].status+' · Shelf '+issues[0].n : 'None outstanding' },
  ];
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Operations Overview</div>
        <div className="sub">Department of Forestry · Native Nursery Programme</div>
      </div>
      {alerts && alerts.length>0 && <AlertBar alerts={alerts} onOpen={onOpenAlerts}/>}
      <div className="stat-grid">
        {cells.map((c,i)=>(
          <div className="stat-cell" key={i}>
            <div className="k">{c.k}</div>
            <div className="v" style={{color: i===0?rs.ink:'var(--ink)'}}>
              {c.v}{c.unit && <small>{c.unit}</small>}
            </div>
            {c.dot
              ? <div className="mini"><span className="d" style={{background:c.dot}}></span>{c.n}</div>
              : <div className="n">{c.n}</div>}
          </div>
        ))}
      </div>

      <div className="db-2col">
        <div className="panel">
          <div className="ph"><div className="pt">Event Log</div><button className="pm" onClick={onOpenHistory} style={{border:'1px solid var(--ink)',borderRadius:'4px',padding:'5px 11px',background:'var(--paper)',cursor:'pointer'}}>Full history &rsaquo;</button></div>
          <div className="pb">
            <div className="log">
              {log.map((e,i)=>(
                <div className="log-row" key={i}>
                  <div style={{display:'flex',gap:10}}>
                    <span className="sev" style={{background:e.sev}}></span>
                    <span className="ts">{e.ts}</span>
                  </div>
                  <div className="ev">{e.ev}<em>{e.em}</em></div>
                </div>
              ))}
            </div>
          </div>
        </div>
        <div className="panel" style={{alignSelf:'start'}}>
          <div className="ph"><div className="pt">Reservoir</div><div className="pm">26.5 L Tank · 7 gal</div></div>
          <div className="pb">
            <div style={{display:'flex',alignItems:'flex-end',gap:14}}>
              <div style={{flex:'none',width:64,height:130,border:'1px solid var(--ink)',borderRadius:3,background:'var(--bone)',position:'relative',overflow:'hidden'}}>
                <div style={{position:'absolute',left:0,right:0,bottom:0,height:reservoir+'%',background:'linear-gradient(180deg,var(--river),var(--river-deep))'}}></div>
                <div style={{position:'absolute',inset:0,background:'repeating-linear-gradient(0deg,transparent 0 calc(20% - 1px),#1B181333 calc(20% - 1px) 20%)'}}></div>
              </div>
              <div>
                <div style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:42,lineHeight:0.9}}>{reservoir}<span style={{fontSize:20,color:'var(--char)'}}>%</span></div>
                <div className="mono" style={{fontSize:10,letterSpacing:'0.14em',textTransform:'uppercase',color:'#1B181399',marginTop:6,lineHeight:1.7}}>
                  ≈ {PG.tankLiters(reservoir)} L on hand<br/>≈ {PG.tankDays(reservoir)} days remaining<br/>Trend · steady
                </div>
              </div>
            </div>
            <div style={{marginTop:16,paddingTop:14,borderTop:'1px solid var(--rule-soft)'}} className="mono">
              <div style={{display:'flex',alignItems:'center',gap:9,fontSize:11,letterSpacing:'0.14em',textTransform:'uppercase'}}>
                <span style={{width:9,height:9,borderRadius:'50%',background:reservoir>20?'var(--sage)':'var(--oxblood)'}}></span> {reservoir>20?'Level nominal':'Refill required'}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---------- SHELF MANAGEMENT ----------
function ShelfCard({ s, onAssign, onEdit, onSetEnabled, onUnassign, role }){
  const meta = PG.STATUS[s.status] || PG.STATUS.Inactive;
  const inBand = s.moisture>=s.lo && s.moisture<=s.hi;
  const readonly = !role || role==='Viewer';
  return (
    <div className={'scard'+(s.active?'':' inactive')}>
      <div className="stripe" style={{background:s.stripe}}></div>
      <div className="sc-top">
        <div className="sc-no">{s.n}<small>SHELF</small></div>
        <div className="sc-sp">
          <div className="nm">{s.species}</div>
          <div className="lt">{s.tag}</div>
          <div className="sc-badge" style={{color:meta.inkCol}}>{s.status}</div>
        </div>
      </div>
      {s.active ? (
        <React.Fragment>
          <div className="sc-mid">
            <div><div className="mk">Moisture</div><div className="mv">{s.moisture>0 ? s.moisture : '—'}{s.moisture>0 && <span>%VWC</span>}</div></div>
            <div><div className="mk">Root Zone Temp</div><div className="mv">{s.temp>0 ? s.temp : '—'}{s.temp>0 && <span>°F</span>}</div></div>
          </div>
          <div className="sc-bar">
            <div className="mk" style={{marginBottom:7}}>Moisture vs. target band</div>
            <div className="barwrap">
              <i style={{width:s.moisture+'%',background:inBand?'var(--sage)':'var(--ochre)'}}></i>
              <span className="tgt" style={{left:s.lo+'%'}}></span>
              <span className="tgt" style={{left:s.hi+'%'}}></span>
            </div>
            <div className="barlabels"><span>0%</span><span>Target {s.lo}–{s.hi}%</span><span>100%</span></div>
          </div>
          <div className="sc-foot">
            <div><div className="k">Lighting</div><div className="v">{s.light?'On · '+s.on:'Off'}</div></div>
            <div>
              <div className="k">Last Water</div>
              <div className="v">{s.lastWater}</div>
              {s.lastWaterDate && <div className="k" style={{marginTop:3,letterSpacing:'0.08em'}}>{s.lastWaterDate}</div>}
            </div>
            <div><div className="k">Usage · 24h</div><div className="v">{s.usage}</div></div>
          </div>
          <div style={{padding:'0 18px 16px',display:'flex',gap:8}}>
            <button className="fx-btn" style={{flex:1}} disabled={readonly}
              onClick={()=>onEdit && onEdit(s)}>
              {readonly ? 'Sign in to edit' : 'Edit Shelf ›'}
            </button>
            {/* change 2: manual override — pause this shelf's sensors + valve. */}
            {!readonly && onSetEnabled &&
              <button className="fx-btn" onClick={()=>onSetEnabled(s.n, false)}>Disable</button>}
          </div>
        </React.Fragment>
      ) : s.assigned ? (
        // change 2: paused shelf — assigned but disabled (automation off).
        <div style={{padding:'22px 18px'}}>
          <div style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',fontSize:'15px',lineHeight:1.4,textAlign:'center'}}>
            Automation paused — sensors &amp; valve disabled for Shelf {s.n}. Readings are ignored and no cycles will fire.
          </div>
          <div style={{display:'flex',gap:8,marginTop:'14px'}}>
            <button className="fx-btn primary" style={{flex:1}} disabled={readonly}
              onClick={()=>onSetEnabled && onSetEnabled(s.n, true)}>
              {readonly ? 'Sign in to enable' : 'Enable Shelf'}
            </button>
            {!readonly && onUnassign &&
              <button className="fx-btn danger" onClick={()=>onUnassign(s.n)}>Clear Bay</button>}
          </div>
        </div>
      ) : (
        <div style={{padding:'26px 18px 22px',textAlign:'center'}}>
          <div style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',fontSize:'15px',lineHeight:1.4}}>
            Bay available — assign a specimen group to bring Shelf {s.n} online.
          </div>
          <button className="fx-btn" style={{marginTop:'14px'}} disabled={readonly} onClick={()=>onAssign && onAssign(s.n)}>
            {readonly ? 'Sign in to assign' : 'Assign Specimen'}
          </button>
        </div>
      )}
    </div>
  );
}

function ShelvesTab({ shelves, onAssign, onEdit, onSetEnabled, onUnassign, role }){
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Shelf Management</div>
        <div className="sub">One card per growing shelf · physical bays 01–04</div>
      </div>
      <div className="shelf-grid">
        {shelves.map(s=> <ShelfCard key={s.n} s={s} onAssign={onAssign} onEdit={onEdit}
          onSetEnabled={onSetEnabled} onUnassign={onUnassign} role={role}/> )}
      </div>
    </div>
  );
}

// ---------- ENVIRONMENTAL ----------
// With getSeries (the live build) the charts render hourly aggregates from
// the backend; without it they fall back to the handoff's demo series.
function EnvironmentalTab({ getSeries }){
  const [period, setPeriod] = React.useState('7 Days');
  const periods = ['24 Hours','7 Days','30 Days','Season'];
  const [live, setLive] = React.useState(null);
  React.useEffect(()=>{
    if(!getSeries) return;
    let alive = true;
    getSeries(period).then(d=>{ if(alive) setLive(d); }).catch(()=>{ if(alive) setLive(null); });
    return ()=>{ alive = false; };
  }, [period, getSeries]);

  const seed = periods.indexOf(period)+1;
  const n = 28;
  const demoMoist = [
    { color:'var(--river)', fill:true, data:PG.series(seed*1.1, n, 67, 12, 2) },
    { color:'var(--ochre)', data:PG.series(seed*2.3, n, 55, 14, 4) },
    { color:'var(--sage)', data:PG.series(seed*3.7, n, 70, 9, -2) },
  ];
  const demoTemp = [
    { color:'var(--oxblood)', fill:true, data:PG.series(seed*4.2, n, 75, 6, 1) },
  ];
  const demoWater = PG.series(seed*5.5, 14, 3, 5, 0).map(v=>Math.max(0,Math.round(v*10)/10));
  const demoEc = [{ color:'var(--sage-deep)', fill:true, data:PG.series(seed*6.1, n, 1150, 220, 15).map(v=>Math.round(v)) }];

  const isLive = !!(getSeries && live);
  const moist = isLive ? live.moist : demoMoist;
  const temp = isLive ? live.temp : demoTemp;
  const waterEvents = isLive ? live.water : demoWater;
  const [mLo, mHi] = isLive ? live.moistY : [40, 85];
  const [tLo, tHi] = isLive ? live.tempY : [68, 82];
  const legend = isLive
    ? live.legend
    : [{color:'var(--river)',label:'Shelf 01'},{color:'var(--ochre)',label:'Shelf 02'},{color:'var(--sage)',label:'Shelf 03'}];
  const noData = (
    <div className="cb" style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',padding:'26px 0',textAlign:'center'}}>
      Collecting data — this chart fills in as hourly aggregates accumulate.
    </div>
  );
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Environmental Monitoring</div>
        <div className="env-controls">
          <div className="seg">
            {periods.map(p=>(
              <button key={p} className={p===period?'on':''} onClick={()=>setPeriod(p)}>{p}</button>
            ))}
          </div>
        </div>
      </div>

      <div className="chart-card">
        <div className="ch-h">
          <div className="ch-t"><span className="sw" style={{background:'var(--river)'}}></span>Soil Moisture · %VWC</div>
          <div className="ch-leg">
            {legend.map((l,i)=>(<span key={i}><i style={{background:l.color}}></i>{l.label}</span>))}
          </div>
        </div>
        {moist.length
          ? <div className="cb"><LineChart datasets={moist} yMin={mLo} yMax={mHi} fmtY={v=>v+'%'}/></div>
          : noData}
      </div>

      <div className="chart-card">
        <div className="ch-h">
          <div className="ch-t"><span className="sw" style={{background:'var(--oxblood)'}}></span>Root Zone Temperature · °F</div>
          <div className="ch-leg"><span><i style={{background:'var(--oxblood)'}}></i>Rack mean</span></div>
        </div>
        {temp.length
          ? <div className="cb"><LineChart datasets={temp} yMin={tLo} yMax={tHi} fmtY={v=>v+'°'}/></div>
          : noData}
      </div>

      <div className="chart-card">
        <div className="ch-h">
          {/* µS/cm sits outside .ch-t's uppercase transform — µ would
              uppercase to Μ (mu) and misread as mS/cm. */}
          <div className="ch-t"><span className="sw" style={{background:'var(--sage-deep)'}}></span>Substrate EC · <span style={{textTransform:'none'}}>µS/cm</span></div>
          <div className="ch-leg">
            {(isLive ? (live.ecLegend||[]) : [{color:'var(--sage-deep)',label:'Shelf 01'}]).map((l,i)=>(
              <span key={i}><i style={{background:l.color}}></i>{l.label}</span>))}
          </div>
        </div>
        {(isLive ? live.ec : demoEc).length
          ? <div className="cb"><LineChart datasets={isLive?live.ec:demoEc} yMin={isLive?live.ecY[0]:400} yMax={isLive?live.ecY[1]:2000} fmtY={v=>Math.round(v)}/></div>
          : noData}
      </div>

      <div className="chart-card">
        <div className="ch-h">
          <div className="ch-t"><span className="sw" style={{background:'var(--river-deep)'}}></span>Watering · minutes of pump time</div>
          <div className="ch-leg"><span className="mono">{period}</span></div>
        </div>
        <div className="cb"><EventBars data={waterEvents} color="var(--river-deep)"/></div>
      </div>

      {/* change 3: EC/temp time-in-target — share of hourly readings inside
          each shelf's happy-band over the period (monitoring only). */}
      {isLive && live.adherence && live.adherence.length>0 && (
        <div className="chart-card">
          <div className="ch-h">
            <div className="ch-t"><span className="sw" style={{background:'var(--sage-deep)'}}></span>Target Adherence · time in happy-band</div>
            <div className="ch-leg"><span className="mono">{period}</span></div>
          </div>
          <div className="cb">
            <div style={{display:'grid',gridTemplateColumns:'1fr 1.4fr 1.4fr',gap:'8px 18px',fontFamily:'"JetBrains Mono",monospace',fontSize:11,alignItems:'baseline'}}>
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'#1B181399'}}>Shelf</div>
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'#1B181399'}}>Substrate EC</div>
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'#1B181399'}}>Root Zone Temp</div>
              {live.adherence.map(a=>(
                <React.Fragment key={a.n}>
                  <div>Shelf {a.n}</div>
                  <div>{a.ec
                    ? <span><b style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:15}}>{a.ec.pct}%</b> in {a.ecBand[0]}–{a.ecBand[1]} µS/cm <span style={{color:'#1B181399'}}>· {a.ec.n}h</span></span>
                    : <span style={{color:'#1B181399'}}>{a.ecBand?'awaiting data':'no band set'}</span>}</div>
                  <div>{a.temp
                    ? <span><b style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:15}}>{a.temp.pct}%</b> in {a.tempBand[0]}–{a.tempBand[1]}°F <span style={{color:'#1B181399'}}>· {a.temp.n}h</span></span>
                    : <span style={{color:'#1B181399'}}>{a.tempBand?'awaiting data':'no band set'}</span>}</div>
                </React.Fragment>
              ))}
            </div>
            <div style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',fontSize:13,marginTop:14,lineHeight:1.5}}>
              Share of hourly readings inside each shelf's target band over the selected period — for historical cross-checking against plant health. Monitoring only; the unit senses substrate EC &amp; root zone temperature but doesn't control them. Temperature is rack-wide until each shelf has its own probe.
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------- CONTROLS ----------
function Slider({ label, sub, value, min, max, unit, onChange, disabled }){
  return (
    <div className="ctrl-row">
      <div className="cl">{label}{sub && <small>{sub}</small>}</div>
      <div style={{display:'flex',alignItems:'center',gap:16}}>
        <input className="rng" type="range" min={min} max={max} value={value} disabled={disabled} onChange={e=>onChange(+e.target.value)}/>
        <div className="cv">{value}<span>{unit}</span></div>
      </div>
    </div>
  );
}

// With a `controls` prop (the live build) the sliders load real per-zone
// moisture rules and the rack photoperiod, and Apply writes them back
// through the API. Without it, the tab behaves as the handoff demo.
function ControlsTab({ role, controls }){
  const readonly = !role || role==='Viewer';
  const shelfList = controls && controls.list.length
    ? controls.list.map(x=>x.n)
    : ['01','02','03'];
  const [irr, setIrr] = React.useState({ target:70, min:58, durMin:5, cool:6 });
  const [shelf, setShelf] = React.useState(shelfList[0] || '01');
  const [scope, setScope] = React.useState('this');
  // Photoperiod is "lights on at + hours of light": the off time is derived,
  // so the two can never add up to more than a day.
  const [photo, setPhoto] = React.useState({ on:6, hours:13 });
  const [dirty, setDirty] = React.useState(false);
  const [saved, setSaved] = React.useState(false);
  const [confirm, setConfirm] = React.useState(false);
  const [applyErr, setApplyErr] = React.useState('');

  // Load the selected shelf's real parameters.
  React.useEffect(()=>{
    if(!controls) return;
    let alive = true;
    controls.load(shelf).then(p=>{
      if(!alive || !p) return;
      setIrr({ target:p.target, min:p.min, durMin:p.durMin, cool:p.cool });
      setPhoto({ on:p.on, hours:p.hours });
      setDirty(false); setSaved(false); setApplyErr('');
    }).catch(()=>{});
    return ()=>{ alive = false; };
  }, [shelf, controls]);

  const offH = (photo.on + photo.hours) % 24;
  const set = (k,v)=>{ setIrr(p=>({...p,[k]:v})); setDirty(true); setSaved(false); };
  const setP = (k,v)=>{ setPhoto(p=>({...p,[k]:v})); setDirty(true); setSaved(false); };
  const valid = irr.target > irr.min && photo.hours >= 1 && photo.hours <= 23;
  const apply = ()=>{
    setConfirm(false);
    if(!controls){ setDirty(false); setSaved(true); return; }
    setApplyErr('');
    controls.apply(scope, shelf, irr, photo)
      .then(()=>{ setDirty(false); setSaved(true); })
      .catch(e=>setApplyErr(e.message || 'apply failed'));
  };
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Irrigation &amp; Lighting</div>
        <div className="sub">Per-shelf operating parameters</div>
      </div>
      {readonly && <div className="ro-note">Read-only — sign in as a Grower or Steward to change operating parameters.</div>}
      <div style={{display:'flex',gap:8,alignItems:'center',margin:'0 0 18px'}}>
        <span className="mono" style={{fontSize:10,letterSpacing:'0.16em',textTransform:'uppercase',color:'#1B181399',marginRight:6}}>Editing</span>
        <div className="seg">
          {shelfList.map(sh=>(
            <button key={sh} className={sh===shelf?'on':''} onClick={()=>setShelf(sh)} disabled={readonly}>Shelf {sh}</button>
          ))}
        </div>
      </div>
      <div className="ctrl-grid">
        <div className="panel">
          <div className="ph"><div className="pt">Irrigation</div><div className="pm">Shelf {shelf}</div></div>
          <div className="pb">
            <Slider label="Moisture target" sub="Pump rests above this" value={irr.target} min={40} max={90} unit="%" onChange={v=>set('target',v)} disabled={readonly}/>
            <Slider label="Moisture minimum" sub="Cycle triggers below this" value={irr.min} min={20} max={80} unit="%" onChange={v=>set('min',v)} disabled={readonly}/>
            <Slider label="Watering duration" sub="Soak time — large pots may need up to 30 min" value={irr.durMin} min={1} max={30} unit="min" onChange={v=>set('durMin',v)} disabled={readonly}/>
            <Slider label="Cooldown period" sub="Minimum rest between cycles" value={irr.cool} min={1} max={24} unit="h" onChange={v=>set('cool',v)} disabled={readonly}/>
            {irr.target<=irr.min && <div className="fx-warn" style={{marginTop:6}}>Target must exceed minimum</div>}
          </div>
        </div>

        <div className="panel" style={{alignSelf:'start'}}>
          <div className="ph"><div className="pt">Lighting · Photoperiod</div><div className="pm">{controls?'Rack-wide':'Shelf '+shelf}</div></div>
          <div className="pb">
            <Slider label="Lights on at" sub="Start of the photoperiod" value={photo.on} min={0} max={23} unit=":00" onChange={v=>setP('on',v)} disabled={readonly}/>
            <Slider label="Hours of light" sub="Off time follows automatically" value={photo.hours} min={1} max={23} unit="h" onChange={v=>setP('hours',v)} disabled={readonly}/>
            <div className="ctrl-row"><div className="cl">Lights off at<small>Computed</small></div><div className="cv">{String(offH).padStart(2,'0')}<span>:00</span></div></div>
            <PhotoRail onH={photo.on} hours={photo.hours}/>
            <div className="ctrl-row" style={{marginTop:4}}>
              <div className="cl">Manual override<small>Force lights on now</small></div>
              <ManualToggle on={controls?controls.lightOn:undefined}
                onToggle={controls?controls.manualLight:undefined}/>
            </div>
          </div>
        </div>
      </div>
      <div className="apply-bar">
        <span className="mono" style={{fontSize:10,letterSpacing:'0.16em',textTransform:'uppercase',color:'#1B181399'}}>Apply to</span>
        <div className="seg">
          <button className={scope==='this'?'on':''} onClick={()=>setScope('this')} disabled={readonly}>Shelf {shelf}</button>
          <button className={scope==='all'?'on':''} onClick={()=>setScope('all')} disabled={readonly}>All active shelves</button>
        </div>
        {!valid && <span className="fx-warn">Fix ranges before applying</span>}
        {applyErr && <span className="fx-warn">{applyErr}</span>}
        <button className="fx-btn primary" style={{marginLeft:'auto'}} disabled={readonly||!dirty||!valid} onClick={()=>setConfirm(true)}>{saved?'Saved ✓':'Review & Apply'}</button>
      </div>
      {confirm && <Confirm title="Apply parameters"
        body={`Apply irrigation & lighting to ${scope==='all'?'all active shelves':'Shelf '+shelf}? Target ${irr.target}%VWC (min ${irr.min}%), ${irr.durMin} min soak, ${irr.cool}h cooldown; photoperiod ${String(photo.on).padStart(2,'0')}:00–${String(offH).padStart(2,'0')}:00 (rack-wide).`}
        confirmLabel="Apply" onConfirm={apply} onCancel={()=>setConfirm(false)}/>}
    </div>
  );
}

function ManualToggle({ on, onToggle }){
  const [local,setLocal]=React.useState(false);
  const state = on !== undefined ? on : local;
  const flip = ()=> onToggle ? onToggle(!state) : setLocal(!state);
  return <div className={'toggle'+(state?' on':'')} onClick={flip}><i></i></div>;
}

// Deep-linkable tabs: "/#environmental", "/#controls", "/#shelves".
// Build AssignModal initial values from a live shelf (quick-edit).
function editInitial(s){
  const onH = parseInt(s.on, 10), offH = parseInt(s.off, 10);
  const raw = s.raw || {};
  return {
    species: raw.species1 || s.species,
    species2: raw.species2 || '',
    lo: s.lo, hi: s.hi,
    durMin: raw.durMin || 5,
    // change 1: carry the shelf's real drain-back time into the edit modal.
    drainMin: raw.drainMin,
    // change 3: EC/temp happy-bands, applied profile, and moisture guards.
    ecLo: raw.ecLo, ecHi: raw.ecHi, tempLo: raw.tempLo, tempHi: raw.tempHi,
    profileId: raw.profileId, stage: raw.stage,
    coolMin: raw.coolMin, maxCycles: raw.maxCycles,
    onH: isNaN(onH) ? 6 : onH,
    lightHrs: (!isNaN(onH) && !isNaN(offH)) ? (((offH - onH + 24) % 24) || 13) : 13,
  };
}

function initialTab(){
  const h = (typeof location !== 'undefined' ? location.hash : '').replace('#','').toLowerCase();
  return { dashboard:'Dashboard', shelves:'Shelf Management', shelf:'Shelf Management',
    environmental:'Environmental', controls:'Controls' }[h] || 'Dashboard';
}

// `auth` (live build): { SignInModal, canManage, openOperators } — swaps the
// demo PIN sign-in for the real one and adds operator management for admins.
function Dashboard({ shelves, reservoir, rackStatus, log, session, onSignIn, onSignOut, alerts=[], onAck, onAckAll, connection='online', assignments, onAssign, getSeries, controls, auth, maintenance=false, onSetMaintenance, onSetEnabled, onUnassign, profiles }){
  const [tab, setTab] = React.useState(initialTab);
  const [assignFor, setAssignFor] = React.useState(null);
  const [showSignIn, setShowSignIn] = React.useState(false);
  const [showSignOut, setShowSignOut] = React.useState(false);
  const [showAlerts, setShowAlerts] = React.useState(false);
  const [showHistory, setShowHistory] = React.useState(false);
  const [showMaint, setShowMaint] = React.useState(false); // change 2: confirm dialog
  const [esc, setEsc] = React.useState(PG.ESCALATION_DEFAULT);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;

  // fallbacks so the component also works standalone (e.g. the handoff figure)
  const [locAssign, setLocAssign] = React.useState({});
  const [locSession, setLocSession] = React.useState({ name:'A. Short', role:'Steward' });
  const ses = session !== undefined ? session : locSession;
  const assignFn = onAssign || ((n,f)=>setLocAssign(a=>({...a,[n]:f})));
  const signInFn = onSignIn || setLocSession;
  const signOutFn = onSignOut || (()=>setLocSession(null));
  const ackFn = onAck || (()=>{});
  const ackAllFn = onAckAll || (()=>{});
  const effShelves = onAssign ? shelves : shelves.map(s=> locAssign[s.n] ? {...s, ...locAssign[s.n], active:true} : s);
  const role = ses ? ses.role : null;

  const canToggleMaint = role && role!=='Viewer' && onSetMaintenance;
  return (
    <div className="db">
      {connection==='offline' && <OfflineBanner/>}
      {/* change 2: maintenance banner sits under any offline banner. */}
      {maintenance && <MaintenanceBanner canManage={!!canToggleMaint} onExit={()=>setShowMaint(true)}/>}
      <div className="db-top">
        <div className="db-brand">
          <ForestMark/>
          <div className="t">Propagation Unit 01<small>Control Center</small></div>
        </div>
        <div className="db-tabs">
          {DB_TABS.map(t=>(
            <button key={t} className={'db-tab'+(t===tab?' on':'')} onClick={()=>setTab(t)}>{t}</button>
          ))}
        </div>
        <div className={'db-alertbtn'+(alerts.length?'':' calm')} onClick={()=>setShowAlerts(true)}>
          Alerts <span className="cnt">{alerts.length}</span>
        </div>
        {auth && auth.canManage &&
          <div className="db-user" onClick={auth.openOperators} title="Manage operators">Operators</div>}
        {ses
          ? <div className="db-user" onClick={()=>setShowSignOut(true)} title="Sign out">
              <span style={{width:9,height:9,borderRadius:'50%',background:'var(--sage)'}}></span>
              <div className="who"><b>{ses.name}</b><span className="role">{ses.role}</span></div>
            </div>
          : <div className="db-user" onClick={()=>setShowSignIn(true)}>Sign In</div>}
        {/* change 2: maintenance toggle (Grower/Steward). Confirms both ways. */}
        {canToggleMaint &&
          <div className="db-user" onClick={()=>setShowMaint(true)}
            title={maintenance?'Resume irrigation automation':'Suspend irrigation automation'}
            style={maintenance?{borderColor:'var(--ochre-deep)',color:'var(--ochre-deep)'}:undefined}>
            <span style={{width:9,height:9,borderRadius:'50%',background:maintenance?'var(--ochre-deep)':'var(--char)'}}></span>
            {maintenance?'Maintenance On':'Maintenance'}
          </div>}
        <div className="db-statuschip">
          <span className="d" style={{background:rs.col}}></span>{rackStatus}
        </div>
      </div>
      <div className="db-body">
        {tab==='Dashboard' && <OverviewTab shelves={effShelves} reservoir={reservoir} rackStatus={rackStatus} log={log} alerts={alerts} onOpenAlerts={()=>setShowAlerts(true)} onOpenHistory={()=>setShowHistory(true)}/>}
        {tab==='Shelf Management' && <ShelvesTab shelves={effShelves}
          onAssign={(n)=>setAssignFor({n})}
          onEdit={(s)=>setAssignFor({n:s.n, initial:editInitial(s)})}
          onSetEnabled={onSetEnabled}
          onUnassign={onUnassign}
          role={role}/>}
        {tab==='Environmental' && <EnvironmentalTab getSeries={getSeries}/>}
        {tab==='Controls' && <ControlsTab role={role} controls={controls}/>}
      </div>

      {assignFor && <AssignModal shelfN={assignFor.n} initial={assignFor.initial} onClose={()=>setAssignFor(null)} onAssign={(n,f)=>{ assignFn(n,f); setAssignFor(null); }}
        onClear={onUnassign ? (n)=>{ onUnassign(n); setAssignFor(null); } : undefined}
        onDisable={onSetEnabled ? (n)=>{ onSetEnabled(n, false); setAssignFor(null); } : undefined}
        profiles={profiles}/>}
      {/* change 2: confirm entering/leaving maintenance mode. */}
      {showMaint && <Confirm
        title={maintenance?'Resume automation':'Enter maintenance mode'}
        body={maintenance
          ? 'Resume irrigation automation? Moisture triggers and schedules will run again on all enabled shelves.'
          : 'Suspend irrigation automation across the rack? Pumps won’t fire from moisture triggers or schedules while you work. Lighting keeps running and you can still water a shelf by hand.'}
        confirmLabel={maintenance?'Resume':'Enter maintenance'}
        onCancel={()=>setShowMaint(false)}
        onConfirm={()=>{ onSetMaintenance && onSetMaintenance(!maintenance); setShowMaint(false); }}/>}
      {showSignIn && (auth && auth.SignInModal
        ? <auth.SignInModal onClose={()=>setShowSignIn(false)} onSignIn={(op)=>{ signInFn(op); setShowSignIn(false); }}/>
        : <SignInModal onClose={()=>setShowSignIn(false)} onSignIn={(op)=>{ signInFn(op); setShowSignIn(false); }}/>)}
      {showSignOut && <Confirm title="Sign out" body={`Sign out ${ses?ses.name:''}? Unsaved control changes are discarded.`} confirmLabel="Sign out" danger onCancel={()=>setShowSignOut(false)} onConfirm={()=>{ signOutFn(); setShowSignOut(false); }}/>}
      {showAlerts && <AlertsModal alerts={alerts} esc={esc} setEsc={setEsc} onAck={ackFn} onAckAll={ackAllFn} onClose={()=>setShowAlerts(false)}/>}
      {showHistory && <HistoryModal onClose={()=>setShowHistory(false)}/>}
    </div>
  );
}

export { Dashboard };
