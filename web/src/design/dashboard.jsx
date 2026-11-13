import React from 'react';
import { PG, ForestMark, EventBars } from './data.jsx';
import { AssignModal, SignInModal, Confirm, AlertsModal, HistoryModal, AlertBar, OfflineBanner, EstopBanner, MaintenanceBanner } from './flows.jsx';
import { ControlsTab } from './controls.jsx';
import { TimeChart } from './timechart.jsx';
import { DialG } from './rack3.jsx';

// pgui-dashboard.jsx — the web dashboard (paper). Tabs:
// Dashboard · Shelf Management · Environmental · Controls

const DB_TABS = ['Dashboard','Shelf Management','Environmental','Controls'];

// ---------- DASHBOARD (overview + log) ----------
function OverviewTab({ shelves, reservoir, reservoirOnline=true, rackStatus, lockouts=[], log, alerts, onOpenAlerts, onOpenHistory }){
  const active = shelves.filter(s=>s.active);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;
  const issues = shelves.filter(s=>s.active && (s.status==='Monitoring'||s.status==='Dry'||s.status==='Sensor Fault'));
  const vacant = shelves.length - active.length;
  const watering = shelves.filter(s=>s.status==='Watering'||s.status==='Draining').length;
  // audit 3.11: a lockout names itself in the status cell
  const statusNote = rackStatus==='Operational' ? 'All subsystems nominal'
    : rackStatus==='Locked Out' ? 'Pump locked out · '+(lockouts.length ? lockouts.join(', ').replace(/_/g,' ') : 'see safety')
    : 'Check diagnostics';
  const cells = [
    { k:'Rack Status', v:rackStatus, dot:rs.col, n: statusNote },
    // audit 3.9: a stale level probe shows "—" and says so; days are an estimate
    reservoirOnline
      ? { k:'Reservoir', v:reservoir, unit:'%', n:`est. ${PG.tankDays(reservoir)} days remaining` }
      : { k:'Reservoir', v:'—', unit:'%', n:'level sensor offline' },
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
                  <span className="ts">{e.ts}</span>
                  <div className="ev"><span className="sev" style={{background:e.sev}}></span><div>{e.ev}<em>{e.em}</em></div></div>
                </div>
              ))}
            </div>
          </div>
        </div>
        <div className="panel" style={{alignSelf:'start'}}>
          <div className="ph"><div className="pt">Reservoir</div><div className="pm">{PG.TANK_L} L Tank · {Math.round(PG.TANK_L/3.785)} gal</div></div>
          <div className="pb">
            <div className="db-glass">
              <div className="tube">
                <i style={{height:(reservoirOnline?reservoir:0)+'%'}}></i>
                <div className="tk"></div>
                <div className="floor"></div>
              </div>
              <div className="rd">
                <div className="pc">{reservoirOnline?reservoir:'—'}<span>%</span></div>
                <div className="est">
                  {reservoirOnline
                    ? <React.Fragment>est. {PG.tankLiters(reservoir)} L on hand<br/>est. {PG.tankDays(reservoir)} days remaining<br/>Estimates · uncalibrated flow</React.Fragment>
                    : <React.Fragment>level sensor offline<br/>no fresh reading<br/>check the reservoir probe</React.Fragment>}
                </div>
              </div>
            </div>
            <div className="db-glass-st">
              <span className="db-chip" style={{color:!reservoirOnline?'var(--oxblood)':reservoir>20?'var(--sage)':'var(--oxblood)'}}>
                <i className="dia"></i>{!reservoirOnline?'Sensor offline':reservoir>20?'Level nominal':'Refill required'}
              </span>
              <span className="k">lockout floor · 20%</span>
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
          <div className="nm">{s.species}{s.latin ? <em className="lat"> {s.latin}</em> : null}</div>
          <div className="sc-badge" style={{color:meta.inkCol}}>{s.status}</div>
        </div>
      </div>
      {s.active ? (
        <React.Fragment>
          {/* 2026-11-13 (console re-skin): the touch screen's dial G with the
              three readings beside it, in place of the bar graph. */}
          <div className="sc-dial">
            <DialG value={s.moisture} lo={s.lo} hi={s.hi} active={!!s.probe} hasBand={s.hasRule!==false} size={104}/>
            <div className="reads">
              <div><div className="k">Moisture</div><div className="v">{s.probe ? s.moisture : '—'}<small>%VWC</small></div><div className="b">{s.hasRule===false ? 'target · not set' : `target ${s.lo}–${s.hi}`}</div></div>
              <div><div className="k">Root Zone Temp</div><div className="v">{s.temp>0 ? s.temp : '—'}<small>°F</small></div><div className="b">{s.tempBand ? `band ${s.tempBand[0]}–${s.tempBand[1]}` : 'band · not set'}</div></div>
              <div><div className="k">Substrate EC</div><div className="v">{s.ec!=null ? s.ec : '—'}<small style={{textTransform:'none'}}>µS/cm</small></div><div className="b">{s.ecBand ? `band ${s.ecBand[0]}–${s.ecBand[1]}` : 'band · not set'}</div></div>
            </div>
          </div>
          <div className="sc-foot">
            <div><div className="k">Lighting</div><div className="v">{s.light?'On · '+s.on:'Off'}</div></div>
            <div>
              <div className="k">Last Water</div>
              <div className="v">{s.lastWater}</div>
              {s.lastWaterDate && <div className="k" style={{marginTop:3,letterSpacing:'0.08em'}}>{s.lastWaterDate}</div>}
            </div>
            <div><div className="k">Last cycle</div><div className="v">{s.usage}</div></div>
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
        <div className="sub">One card per growing shelf · bays 01–03</div>
      </div>
      <div className="shelf-grid">
        {shelves.map(s=> <ShelfCard key={s.n} s={s} onAssign={onAssign} onEdit={onEdit}
          onSetEnabled={onSetEnabled} onUnassign={onUnassign} role={role}/> )}
      </div>
    </div>
  );
}

// ---------- ENVIRONMENTAL ----------
// With getSeries (the live build) the charts render the backend's readings:
// 5-minute buckets for the 24-hour view, hourly aggregates beyond. Austin
// (2026-11-11): line charts, not dials — a real date axis that scales with
// the window, the period's peak and low labelled, hover for the numbers.
const HOURS = { '24 Hours':24, '7 Days':168, '30 Days':720, 'Season':2160 };
function MetricChart({ title, unit, color, series, y, range, fmtY, period, runs=[], now=null }){
  const tinted = series.map(s=>({ ...s, color }));
  const tgt = tinted.find(s=>s.target);
  return (
    <div className="chart-card">
      <div className="ch-h">
        <div className="ch-t"><span className="sw" style={{background:color}}></span>{title} · <span style={{textTransform:'none'}}>{unit}</span>
          {/* The live reading, right by the label (Austin, 2026-11-11). */}
          <span className="ch-now" style={{color}}>{now!=null ? <React.Fragment><small>now</small>{now}</React.Fragment> : <small>now —</small>}</span>
        </div>
        <div className="ch-leg">
          {tgt ? <span>ideal {tgt.target[0]}–{tgt.target[1]}</span> : <span>no target set</span>}
          <span>avg · dashed</span>
          <span><i style={{background:'var(--river-deep)',width:6,height:8,borderRadius:'50% 50% 50% 50% / 30% 30% 70% 70%'}}></i>watered</span>
          <span className="mono">{period}</span>
        </div>
      </div>
      <div className="cb"><TimeChart series={tinted} runs={runs} fromMs={range.fromMs} toMs={range.toMs} yMin={y[0]} yMax={y[1]} fmtY={fmtY} unit={unit==='%VWC'?'%':unit==='°F'?'°':''} h={200}/></div>
    </div>
  );
}
// Each shelf gets its own section — its three traces, nothing else's.
function ShelfSection({ ps, shelf, range, period }){
  return (
    <div className="env-shelf">
      <div className="env-shelf-h"><span className="sw" style={{background:shelf.stripe||'var(--river)'}}></span>Shelf {ps.n} · <span style={{textTransform:'none'}}>{shelf.species||''}</span></div>
      <MetricChart title="Soil Moisture" unit="%VWC" color="var(--river)" period={period} range={range} runs={ps.runs}
        series={ps.moist.series} y={ps.moist.y} fmtY={v=>v} now={shelf.probe ? shelf.moisture : null}/>
      <MetricChart title="Root Zone Temperature" unit="°F" color="var(--ochre-deep)" period={period} range={range} runs={ps.runs}
        series={ps.temp.series} y={ps.temp.y} fmtY={v=>v} now={shelf.probe && shelf.temp>0 ? shelf.temp : null}/>
      <MetricChart title="Substrate EC" unit="µS/cm" color="var(--sage-deep)" period={period} range={range} runs={ps.runs}
        series={ps.ec.series} y={ps.ec.y} fmtY={v=>Math.round(v)} now={shelf.probe && shelf.ec!=null ? shelf.ec : null}/>
    </div>
  );
}

function EnvironmentalTab({ getSeries, shelves=[] }){
  const [period, setPeriod] = React.useState('7 Days');
  const periods = ['24 Hours','7 Days','30 Days','Season'];
  const [live, setLive] = React.useState(null);
  // One shelf at a time (Austin, 2026-11-11): each shelf is its own sub-page.
  const [shelfN, setShelfN] = React.useState(null);
  React.useEffect(()=>{
    if(!getSeries) return;
    let alive = true;
    getSeries(period).then(d=>{ if(alive) setLive(d); }).catch(()=>{ if(alive) setLive(null); });
    return ()=>{ alive = false; };
  }, [period, getSeries]);

  const isLive = !!(getSeries && live);
  const perShelf = isLive && live.perShelf ? live.perShelf : [];
  const curN = perShelf.some(ps=>String(ps.n)===String(shelfN)) ? shelfN : (perShelf[0] ? perShelf[0].n : null);
  const range = isLive && live.range ? live.range : { fromMs: Date.now()-(HOURS[period]||168)*3600e3, toMs: Date.now() };
  const waterEvents = isLive ? live.water : PG.series((periods.indexOf(period)+1)*5.5, 14, 3, 5, 0).map(v=>Math.max(0,Math.round(v*10)/10));
  return (
    <div>
      <div className="db-h">
        <div className="ttl">Environmental Monitoring</div>
        <div className="env-controls">
          {perShelf.length>1 && (
            <div className="seg">
              {perShelf.map(ps=>(
                <button key={ps.n} className={String(ps.n)===String(curN)?'on':''} onClick={()=>setShelfN(ps.n)}>Shelf {ps.n}</button>
              ))}
            </div>
          )}
          <div className="seg">
            {periods.map(p=>(
              <button key={p} className={p===period?'on':''} onClick={()=>setPeriod(p)}>{p}</button>
            ))}
          </div>
        </div>
      </div>

      {perShelf.filter(ps=>String(ps.n)===String(curN)).map(ps=>(
        <ShelfSection key={ps.n} ps={ps} shelf={shelves.find(x=>String(x.n)===String(ps.n))||{}} range={range} period={period}/>
      ))}
      {!perShelf.length &&
        <div className="chart-card"><div className="cb" style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',padding:'26px 0',textAlign:'center'}}>No readings yet for this window — each assigned shelf gets its own section here.</div></div>}

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
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'var(--char)'}}>Shelf</div>
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'var(--char)'}}>Substrate EC</div>
              <div style={{fontSize:9,letterSpacing:'0.14em',textTransform:'uppercase',color:'var(--char)'}}>Root Zone Temp</div>
              {live.adherence.map(a=>(
                <React.Fragment key={a.n}>
                  <div>Shelf {a.n}</div>
                  <div>{a.ec
                    ? <span><b style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:15}}>{a.ec.pct}%</b> in {a.ecBand[0]}–{a.ecBand[1]} µS/cm <span style={{color:'var(--char)'}}>· {a.ec.n}h</span></span>
                    : <span style={{color:'var(--char)'}}>{a.ecBand?'awaiting data':'no band set'}</span>}</div>
                  <div>{a.temp
                    ? <span><b style={{fontFamily:'Zilla Slab',fontWeight:900,fontSize:15}}>{a.temp.pct}%</b> in {a.tempBand[0]}–{a.tempBand[1]}°F <span style={{color:'var(--char)'}}>· {a.temp.n}h</span></span>
                    : <span style={{color:'var(--char)'}}>{a.tempBand?'awaiting data':'no band set'}</span>}</div>
                </React.Fragment>
              ))}
            </div>
            <div style={{fontFamily:'Newsreader',fontStyle:'italic',color:'var(--char)',fontSize:13,marginTop:14,lineHeight:1.5}}>
              Share of hourly readings inside each shelf's target band over the selected period — for historical cross-checking against plant health. Monitoring only; the unit senses substrate EC &amp; root zone temperature but doesn't control them. Each shelf is scored against its own probe.
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------- CONTROLS ----------
// Deep-linkable tabs: "/#environmental", "/#controls", "/#shelves".
// Build AssignModal initial values from a live shelf (quick-edit).
function editInitial(s){
  const raw = s.raw || {};
  return {
    species: raw.species1 || s.species,
    species2: raw.species2 || '',
    lo: s.lo, hi: s.hi,
    durMin: raw.durMin || 5,
    // change 1: carry the shelf's real drain-back time into the edit modal.
    drainMin: raw.drainMin,
    // Post-drain purge: carry the stored values so a save never resets them.
    purgeAfterMin: raw.purgeAfterMin, purgeSec: raw.purgeSec,
    // change 3: EC/temp happy-bands, applied profile, and moisture guards.
    ecLo: raw.ecLo, ecHi: raw.ecHi, tempLo: raw.tempLo, tempHi: raw.tempHi,
    profileId: raw.profileId, stage: raw.stage,
    coolMin: raw.coolMin, maxCycles: raw.maxCycles,
  };
}

function initialTab(){
  const h = (typeof location !== 'undefined' ? location.hash : '').replace('#','').toLowerCase();
  return { dashboard:'Dashboard', shelves:'Shelf Management', shelf:'Shelf Management',
    environmental:'Environmental', controls:'Controls' }[h] || 'Dashboard';
}

// `auth` (live build): { SignInModal, canManage, openOperators } — swaps the
// demo PIN sign-in for the real one and adds operator management for admins.
function Dashboard({ shelves, reservoir, reservoirOnline=true, rackStatus, lockouts=[], log, session, onSignIn, onSignOut, alerts=[], onAck, onAckAll, connection='online', lastSync='—', assignments, onAssign, getSeries, controls, auth, maintenance=false, onSetMaintenance, onSetEnabled, onUnassign, profiles, toast=null, onDismissToast, estop=false, onEmergencyStop, onSafetyReset, signInRequest=0 }){
  const [tab, setTab] = React.useState(initialTab);
  const [assignFor, setAssignFor] = React.useState(null);
  const [showSignIn, setShowSignIn] = React.useState(false);
  const [showEstop, setShowEstop] = React.useState(false); // audit 3.3: confirm e-stop / reset
  // audit 3.10/3.11: the live build bumps signInRequest on any 401 → reopen sign-in
  React.useEffect(()=>{ if(signInRequest) setShowSignIn(true); }, [signInRequest]);
  const [showSignOut, setShowSignOut] = React.useState(false);
  const [showAlerts, setShowAlerts] = React.useState(false);
  const [showHistory, setShowHistory] = React.useState(false);
  const [showMaint, setShowMaint] = React.useState(false); // change 2: confirm dialog
  const [esc, setEsc] = React.useState(PG.ESCALATION_DEFAULT);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;

  // fallbacks so the component also works standalone (e.g. the handoff figure)
  const [locAssign, setLocAssign] = React.useState({});
  const [locSession, setLocSession] = React.useState({ name:'Steward', role:'Steward' });
  const ses = session !== undefined ? session : locSession;
  const assignFn = onAssign || ((n,f)=>setLocAssign(a=>({...a,[n]:f})));
  const signInFn = onSignIn || setLocSession;
  const signOutFn = onSignOut || (()=>setLocSession(null));
  const ackFn = onAck || (()=>{});
  const ackAllFn = onAckAll || (()=>{});
  const effShelves = onAssign ? shelves : shelves.map(s=> locAssign[s.n] ? {...s, ...locAssign[s.n], active:true} : s);
  const role = ses ? ses.role : null;

  const canToggleMaint = role && role!=='Viewer' && onSetMaintenance;
  const canStop = role && role!=='Viewer' && (onEmergencyStop || onSafetyReset);
  return (
    <div className="db">
      {connection==='offline' && <OfflineBanner lastSync={lastSync}/>}
      {/* audit 3.3: e-stop banner outranks maintenance. */}
      {estop && <EstopBanner canManage={!!(canStop && onSafetyReset)} onReset={()=>setShowEstop(true)}/>}
      {/* change 2: maintenance banner sits under any offline banner. */}
      {maintenance && <MaintenanceBanner canManage={!!canToggleMaint} onExit={()=>setShowMaint(true)}/>}
      <div className="db-top">
       <div className="db-mast">
        <div className="db-brand">
          <ForestMark size={46}/>
        </div>
        <div className="db-mh">
          <div className="eye"><span className="dia"></span>Dept. of Forestry · Field Station<span className="dia"></span></div>
          <div className="ttl">Propagation Unit · No. 01</div>
        </div>
        <div className="db-chips">
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
        {/* audit 3.3: emergency stop / reset safety (Grower+). Confirms both ways. */}
        {canStop &&
          <div className={'db-user db-estop'+(estop?' active':'')} onClick={()=>setShowEstop(true)}
            title={estop?'Reset the safety supervisor':'Emergency stop — all outputs off'}>
            {estop?'Reset safety':'E-STOP'}
          </div>}
        </div>
       </div>
       <div className="db-tabbar">
        <div className="db-tabs">
          {DB_TABS.map(t=>(
            <button key={t} className={'db-tab'+(t===tab?' on':'')} onClick={()=>setTab(t)}>{t}</button>
          ))}
        </div>
        <div className="db-statuschip">
          <span className="d" style={{background:rs.col}}></span>{rackStatus}
        </div>
       </div>
      </div>
      {/* audit 3.1: the last rejected command, under the top bar. */}
      {toast && <div className="db-toast" onClick={onDismissToast} role="status"><span className="d"></span>{toast.text}</div>}
      <div className="db-body">
        {tab==='Dashboard' && <OverviewTab shelves={effShelves} reservoir={reservoir} reservoirOnline={reservoirOnline} rackStatus={rackStatus} lockouts={lockouts} log={log} alerts={alerts} onOpenAlerts={()=>setShowAlerts(true)} onOpenHistory={()=>setShowHistory(true)}/>}
        {tab==='Shelf Management' && <ShelvesTab shelves={effShelves}
          onAssign={(n)=>setAssignFor({n})}
          onEdit={(s)=>setAssignFor({n:s.n, initial:editInitial(s)})}
          onSetEnabled={onSetEnabled}
          onUnassign={onUnassign}
          role={role}/>}
        {tab==='Environmental' && <EnvironmentalTab getSeries={getSeries} shelves={effShelves}/>}
        {tab==='Controls' && <ControlsTab role={role} controls={controls}/>}
      </div>

      {assignFor && <AssignModal shelfN={assignFor.n} initial={assignFor.initial} onClose={()=>setAssignFor(null)} onAssign={(n,f)=>{ assignFn(n,f); setAssignFor(null); }}
        onClear={onUnassign ? (n)=>{ onUnassign(n); setAssignFor(null); } : undefined}
        onDisable={onSetEnabled ? (n)=>{ onSetEnabled(n, false); setAssignFor(null); } : undefined}
        profiles={profiles}/>}
      {/* audit 3.3: confirm e-stop / reset. */}
      {showEstop && <Confirm
        title={estop?'Reset safety':'Emergency stop'}
        body={estop
          ? 'Clear the emergency stop? Automation resumes; any lockouts still in force stay until their condition clears.'
          : 'Stop everything now? Pump, valves and lighting are switched off and held off until a Grower or Steward resets safety.'}
        confirmLabel={estop?'Reset':'E-STOP'} danger={!estop}
        onCancel={()=>setShowEstop(false)}
        onConfirm={()=>{ (estop ? onSafetyReset : onEmergencyStop)?.(); setShowEstop(false); }}/>}
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
