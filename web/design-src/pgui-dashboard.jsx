// pgui-dashboard.jsx — the web dashboard (paper). Tabs:
// Dashboard · Shelf Management · Environmental · Controls

const DB_TABS = ['Dashboard','Shelf Management','Environmental','Controls'];

// ---------- DASHBOARD (overview + log) ----------
function OverviewTab({ shelves, reservoir, rackStatus, log, alerts, onOpenAlerts, onOpenHistory }){
  const active = shelves.filter(s=>s.active);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;
  const issues = shelves.filter(s=>s.active && (s.status==='Monitoring'||s.status==='Dry'||s.status==='Sensor Fault'));
  const cells = [
    { k:'Rack Status', v:rackStatus, dot:rs.col, n:'All subsystems nominal' },
    { k:'Reservoir', v:reservoir, unit:'%', n:'≈ 6 days remaining' },
    { k:'Active Shelves', v:active.length, unit:' / 4', n:'1 bay unassigned' },
    { k:'Water · Today', v:'7.4', unit:' L', n:'3 cycles logged' },
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
          <div className="ph"><div className="pt">Reservoir</div><div className="pm">120 L Tank</div></div>
          <div className="pb">
            <div style={{display:'flex',alignItems:'flex-end',gap:14}}>
              <div style={{flex:'none',width:64,height:130,border:'1px solid var(--ink)',borderRadius:3,background:'var(--bone)',position:'relative',overflow:'hidden'}}>
                <div style={{position:'absolute',left:0,right:0,bottom:0,height:reservoir+'%',background:'linear-gradient(180deg,var(--river),var(--river-deep))'}}></div>
                <div style={{position:'absolute',inset:0,background:'repeating-linear-gradient(0deg,transparent 0 calc(20% - 1px),#1B181333 calc(20% - 1px) 20%)'}}></div>
              </div>
              <div>
                <div style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:42,lineHeight:0.9}}>{reservoir}<span style={{fontSize:20,color:'var(--char)'}}>%</span></div>
                <div className="mono" style={{fontSize:10,letterSpacing:'0.14em',textTransform:'uppercase',color:'#1B181399',marginTop:6,lineHeight:1.7}}>
                  ≈ 94 L on hand<br/>≈ 6 days remaining<br/>Trend · steady
                </div>
              </div>
            </div>
            <div style={{marginTop:16,paddingTop:14,borderTop:'1px solid var(--rule-soft)'}} className="mono">
              <div style={{display:'flex',alignItems:'center',gap:9,fontSize:11,letterSpacing:'0.14em',textTransform:'uppercase'}}>
                <span style={{width:9,height:9,borderRadius:'50%',background:'var(--sage)'}}></span> Level nominal
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---------- SHELF MANAGEMENT ----------
function ShelfCard({ s, onAssign, role }){
  const meta = PG.STATUS[s.status];
  const inBand = s.moisture>=s.lo && s.moisture<=s.hi;
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
            <div><div className="mk">Moisture</div><div className="mv">{s.moisture}<span>%RH</span></div></div>
            <div><div className="mk">Temperature</div><div className="mv">{s.temp}<span>°F</span></div></div>
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
            <div><div className="k">Last Water</div><div className="v">{s.lastWater}</div></div>
            <div><div className="k">Usage · 24h</div><div className="v">{s.usage}</div></div>
          </div>
        </React.Fragment>
      ) : (
        <div style={{padding:'26px 18px 22px',textAlign:'center'}}>
          <div style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',fontSize:'15px',lineHeight:1.4}}>
            Bay available — assign a specimen group to bring Shelf {s.n} online.
          </div>
          <button className="fx-btn" style={{marginTop:'14px'}} disabled={!role||role==='Viewer'} onClick={()=>onAssign && onAssign(s.n)}>
            {(!role||role==='Viewer') ? 'Sign in to assign' : 'Assign Specimen'}
          </button>
        </div>
      )}
    </div>
  );
}

function ShelvesTab({ shelves, onAssign, role }){
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Shelf Management</div>
        <div className="sub">One card per growing shelf · physical bays 01–04</div>
      </div>
      <div className="shelf-grid">
        {shelves.map(s=> <ShelfCard key={s.n} s={s} onAssign={onAssign} role={role}/> )}
      </div>
    </div>
  );
}

// ---------- ENVIRONMENTAL ----------
function EnvironmentalTab(){
  const [period, setPeriod] = React.useState('7 Days');
  const periods = ['24 Hours','7 Days','30 Days','Season'];
  const seed = periods.indexOf(period)+1;
  const n = 28;
  const moist = [
    { color:'var(--river)', fill:true, data:PG.series(seed*1.1, n, 67, 12, 2) },
    { color:'var(--ochre)', data:PG.series(seed*2.3, n, 55, 14, 4) },
    { color:'var(--sage)', data:PG.series(seed*3.7, n, 70, 9, -2) },
  ];
  const temp = [
    { color:'var(--oxblood)', fill:true, data:PG.series(seed*4.2, n, 75, 6, 1) },
  ];
  const waterEvents = PG.series(seed*5.5, 14, 3, 5, 0).map(v=>Math.max(0,Math.round(v*10)/10));
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
          <div className="ch-t"><span className="sw" style={{background:'var(--river)'}}></span>Soil Moisture · %RH</div>
          <div className="ch-leg">
            <span><i style={{background:'var(--river)'}}></i>Shelf 01</span>
            <span><i style={{background:'var(--ochre)'}}></i>Shelf 02</span>
            <span><i style={{background:'var(--sage)'}}></i>Shelf 03</span>
          </div>
        </div>
        <div className="cb"><LineChart datasets={moist} yMin={40} yMax={85} fmtY={v=>v+'%'}/></div>
      </div>

      <div className="chart-card">
        <div className="ch-h">
          <div className="ch-t"><span className="sw" style={{background:'var(--oxblood)'}}></span>Soil Temperature · °F</div>
          <div className="ch-leg"><span><i style={{background:'var(--oxblood)'}}></i>Rack mean</span></div>
        </div>
        <div className="cb"><LineChart datasets={temp} yMin={68} yMax={82} fmtY={v=>v+'°'}/></div>
      </div>

      <div className="chart-card">
        <div className="ch-h">
          <div className="ch-t"><span className="sw" style={{background:'var(--river-deep)'}}></span>Watering Events · litres</div>
          <div className="ch-leg"><span className="mono">{period}</span></div>
        </div>
        <div className="cb"><EventBars data={waterEvents} color="var(--river-deep)"/></div>
      </div>
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

function ControlsTab({ role }){
  const readonly = !role || role==='Viewer';
  const [irr, setIrr] = React.useState({ target:70, min:58, dur:45, cool:6 });
  const [shelf, setShelf] = React.useState('01');
  const [scope, setScope] = React.useState('this');
  const [photo, setPhoto] = React.useState({ on:6, off:20 });
  const [dirty, setDirty] = React.useState(false);
  const [saved, setSaved] = React.useState(false);
  const [confirm, setConfirm] = React.useState(false);
  const litLeft = (photo.on/24)*100, litW = ((photo.off-photo.on)/24)*100;
  const set = (k,v)=>{ setIrr(p=>({...p,[k]:v})); setDirty(true); setSaved(false); };
  const setP = (k,v)=>{ setPhoto(p=>({...p,[k]:v})); setDirty(true); setSaved(false); };
  const valid = irr.target > irr.min && photo.off > photo.on;
  const apply = ()=>{ setConfirm(false); setDirty(false); setSaved(true); };
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
          {['01','02','03'].map(sh=>(
            <button key={sh} className={sh===shelf?'on':''} onClick={()=>setShelf(sh)} disabled={readonly}>Shelf {sh}</button>
          ))}
        </div>
      </div>
      <div className="ctrl-grid">
        <div className="panel">
          <div className="ph"><div className="pt">Irrigation</div><div className="pm">Shelf {shelf}</div></div>
          <div className="pb">
            <Slider label="Moisture target" sub="Pump rests above this" value={irr.target} min={40} max={90} unit="%" onChange={v=>set('target',v)} disabled={readonly}/>
            <Slider label="Moisture minimum" sub="Cycle triggers below this" value={irr.min} min={30} max={80} unit="%" onChange={v=>set('min',v)} disabled={readonly}/>
            <Slider label="Watering duration" sub="Valve open time" value={irr.dur} min={10} max={120} unit="s" onChange={v=>set('dur',v)} disabled={readonly}/>
            <Slider label="Cooldown period" sub="Minimum rest between cycles" value={irr.cool} min={1} max={24} unit="h" onChange={v=>set('cool',v)} disabled={readonly}/>
            {irr.target<=irr.min && <div className="fx-warn" style={{marginTop:6}}>Target must exceed minimum</div>}
          </div>
        </div>

        <div className="panel" style={{alignSelf:'start'}}>
          <div className="ph"><div className="pt">Lighting · Photoperiod</div><div className="pm">Shelf {shelf}</div></div>
          <div className="pb">
            <Slider label="On time" value={photo.on} min={0} max={12} unit="h" onChange={v=>setP('on',v)} disabled={readonly}/>
            <Slider label="Off time" value={photo.off} min={13} max={23} unit="h" onChange={v=>setP('off',v)} disabled={readonly}/>
            <div className="ctrl-row"><div className="cl">Daily hours<small>Computed</small></div><div className="cv">{photo.off-photo.on}<span>h</span></div></div>
            <div className="photo-rail">
              <div className="photo-track">
                <div className="lit" style={{left:litLeft+'%',width:litW+'%'}}></div>
                <div className="sun" style={{left:((photo.on+photo.off)/2/24)*100+'%'}}></div>
              </div>
              <div className="photo-scale"><span>00:00</span><span>06:00</span><span>12:00</span><span>18:00</span><span>24:00</span></div>
            </div>
            <div className="ctrl-row" style={{marginTop:4}}>
              <div className="cl">Manual override<small>Force lights on now</small></div>
              <ManualToggle/>
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
        <button className="fx-btn primary" style={{marginLeft:'auto'}} disabled={readonly||!dirty||!valid} onClick={()=>setConfirm(true)}>{saved?'Saved ✓':'Review & Apply'}</button>
      </div>
      {confirm && <Confirm title="Apply parameters"
        body={`Apply irrigation & lighting to ${scope==='all'?'all active shelves':'Shelf '+shelf}? Target ${irr.target}%RH (min ${irr.min}%), ${irr.dur}s valve, ${irr.cool}h cooldown; photoperiod ${String(photo.on).padStart(2,'0')}:00–${String(photo.off).padStart(2,'0')}:00.`}
        confirmLabel="Apply" onConfirm={apply} onCancel={()=>setConfirm(false)}/>}
    </div>
  );
}

function ManualToggle(){
  const [on,setOn]=React.useState(false);
  return <div className={'toggle'+(on?' on':'')} onClick={()=>setOn(!on)}><i></i></div>;
}

function Dashboard({ shelves, reservoir, rackStatus, log, session, onSignIn, onSignOut, alerts=[], onAck, onAckAll, connection='online', assignments, onAssign }){
  const [tab, setTab] = React.useState('Dashboard');
  const [assignFor, setAssignFor] = React.useState(null);
  const [showSignIn, setShowSignIn] = React.useState(false);
  const [showSignOut, setShowSignOut] = React.useState(false);
  const [showAlerts, setShowAlerts] = React.useState(false);
  const [showHistory, setShowHistory] = React.useState(false);
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

  return (
    <div className="db">
      {connection==='offline' && <OfflineBanner/>}
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
        {ses
          ? <div className="db-user" onClick={()=>setShowSignOut(true)} title="Sign out">
              <span style={{width:9,height:9,borderRadius:'50%',background:'var(--sage)'}}></span>
              <div className="who"><b>{ses.name}</b><span className="role">{ses.role}</span></div>
            </div>
          : <div className="db-user" onClick={()=>setShowSignIn(true)}>Sign In</div>}
        <div className="db-statuschip">
          <span className="d" style={{background:rs.col}}></span>{rackStatus}
        </div>
      </div>
      <div className="db-body">
        {tab==='Dashboard' && <OverviewTab shelves={effShelves} reservoir={reservoir} rackStatus={rackStatus} log={log} alerts={alerts} onOpenAlerts={()=>setShowAlerts(true)} onOpenHistory={()=>setShowHistory(true)}/>}
        {tab==='Shelf Management' && <ShelvesTab shelves={effShelves} onAssign={setAssignFor} role={role}/>}
        {tab==='Environmental' && <EnvironmentalTab/>}
        {tab==='Controls' && <ControlsTab role={role}/>}
      </div>

      {assignFor && <AssignModal shelfN={assignFor} onClose={()=>setAssignFor(null)} onAssign={(n,f)=>{ assignFn(n,f); setAssignFor(null); }}/>}
      {showSignIn && <SignInModal onClose={()=>setShowSignIn(false)} onSignIn={(op)=>{ signInFn(op); setShowSignIn(false); }}/>}
      {showSignOut && <Confirm title="Sign out" body={`Sign out ${ses?ses.name:''}? Unsaved control changes are discarded.`} confirmLabel="Sign out" danger onCancel={()=>setShowSignOut(false)} onConfirm={()=>{ signOutFn(); setShowSignOut(false); }}/>}
      {showAlerts && <AlertsModal alerts={alerts} esc={esc} setEsc={setEsc} onAck={ackFn} onAckAll={ackAllFn} onClose={()=>setShowAlerts(false)}/>}
      {showHistory && <HistoryModal onClose={()=>setShowHistory(false)}/>}
    </div>
  );
}

Object.assign(window, { Dashboard });
