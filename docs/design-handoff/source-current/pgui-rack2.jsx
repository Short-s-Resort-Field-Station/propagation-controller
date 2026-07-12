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
  Maintenance:'var(--ochre-l)', Disabled:'var(--sfaint)' // change 2
};
const RS2 = {
  Operational:'var(--rk2-accent)', Watering:'var(--river-l)', Draining:'var(--river-l)', // live-edit (ebb-flow)
  Fault:'var(--oxblood-l)', Maintenance:'var(--ochre-l)'
};

// flat needle dial — 270° sweep, engraved ticks, target band, no glow
function DialGauge({ value, lo, hi, accent, size=82, active=true }){
  const cx=size/2, cy=size/2, r=size/2-6, start=135, sweep=270;
  const pol=(deg,rr)=>{const a=deg*Math.PI/180;return [cx+rr*Math.cos(a), cy+rr*Math.sin(a)];};
  const ticks=[];
  for(let i=0;i<=10;i++){
    const deg=start+sweep*(i/10), major=i%5===0;
    const [x1,y1]=pol(deg,r), [x2,y2]=pol(deg,r-(major?8:5));
    ticks.push(<line key={i} x1={x1.toFixed(1)} y1={y1.toFixed(1)} x2={x2.toFixed(1)} y2={y2.toFixed(1)} stroke="var(--sline)" strokeWidth={major?1.4:0.8}/>);
  }
  const arc=(d0,d1)=>{const [x0,y0]=pol(d0,r-3),[x1b,y1b]=pol(d1,r-3);const large=(d1-d0)>180?1:0;
    return `M${x0.toFixed(1)} ${y0.toFixed(1)} A${r-3} ${r-3} 0 ${large} 1 ${x1b.toFixed(1)} ${y1b.toFixed(1)}`;};
  const v=Math.max(0,Math.min(100,value));
  const ndeg=start+sweep*(v/100), [nx,ny]=pol(ndeg,r-7);
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{display:'block',margin:'0 auto'}}>
      <circle cx={cx} cy={cy} r={r} fill="none" stroke="var(--sline-soft)" strokeWidth="1"/>
      {active && <path d={arc(start+sweep*(lo/100), start+sweep*(hi/100))} fill="none" stroke={accent} strokeWidth="2.5" opacity="0.45" strokeLinecap="butt"/>}
      {ticks}
      {active && <line x1={cx} y1={cy} x2={nx.toFixed(1)} y2={ny.toFixed(1)} stroke={accent} strokeWidth="2.2" strokeLinecap="round"/>}
      <circle cx={cx} cy={cy} r="3.2" fill={active?accent:'var(--sfaint)'}/>
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
      <div className="dial"><DialGauge value={0} lo={s.lo} hi={s.hi} accent={col} size={82} active={false}/></div>
      <div className="rd noprobe" style={{color:'var(--sfaint)'}}>standby</div>
      <div className="tp">No specimen</div>
      <div className="tag" style={{color:'var(--sfaint)'}}><span className="d"></span>Vacant</div>
    </div>
  );
  return (
    <div className="rk2-plate" onClick={()=>onOpen(s)}>
      <div className="ph">Shelf · {s.n}</div>
      <div className="nm">{s.species}</div>
      <div className="dial"><DialGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={col} size={82} active={s.moisture>0}/></div>
      {s.moisture>0
        ? <div className="rd">{s.moisture} <small>%VWC</small></div>
        : <div className="rd noprobe">no probe</div>}
      <div className="tp">Target · {s.lo}–{s.hi}<br/>Temp · {s.temp}°F{s.ph!=null ? ` · pH ${s.ph}` : ''}</div>
      <div className="tag" style={{color:col}}><span className="d"></span>{s.status}</div>
    </div>
  );
}

function Ledger({ shelves, onOpen }){
  return (
    <div className="rk2-ledger">
      <div className="rk2-lhead">
        <div>Shelf</div><div>Specimen</div><div>Moisture</div><div>Temp</div><div>Status</div>
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
            <div><span className="sp">{s.species}<small>{s.tag}</small></span></div>
            <div>
              <div style={{width:'100%'}}>
                <div className="rk2-mini"><i style={{width:s.moisture+'%',background:inBand?col:'var(--ochre-l)'}}></i>
                  <span className="tg" style={{left:s.lo+'%'}}></span><span className="tg" style={{left:s.hi+'%'}}></span></div>
                <div style={{fontSize:9,letterSpacing:'0.06em',color:'var(--sdim)',marginTop:5}}>{s.moisture>0 ? s.moisture+'%' : 'no probe'} · {s.lo}–{s.hi}</div>
              </div>
            </div>
            <div style={{fontSize:12,color:'var(--sink)'}}>{s.temp}°F</div>
            <div><span className="rk2-tag2" style={{color:col}}><span className="d"></span>{s.status}</span></div>
          </div>
        );
      })}
    </div>
  );
}

function Detail2({ s, onBack, onWater, watering, theme, onLight, onFan, maintenance, onSetMaintenance, onSetEnabled }){
  const col = watering ? 'var(--river-l)' : (SC2[s.status] || 'var(--sfaint)');
  const label = watering ? 'Watering' : s.status;
  const sev = watering ? PG.STATUS['Watering'].sev : (PG.STATUS[s.status] || PG.STATUS.Inactive).sev;
  // change 2: a disabled shelf can't be watered (engine rejects it); a manual
  // water while maintenance is on asks first.
  const disabled = !s.active;
  const [confirmWater, setConfirmWater] = React.useState(false);
  const tapWater = ()=>{ if(maintenance){ setConfirmWater(true); } else { onWater && onWater(); } };
  const tN = (s.temp-60)/30*100;
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
          <button className="rk2-plaque" onClick={()=>{ if(!watering) onBack(); }}>‹ Return</button>
          <span className="rk2-plaque" style={{color:col,borderColor:col}}><span style={{width:6,height:6,background:col,transform:'rotate(45deg)',display:'inline-block'}}></span>{label} · {sev}</span>
        </div>
        <div className="rk2-dname">
          <div className="nm">{s.species}</div>
          <div className="tg">Shelf {s.n}{s.tag ? ' · '+s.tag : ''}</div>
        </div>
        <div className="rk2-dread three">
          <div className="rk2-card">
            <DialGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={col} size={74} active={s.moisture>0}/>
            <div className="meta">
              <div className="k">Moisture</div>
              {s.moisture>0
                ? <div className="v">{s.moisture}<span>%VWC</span></div>
                : <div className="v noprobe">no probe</div>}
              <div className="rng">Target · {s.lo}–{s.hi}%</div>
            </div>
          </div>
          <div className="rk2-card">
            {/* change 3: temp happy-band from the shelf's profile (°F→dial scale, 60–90°F span). */}
            <DialGauge value={tN} lo={s.tempBand?(s.tempBand[0]-60)/30*100:40} hi={s.tempBand?(s.tempBand[1]-60)/30*100:60} accent="var(--ochre-l)" size={74} active={s.temp>0}/>
            <div className="meta">
              <div className="k">Temperature</div>
              {s.temp>0
                ? <div className="v">{s.temp}<span>°F</span></div>
                : <div className="v noprobe">no probe</div>}
              <div className="rng">{s.tempBand?`Target · ${s.tempBand[0]}–${s.tempBand[1]}°F`:'Target · not set'}</div>
            </div>
          </div>
          <div className="rk2-card">
            {/* change 3: pH happy-band from the shelf's profile. */}
            <DialGauge value={s.ph!=null ? (s.ph/14)*100 : 0} lo={s.phBand?(s.phBand[0]/14)*100:(5.5/14)*100} hi={s.phBand?(s.phBand[1]/14)*100:(7.5/14)*100}
              accent="var(--rk2-accent)" size={74} active={s.ph!=null}/>
            <div className="meta">
              <div className="k">Soil pH</div>
              {s.ph!=null
                ? <div className="v">{s.ph}<span>pH</span></div>
                : <div className="v noprobe">no probe</div>}
              <div className="rng">{s.phBand?`Band · ${s.phBand[0]}–${s.phBand[1]}`:'Band · not set'}</div>
            </div>
          </div>
        </div>
        <div className="rk2-controls2">
          <div className="rk2-light">
            <div className="ll"><span className="k">Lighting · Rack</span><span className="ph">{s.on} – {s.off}</span></div>
            <div className="rk2-seg">
              <button className={lightOn?'on':''} onClick={()=>onLight && onLight(true)}>On</button>
              <button className={!lightOn?'on':''} onClick={()=>onLight && onLight(false)}>Off</button>
            </div>
          </div>
          <div className="rk2-light">
            <div className="ll"><span className="k">Fans · Rack</span><span className="ph">Circulation</span></div>
            <div className="rk2-seg">
              <button className={fanOn?'on':''} onClick={()=>onFan && onFan(true)}>On</button>
              <button className={!fanOn?'on':''} onClick={()=>onFan && onFan(false)}>Off</button>
            </div>
          </div>
        </div>
        <div className="rk2-water">
          {watering ? (
            <div className="rk2-pump"><div className="tk"></div><div className="fill"></div><span className="lbl">Pump active · valve open · 45 s cycle</span></div>
          ) : disabled ? (
            <div className="rk2-plaque" style={{justifyContent:'center',width:'100%',color:'var(--sfaint)'}}>Shelf disabled — enable to water</div>
          ) : (
            <button className="rk2-wbtn" onClick={tapWater}><span className="d"></span>Water Now<span className="d"></span></button>
          )}
          {/* change 2: per-shelf disable toggle + system maintenance toggle */}
          <div className="rk2-actions">
            <button className="rk2-plaque" style={{justifyContent:'center'}}
              onClick={()=>onSetEnabled && onSetEnabled(s.n, disabled)}>
              {disabled ? 'Enable Shelf' : 'Disable Shelf'}
            </button>
            <button className="rk2-plaque" style={{justifyContent:'center',...(maintenance?{color:'var(--ochre-l)',borderColor:'var(--ochre-l)'}:{})}}
              onClick={()=>onSetMaintenance && onSetMaintenance(!maintenance)}>
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

function RackDisplay2({ shelves, reservoir, rackStatus, layout, clock, day, onWater, onLight, onFan, theme, locked, connection='online', onUnlock, initialOpen, onThemeToggle, maintenance=false, onSetMaintenance, onSetEnabled }){
  const [open, setOpen] = React.useState(initialOpen ? { n: initialOpen } : null);
  const [watering, setWatering] = React.useState(false);
  const active = shelves.filter(s=>s.active);
  const statusCol = RS2[rackStatus] || RS2.Operational;
  const current = open ? shelves.find(s=>s.n===open.n) : null;
  // Live cycle state wins over the local animation: the lamp keeps pulsing
  // until the backend reports the cycle closed (handoff §8).
  const isWatering = watering || (current && current.status==='Watering');
  const doWater = ()=>{ if(isWatering) return; setWatering(true); onWater && onWater(current); setTimeout(()=>setWatering(false), 3200); };

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
        <div className="rk2-div"><span className="ln"></span><span className="dia"></span><span className="ln"></span></div>
        <div className="rk2-sub">
          <span className="date">Day · <b>{day || '06 / 21 / 26'}</b></span>
          <span className="rk2-status" style={{color:connection==='offline'?'var(--oxblood-l)':statusCol}}><span className="d"></span>{connection==='offline'?'Controller Offline':'System '+rackStatus}</span>
          <span className="time">Time · <b>{clock}</b></span>
        </div>

        {/* change 2: maintenance banner strip (both surfaces) */}
        {maintenance && (
          <div style={{display:'flex',alignItems:'center',justifyContent:'center',gap:8,margin:'0 0 8px',padding:'6px 10px',border:'1px solid var(--ochre-l)',borderRadius:6,background:'rgba(181,138,60,0.12)',color:'var(--ochre-l)',fontFamily:'"JetBrains Mono",monospace',fontSize:10,letterSpacing:'0.14em',textTransform:'uppercase'}}>
            <span style={{width:8,height:8,borderRadius:'50%',background:'var(--ochre-l)'}}></span>Maintenance · automation suspended · lighting on
          </div>
        )}

        <div className={connection==='offline'?'rk-stale':''} style={{flex:1,display:'flex',flexDirection:'column'}}>
        {layout==='roster'
          ? <Ledger shelves={shelves} onOpen={setOpen}/>
          : <div className="rk2-plates">{shelves.map(s=> <Plate key={s.n} s={s} onOpen={setOpen}/> )}</div>}
        </div>

        <div className="rk2-resv">
          <div className="lab"><span className="ln"></span>Reservoir · 26.5 L Tank<span className="ln"></span></div>
          <div className="rk2-measure"><i style={{width:reservoir+'%'}}></i><div className="tk"></div></div>
          <div className="pct"><b>{reservoir}%</b> · ≈ {PG.tankLiters(reservoir)} L on hand · ≈ {PG.tankDays(reservoir)} days</div>
        </div>
      </div>

      {current && <Detail2 s={current} watering={isWatering} theme={theme} onBack={()=>setOpen(null)} onWater={doWater} onLight={onLight} onFan={onFan}
        maintenance={maintenance} onSetMaintenance={onSetMaintenance} onSetEnabled={onSetEnabled}/>}
      {locked && <LockScreen clock={clock} onUnlock={onUnlock}/>}
    </div>
  );
}

export { RackDisplay2, DialGauge };
