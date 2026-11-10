import React from 'react';
import { PG } from './data.jsx';
import { LockScreen } from './flows.jsx';

// pgui-rack2.jsx — ROUND 2: sculptural instrument-panel treatment.
// Same functionality (tap shelf → detail, Water Now, reservoir, two layouts)
// re-imagined as a flat, bilaterally-symmetric apparatus face: framed,
// engraved labels, needle dials, diamond ornaments. Honors the layout tweak.

const SC2 = {
  Healthy:'var(--rk2-accent)', Monitoring:'var(--ochre-l)', Watering:'var(--river-l)',
  Draining:'var(--river-l)', // live-edit (ebb-flow)
  Dry:'var(--ochre-l)', 'Sensor Fault':'var(--oxblood-l)', Inactive:'var(--sfaint)',
  Maintenance:'var(--ochre-l)', Disabled:'var(--sfaint)', // change 2
  'Locked Out':'var(--oxblood-l)' // audit 3.11
};
const RS2 = {
  Operational:'var(--rk2-accent)', Watering:'var(--river-l)', Draining:'var(--river-l)', // live-edit (ebb-flow)
  Fault:'var(--oxblood-l)', Maintenance:'var(--ochre-l)', 'Locked Out':'var(--oxblood-l)' // audit 3.11
};

// Audit 3.2: the engine's cycle phase, in the panel's own words.
const CYCLE_LABEL = {
  precheck:'Prechecks', opening_valve:'Opening valve', starting_pump:'Starting pump',
  flooding:'Pump active · valve open', stopping_pump:'Stopping pump',
  draining:'Draining · valve open · pump off', closing_valve:'Closing valve',
};
function cycleLabel(cycle, fallback){
  if(!cycle) return fallback;
  const txt = CYCLE_LABEL[cycle.state] || cycle.state || fallback;
  return cycle.duration ? `${txt} · ${cycle.duration} s` : txt;
}
const fmtUntil = (iso)=>{ const d=new Date(iso); return isNaN(d) ? '' : String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0'); };
// Audit 3.11: no target band without a moisture rule.
const targetText = (s)=> s.hasRule===false ? 'Target · not set' : `Target · ${s.lo}–${s.hi}`;
const tempText = (s)=> s.temp>0 ? `${s.temp}°F` : '—';

// flat dial — 270° sweep, engraved ticks, target band. Two readouts:
// needle (home plates) or marker + center-readout (shelf detail). The refined
// marker treatment is claude-design's (v1 handoff), wired here to live bands.
function DialGauge({ value, lo, hi, accent, size=82, active=true, marker=false, bg='#2A443F' }){
  const cx=size/2, cy=size/2, r=size/2-6, start=135, sweep=270;
  const pol=(deg,rr)=>{const a=deg*Math.PI/180;return [cx+rr*Math.cos(a), cy+rr*Math.sin(a)];};
  const ticks=[];
  for(let i=0;i<=10;i++){
    const deg=start+sweep*(i/10), major=i%5===0;
    const [x1,y1]=pol(deg,r), [x2,y2]=pol(deg,r-(major?8:5));
    ticks.push(<line key={i} x1={x1.toFixed(1)} y1={y1.toFixed(1)} x2={x2.toFixed(1)} y2={y2.toFixed(1)} stroke="var(--sline)" strokeWidth={major?1.4:0.8}/>);
  }
  const arc=(d0,d1,rr)=>{const [x0,y0]=pol(d0,rr),[x1b,y1b]=pol(d1,rr);const large=(d1-d0)>180?1:0;
    return `M${x0.toFixed(1)} ${y0.toFixed(1)} A${rr} ${rr} 0 ${large} 1 ${x1b.toFixed(1)} ${y1b.toFixed(1)}`;};
  const v=Math.max(0,Math.min(100,value));
  const ndeg=start+sweep*(v/100), [nx,ny]=pol(ndeg,r-7);
  const [mo,moy]=pol(ndeg,r+3), [mi,miy]=pol(ndeg,r-7), [dx,dy]=pol(ndeg,r-3);
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{display:'block',margin:'0 auto'}}>
      <circle cx={cx} cy={cy} r={r} fill="none" stroke="var(--sline-soft)" strokeWidth="1"/>
      {active && <path d={arc(start+sweep*(lo/100), start+sweep*(hi/100), r-3)} fill="none" stroke={accent} strokeWidth={marker?4.5:2.5} opacity={marker?0.9:0.45} strokeLinecap="butt"/>}
      {ticks}
      {active && !marker && <line x1={cx} y1={cy} x2={nx.toFixed(1)} y2={ny.toFixed(1)} stroke={accent} strokeWidth="2.2" strokeLinecap="round"/>}
      {active && !marker && <circle cx={cx} cy={cy} r="3.2" fill={accent}/>}
      {active && marker && <line x1={mo.toFixed(1)} y1={moy.toFixed(1)} x2={mi.toFixed(1)} y2={miy.toFixed(1)} stroke="var(--sink)" strokeWidth="2.4" strokeLinecap="round"/>}
      {active && marker && <circle cx={dx.toFixed(1)} cy={dy.toFixed(1)} r="4.2" fill={accent} stroke={bg} strokeWidth="1.6"/>}
      {!active && <circle cx={cx} cy={cy} r="3.2" fill="var(--sfaint)"/>}
    </svg>
  );
}

function Plate({ s, onOpen }){
  const col = SC2[s.status] || 'var(--sfaint)';
  // change 2: only a truly empty bay is "Vacant"; a paused (assigned but
  // disabled) shelf falls through to the normal plate — dimmed, still tappable
  // so it can be re-enabled from the detail view.
  if(!s.active && !s.assigned) return (
    <div className="rk2-plate vacant">
      <div className="ph">Shelf · {s.n}</div>
      <div className="nm" style={{color:'var(--sfaint)'}}>Vacant Bay</div>
      <div className="dial"><DialGauge value={0} lo={s.lo} hi={s.hi} accent={col} size={100} active={false}/></div>
      <div className="rd noprobe" style={{color:'var(--sfaint)'}}>standby</div>
      <div className="tp">No specimen</div>
      <div className="tag" style={{color:'var(--sfaint)'}}><span className="d"></span>Vacant</div>
    </div>
  );
  return (
    <div className="rk2-plate" onClick={()=>onOpen(s)}>
      <div className="ph">Shelf · {s.n}</div>
      {/* audit 3.12: long names clamp to two lines inside the centred box */}
      <div className={'nm'+((s.species||'').length>22?' long':'')}><span className="clamp">{s.species}{s.latin ? <em className="lat"> {s.latin}</em> : null}</span></div>
      <div className="dial"><DialGauge value={s.moisture} lo={s.hasRule===false?0:s.lo} hi={s.hasRule===false?0:s.hi} accent={col} size={100} active={s.probe}/></div>
      {s.probe
        ? <div className="rd">{s.moisture} <small>%VWC</small></div>
        : <div className="rd noprobe">no probe</div>}
      {/* .tp is nowrap — with an EC reading the labelled form clips, so the
          probed shelf gets the compact "69°F · EC 1131" form instead. */}
      <div className="tp">{targetText(s)}<br/>{s.ec!=null ? `${tempText(s)} · EC ${s.ec}` : `Root Zone · ${tempText(s)}`}</div>
      <div className="tag" style={{color:col}}><span className="d"></span>{s.status}</div>
    </div>
  );
}

function Ledger({ shelves, onOpen }){
  return (
    <div className="rk2-ledger">
      <div className="rk2-lhead">
        <div>Shelf</div><div>Specimen</div><div>Moisture</div><div>Root Zone Temp</div><div>Status</div>
      </div>
      {shelves.map(s=>{
        const col=SC2[s.status] || 'var(--sfaint)';
        const inBand=s.moisture>=s.lo && s.moisture<=s.hi;
        if(!s.active && !s.assigned) return ( // change 2: only truly-empty bays
          <div className="rk2-lrow vacant" key={s.n}>
            <div><span className="rn" style={{color:'var(--sfaint)'}}>{s.n}</span></div>
            <div style={{gridColumn:'2 / -1',fontFamily:'Newsreader',fontStyle:'italic',fontSize:14}}>Vacant bay — unassigned</div>
          </div>
        );
        return (
          <div className="rk2-lrow" key={s.n} onClick={()=>onOpen(s)}>
            <div><span className="rn">{s.n}</span></div>
            <div><span className="sp">{s.species}{s.latin ? <small><em>{s.latin}</em></small> : null}</span></div>
            <div>
              <div style={{width:'100%'}}>
                <div className="rk2-mini"><i style={{width:s.moisture+'%',background:inBand?col:'var(--ochre-l)'}}></i>
                  <span className="tg" style={{left:s.lo+'%'}}></span><span className="tg" style={{left:s.hi+'%'}}></span></div>
                <div style={{fontSize:9,letterSpacing:'0.06em',color:'var(--sdim)',marginTop:5}}>{s.probe ? s.moisture+'%' : 'no probe'} · {s.hasRule===false ? 'not set' : `${s.lo}–${s.hi}`}</div>
              </div>
            </div>
            <div style={{fontSize:12,color:'var(--sink)'}}>{tempText(s)}</div>
            <div><span className="rk2-tag2" style={{color:col}}><span className="d"></span>{s.status}</span></div>
          </div>
        );
      })}
    </div>
  );
}

function Detail2({ s, onBack, onWater, onDrain, onStop, watering, busy=false, pending=false, theme, onLight, onFan, maintenance, onSetMaintenance, onSetEnabled, connection='online', lastSync='—' }){
  const col = watering ? 'var(--river-l)' : (SC2[s.status] || 'var(--sfaint)');
  const label = watering ? 'Watering' : s.status;
  const sev = watering ? PG.STATUS['Watering'].sev : (PG.STATUS[s.status] || PG.STATUS.Inactive).sev;
  // change 2: a disabled shelf can't be watered (engine rejects it); a manual
  // water while maintenance is on asks first.
  const disabled = !s.active;
  // Audit 3.6: offline → every control is inert and says so.
  const offline = connection==='offline';
  const [confirmWater, setConfirmWater] = React.useState(false);
  const tapWater = ()=>{ if(offline||pending) return; if(maintenance){ setConfirmWater(true); } else { onWater && onWater(); } };
  const guard = (fn)=>(...a)=>{ if(offline) return; fn && fn(...a); };
  const tN = (s.temp-60)/30*100;
  // marker-dot ring colour blends into the panel so the dot reads as "on top".
  const dbg = theme==='light' ? '#E7DFCA' : '#243029';
  // Live-wired: lighting is rack-wide (one relay), fans are a single-speed
  // ganged circuit — both render from live state, not local state.
  const lightOn = s.light;
  const fanOn = !!s.fan;
  return (
    <div className={"rk2-detail"+(theme==='light'?' light':'')}>
      <div className="rk2-grain"></div>
      <div className="rk2-frame"></div><div className="rk2-frame inner"></div>
      <span className="rk2-corner tl"></span><span className="rk2-corner tr"></span>
      <span className="rk2-corner bl"></span><span className="rk2-corner br"></span>
      <div className="rk2-dinner">
        <div className="rk2-dtop">
          <button className="rk2-plaque" onClick={onBack}>‹ Return</button>
          <span className="rk2-plaque" style={{color:col,borderColor:col}}><span style={{width:6,height:6,background:col,transform:'rotate(45deg)',display:'inline-block'}}></span>{label} · {sev}</span>
        </div>
        <div className="rk2-dname">
          <div className="nm">{s.species}{s.latin ? <em className="lat"> {s.latin}</em> : null}</div>
          <div className="tg">Shelf {s.n}{s.latin ? <React.Fragment> · <em>{s.latin}</em></React.Fragment> : null}</div>
        </div>
        {/* claude-design refined 3-dial detail (marker + centered readout),
            wired to the live moisture / temp / EC bands. no-probe → dimmed dash. */}
        <div className="rk2-dial3">
          <div className="rk2-dcell">
            <div className="k">Soil Moisture</div>
            <div className="rk2-dwrap">
              <DialGauge value={s.moisture} lo={s.hasRule===false?0:s.lo} hi={s.hasRule===false?0:s.hi} accent={col} size={90} marker active={s.probe} bg={dbg}/>
              <div className="rk2-readout">{s.probe ? <React.Fragment><b>{s.moisture}</b><small>%VWC</small></React.Fragment> : <b className="np">—</b>}</div>
            </div>
            <div className="rk2-dtgt">{s.hasRule===false ? <span className="np">Target · not set</span> : <React.Fragment>Target · <b>{s.lo}–{s.hi}%</b></React.Fragment>}</div>
          </div>
          <div className="rk2-dcell">
            <div className="k">Root Zone Temp</div>
            <div className="rk2-dwrap">
              <DialGauge value={tN} lo={s.tempBand?(s.tempBand[0]-60)/30*100:40} hi={s.tempBand?(s.tempBand[1]-60)/30*100:60} accent="var(--ochre-l)" size={90} marker active={s.temp>0} bg={dbg}/>
              <div className="rk2-readout">{s.temp>0 ? <React.Fragment><b>{s.temp}</b><small>°F</small></React.Fragment> : <b className="np">—</b>}</div>
            </div>
            <div className="rk2-dtgt">{s.tempBand ? <React.Fragment>Band · <b>{s.tempBand[0]}–{s.tempBand[1]}°F</b></React.Fragment> : <span className="np">Band · not set</span>}</div>
          </div>
          <div className="rk2-dcell">
            <div className="k">Substrate EC</div>
            <div className="rk2-dwrap">
              {/* dial spans 0–3000 µS/cm — the working range for propagation
                  substrate; the probe itself reads to 20,000. */}
              <DialGauge value={s.ec!=null ? Math.min(100,(s.ec/3000)*100) : 0} lo={s.ecBand?(s.ecBand[0]/3000)*100:(500/3000)*100} hi={s.ecBand?(s.ecBand[1]/3000)*100:(1500/3000)*100} accent="var(--river-l)" size={90} marker active={s.ec!=null} bg={dbg}/>
              <div className="rk2-readout">{s.ec!=null ? <React.Fragment><b>{s.ec}</b><small>µS/cm</small></React.Fragment> : <b className="np">—</b>}</div>
            </div>
            {/* keep µS/cm out of the caption's text-transform:uppercase — µ
                uppercases to Μ (mu) and the unit misreads as mS/cm. */}
            <div className="rk2-dtgt">{s.ecBand ? <React.Fragment>Band · <b>{s.ecBand[0]}–{s.ecBand[1]} <span style={{textTransform:'none'}}>µS/cm</span></b></React.Fragment> : <span className="np">Band · not set</span>}</div>
          </div>
        </div>
        <div className="rk2-controls2">
          <div className="rk2-light">
            {/* audit 3.10: a manual override is visible wherever lights are toggled */}
            <div className="ll"><span className="k">Lighting · Rack{s.lightOverride ? ' · manual' : ''}</span><span className="ph">{s.lightOverride ? `Forced ${s.lightOverride.mode}${s.lightOverride.until ? ' until '+fmtUntil(s.lightOverride.until) : ''}` : `${s.on} – ${s.off}`}</span></div>
            <div className="rk2-seg">
              <button className={lightOn?'on':''} disabled={offline} onClick={guard(()=>onLight && onLight(true))}>On</button>
              <button className={!lightOn?'on':''} disabled={offline} onClick={guard(()=>onLight && onLight(false))}>Off</button>
            </div>
          </div>
          <div className="rk2-light">
            <div className="ll"><span className="k">Fans · Rack</span><span className="ph">Circulation</span></div>
            <div className="rk2-seg">
              <button className={fanOn?'on':''} disabled={offline} onClick={guard(()=>onFan && onFan(true))}>On</button>
              <button className={!fanOn?'on':''} disabled={offline} onClick={guard(()=>onFan && onFan(false))}>Off</button>
            </div>
          </div>
        </div>
        <div className="rk2-water">
          {offline ? (
            // audit 3.6: the detail screen says it is offline instead of looking live
            <div className="rk2-plaque rk2-offline" style={{justifyContent:'center',width:'100%'}}>Controller offline — last reading {lastSync}</div>
          ) : busy ? (
            // audit 3.2/3.3/3.4: the zone is Watering/Draining — real phase text
            // from the engine, and Stop is the only irrigation control offered.
            <div className="rk2-wrow busy">
              <div className="rk2-pump"><div className="tk"></div><span className="lbl">{cycleLabel(s.cycle, s.status==='Draining' ? 'Draining' : 'Cycle running')}</span></div>
              <button className="rk2-wbtn rk2-dbtn rk2-stop" disabled={pending} onClick={()=>onStop && onStop()} title="Stop this cycle: pump off, valve closed">Stop</button>
            </div>
          ) : watering ? (
            <div className="rk2-pump"><div className="tk"></div><span className="lbl">{cycleLabel(s.cycle, 'Cycle accepted · starting')}</span></div>
          ) : disabled ? (
            <div className="rk2-plaque" style={{justifyContent:'center',width:'100%',color:'var(--sfaint)'}}>Shelf disabled — enable to water</div>
          ) : (
            <div className="rk2-wrow">
              <button className="rk2-wbtn" disabled={pending} onClick={tapWater}><span className="d"></span>{pending ? 'Sending…' : 'Water Now'}<span className="d"></span></button>
              {/* Drain (Austin, 2026-11-10): valve open for the drain-back time, pump locked off —
                  for a tray left full by an interrupted cycle. */}
              <button className="rk2-wbtn rk2-dbtn" disabled={pending} onClick={()=>{ if(!pending && onDrain) onDrain(); }} title="Open valve, pump off, drain back to reservoir">Drain</button>
            </div>
          )}
          {/* change 2: per-shelf disable toggle + system maintenance toggle */}
          <div className="rk2-actions">
            <button className="rk2-plaque" style={{justifyContent:'center'}} disabled={offline}
              onClick={guard(()=>onSetEnabled && onSetEnabled(s.n, disabled))}>
              {disabled ? 'Enable Shelf' : 'Disable Shelf'}
            </button>
            <button className="rk2-plaque" style={{justifyContent:'center',...(maintenance?{color:'var(--ochre-l)',borderColor:'var(--ochre-l)'}:{})}} disabled={offline}
              onClick={guard(()=>onSetMaintenance && onSetMaintenance(!maintenance))}>
              {maintenance ? 'Exit Maintenance' : 'Maintenance'}
            </button>
          </div>
        </div>
      </div>
      {/* change 2: confirm a hand-water while maintenance mode is on. */}
      {confirmWater && (
        <div className="rk2-confirm-back" onClick={()=>setConfirmWater(false)}
          style={{position:'absolute',inset:0,zIndex:30,background:'rgba(11,9,7,0.8)',backdropFilter:'blur(2px)',display:'flex',alignItems:'center',justifyContent:'center'}}>
          <div onClick={e=>e.stopPropagation()}
            style={{background:'#1B1812',border:'1.5px solid var(--ochre-l)',borderRadius:10,padding:'22px 24px',maxWidth:420,textAlign:'center',color:'var(--lcd)'}}>
            <div style={{fontFamily:'"Zilla Slab",serif',fontWeight:900,fontSize:18,marginBottom:8,color:'var(--ochre-l)'}}>Maintenance mode is on</div>
            <div style={{fontFamily:'"Newsreader",serif',fontSize:15,lineHeight:1.5,marginBottom:18}}>Water Shelf {s.n} by hand anyway? Automation stays suspended.</div>
            <div style={{display:'flex',gap:10,justifyContent:'center'}}>
              <button className="rk2-plaque" style={{justifyContent:'center'}} onClick={()=>setConfirmWater(false)}>Cancel</button>
              <button className="rk2-wbtn" style={{padding:'10px 20px'}} onClick={()=>{ setConfirmWater(false); onWater && onWater(); }}>Water Now</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function RackDisplay2({ shelves, reservoir, reservoirOnline=true, rackStatus, lockouts=[], layout, clock, day, onWater, onDrain, onStop, onShutdown, onLight, onFan, theme, locked, connection='online', onUnlock, initialOpen, onThemeToggle, maintenance=false, onSetMaintenance, onSetEnabled, toast=null, onDismissToast, lastSync='—' }){
  const [open, setOpen] = React.useState(initialOpen ? { n: initialOpen } : null);
  // Easter egg (Austin, 2026-11-10): press and hold the thin divider under the
  // title for 3 s to get the power-down popup. Nothing visible, nothing to
  // bump by accident. 'idle' | 'ask' | 'down'
  const [power, setPower] = React.useState('idle');
  const holdRef = React.useRef(null);
  const holdStart = () => { if (holdRef.current) return; holdRef.current = setTimeout(() => { holdRef.current = null; setPower('ask'); }, 3000); };
  const holdEnd = () => { if (holdRef.current) { clearTimeout(holdRef.current); holdRef.current = null; } };
  const doShutdown = async () => {
    setPower('down');
    const ok = onShutdown ? await onShutdown() : false;
    if (ok === false) setPower('idle');
  };
  const [watering, setWatering] = React.useState(false);
  const [pending, setPending] = React.useState(false);
  const pendingRef = React.useRef(false);
  const statusCol = RS2[rackStatus] || RS2.Operational;
  const current = open ? shelves.find(s=>s.n===open.n) : null;
  // Audit 3.4: the zone is busy for the whole cycle, fill and drain-back
  // alike — Water Now and Drain both hide, Stop is offered instead.
  const busy = !!(current && (current.status==='Watering' || current.status==='Draining'));
  // Audit 3.2: `watering` is only set once the backend accepted the command,
  // and clears as soon as the engine reports the zone busy (or after a short
  // grace period if it never does — e.g. the cycle was queued).
  React.useEffect(()=>{
    if(!watering) return;
    if(busy){ setWatering(false); return; }
    const t = setTimeout(()=>setWatering(false), 5000);
    return ()=>clearTimeout(t);
  }, [watering, busy]);
  React.useEffect(()=>{ if(!current) setWatering(false); }, [current]);
  // One in-flight irrigation command at a time; the button that fired stays
  // inert until its promise settles (success or rejection).
  const fire = (fn, shelf, onOk)=>{
    if(pendingRef.current || !fn || !shelf) return;
    pendingRef.current = true; setPending(true);
    Promise.resolve(fn(shelf))
      .then((ok)=>{ if(ok && onOk) onOk(); })
      .catch(()=>{})
      .then(()=>{ pendingRef.current = false; setPending(false); });
  };
  const doWater = ()=>{ if(busy) return; fire(onWater, current, ()=>setWatering(true)); };
  const doDrain = ()=>{ if(busy) return; fire(onDrain, current); };
  const doStop = ()=>{ fire(onStop, current, ()=>setWatering(false)); };
  // Audit 3.7/3.11: one status line carries offline / lockouts / maintenance,
  // so no extra banner competes with the three plates for height.
  const statusText = connection==='offline' ? 'Controller Offline'
    : rackStatus==='Locked Out' ? 'System Locked Out' + (lockouts.length ? ' · ' + lockouts.join(', ').replace(/_/g,' ') : '')
    : 'System ' + rackStatus + (maintenance && rackStatus!=='Maintenance' ? ' · maintenance' : '');

  return (
    <div className={"rk2-screen"+(theme==='light'?' light':'')}>
      <div className="rk2-grain"></div>
      <div className="rk2-frame"></div><div className="rk2-frame inner"></div>
      <span className="rk2-corner tl"></span><span className="rk2-corner tr"></span>
      <span className="rk2-corner bl"></span><span className="rk2-corner br"></span>

      {onThemeToggle && !current && (
        <button className="rk2-themebtn" onClick={onThemeToggle}
          aria-label={theme==='light'?'Switch to dark':'Switch to limestone'}>
          {theme==='light'
            ? <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M20 13.5A8 8 0 1 1 10.5 4 6.5 6.5 0 0 0 20 13.5Z"/></svg>
            : <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><circle cx="12" cy="12" r="4.4"/><path d="M12 2.5v2.6M12 18.9v2.6M2.5 12h2.6M18.9 12h2.6M5 5l1.8 1.8M17.2 17.2 19 19M19 5l-1.8 1.8M6.8 17.2 5 19"/></svg>}
        </button>
      )}
      <div className="rk2-inner">
        <div className="rk2-mast">
          <div className="rk2-eye"><span className="st"></span>Dept. of Forestry · Field Station<span className="st"></span></div>
          <div className="rk2-ttl">Propagation Unit · No. 01</div>
        </div>
        <div className="rk2-div rk2-hold" onPointerDown={holdStart} onPointerUp={holdEnd} onPointerLeave={holdEnd} onPointerCancel={holdEnd} onContextMenu={e=>e.preventDefault()}><span className="ln"></span><span className="dia"></span><span className="ln"></span></div>
        <div className="rk2-sub">
          <span className="date">Day · <b>{day || '06 / 21 / 26'}</b></span>
          <span className={'rk2-status'+(statusText.length>24?' long':'')} style={{color:connection==='offline'?'var(--oxblood-l)':statusCol}}><span className="d"></span>{statusText}</span>
          <span className="time">Time · <b>{clock}</b></span>
        </div>

        {/* audit 3.1: the last rejected command, over the status row */}
        {toast && (
          <div className="rk2-toast" onClick={onDismissToast} role="status">
            <span className="d"></span>{toast.text}
          </div>
        )}

        <div className={connection==='offline'?'rk-stale':''} style={{flex:1,display:'flex',flexDirection:'column'}}>
        {layout==='roster'
          ? <Ledger shelves={shelves} onOpen={setOpen}/>
          : <div className="rk2-plates">{shelves.map(s=> <Plate key={s.n} s={s} onOpen={setOpen}/> )}</div>}
        </div>

        <div className="rk2-resv">
          <div className="lab"><span className="ln"></span>Reservoir · {PG.TANK_L} L Tank<span className="ln"></span></div>
          <div className="rk2-measure"><i style={{width:(reservoirOnline?reservoir:0)+'%'}}></i><div className="tk"></div></div>
          {/* audit 3.9: a stale level probe reads "— %" / "sensor offline"; liters and days are estimates */}
          {reservoirOnline
            ? <div className="pct"><b>{reservoir}%</b> · est. {PG.tankLiters(reservoir)} L on hand · est. {PG.tankDays(reservoir)} days</div>
            : <div className="pct"><b>— %</b> · level sensor offline</div>}
        </div>
      </div>

      {current && <Detail2 s={current} watering={watering} busy={busy} pending={pending} theme={theme} onBack={()=>setOpen(null)} onWater={doWater} onDrain={doDrain} onStop={doStop} onLight={onLight} onFan={onFan}
        maintenance={maintenance} onSetMaintenance={onSetMaintenance} onSetEnabled={onSetEnabled} connection={connection} lastSync={lastSync}/>}
      {/* the toast also shows over the detail screen */}
      {current && toast && (
        <div className="rk2-toast over" onClick={onDismissToast} role="status"><span className="d"></span>{toast.text}</div>
      )}
      {power !== 'idle' && (
        <div className="rk2-power" onClick={()=>{ if (power==='ask') setPower('idle'); }}>
          <div onClick={e=>e.stopPropagation()}
            style={{background:'#1B1812',border:'1.5px solid var(--ochre-l)',borderRadius:10,padding:'22px 26px',maxWidth:440,textAlign:'center',color:'var(--lcd)'}}>
            {power === 'ask' ? (
              <React.Fragment>
                <div style={{fontFamily:'"Zilla Slab",serif',fontWeight:900,fontSize:18,marginBottom:8,color:'var(--ochre-l)'}}>Power down Unit 01?</div>
                <div style={{fontFamily:'"Newsreader",serif',fontSize:15,lineHeight:1.5,marginBottom:18}}>Stops any watering, switches every relay off, and shuts the controller down cleanly. Switch the power off once the screen goes dark.</div>
                <div style={{display:'flex',gap:10,justifyContent:'center'}}>
                  <button className="rk2-plaque" style={{justifyContent:'center'}} onClick={()=>setPower('idle')}>Cancel</button>
                  <button className="rk2-wbtn rk2-dbtn" style={{padding:'10px 20px',color:'var(--ochre-l)',borderColor:'var(--ochre-l)'}} onClick={doShutdown}>Shut Down</button>
                </div>
              </React.Fragment>
            ) : (
              <React.Fragment>
                <div style={{fontFamily:'"Zilla Slab",serif',fontWeight:900,fontSize:18,marginBottom:8,color:'var(--ochre-l)'}}>Shutting down</div>
                <div style={{fontFamily:'"Newsreader",serif',fontSize:15,lineHeight:1.5}}>Relays off. The screen will go dark in a few seconds.</div>
              </React.Fragment>
            )}
          </div>
        </div>
      )}
      {locked && <LockScreen clock={clock} onUnlock={onUnlock}/>}
    </div>
  );
}

export { RackDisplay2, DialGauge };
