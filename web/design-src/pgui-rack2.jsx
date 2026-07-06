// pgui-rack2.jsx — ROUND 2: sculptural instrument-panel treatment.
// Same functionality (tap shelf → detail, Water Now, reservoir, two layouts)
// re-imagined as a flat, bilaterally-symmetric apparatus face: framed,
// engraved labels, needle dials, diamond ornaments. Honors the layout tweak.

const SC2 = {
  Healthy:'var(--rk2-accent)', Monitoring:'var(--ochre-l)', Watering:'var(--river-l)',
  Dry:'var(--ochre-l)', 'Sensor Fault':'var(--oxblood-l)', Inactive:'var(--sfaint)'
};
const RS2 = {
  Operational:'var(--rk2-accent)', Watering:'var(--river-l)',
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
  const col = SC2[s.status];
  if(!s.active) return (
    <div className="rk2-plate vacant">
      <div className="ph">Shelf · {s.n}</div>
      <div className="nm" style={{color:'var(--sfaint)'}}>Vacant Bay</div>
      <div className="dial"><DialGauge value={0} lo={s.lo} hi={s.hi} accent={col} size={96} active={false}/></div>
      <div className="rd" style={{color:'var(--sfaint)'}}>—</div>
      <div className="tp">No specimen</div>
      <div className="tag" style={{color:'var(--sfaint)'}}><span className="d"></span>Vacant</div>
    </div>
  );
  return (
    <div className="rk2-plate" onClick={()=>onOpen(s)}>
      <div className="ph">Shelf · {s.n}</div>
      <div className="nm">{s.species}</div>
      <div className="dial"><DialGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={col} size={96}/></div>
      <div className="rd">{s.moisture} <small>%RH</small></div>
      <div className="tp">Target · {s.lo}–{s.hi}<br/>Temp · {s.temp}°F</div>
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
        const col=SC2[s.status];
        const inBand=s.moisture>=s.lo && s.moisture<=s.hi;
        if(!s.active) return (
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
                <div style={{fontSize:9,letterSpacing:'0.06em',color:'var(--sdim)',marginTop:5}}>{s.moisture}% · {s.lo}–{s.hi}</div>
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

function Detail2({ s, onBack, onWater, watering, theme }){
  const col = watering ? 'var(--river-l)' : SC2[s.status];
  const label = watering ? 'Watering' : s.status;
  const sev = watering ? PG.STATUS['Watering'].sev : PG.STATUS[s.status].sev;
  const tN = (s.temp-60)/30*100;
  const [lightOn, setLightOn] = React.useState(s.light);
  const [fan, setFan] = React.useState(s.fan!=null?s.fan:3);
  const bumpFan = (d)=>setFan(v=>Math.max(0,Math.min(10,v+d)));
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
          <div className="tg">Shelf {s.n} · {s.tag}</div>
        </div>
        <div className="rk2-dread">
          <div className="rk2-card">
            <DialGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={col} size={92}/>
            <div className="meta">
              <div className="k">Moisture</div>
              <div className="v">{s.moisture}<span>%RH</span></div>
              <div className="rng">Target · {s.lo}–{s.hi}%</div>
            </div>
          </div>
          <div className="rk2-card">
            <DialGauge value={tN} lo={40} hi={60} accent="var(--ochre-l)" size={92}/>
            <div className="meta">
              <div className="k">Temperature</div>
              <div className="v">{s.temp}<span>°F</span></div>
              <div className="rng">Setpoint · 72–78°F</div>
            </div>
          </div>
        </div>
        <div className="rk2-controls2">
          <div className="rk2-light">
            <div className="ll"><span className="k">Lighting</span><span className="ph">{s.on} – {s.off}</span></div>
            <div className="rk2-seg">
              <button className={lightOn?'on':''} onClick={()=>setLightOn(true)}>On</button>
              <button className={!lightOn?'on':''} onClick={()=>setLightOn(false)}>Off</button>
            </div>
          </div>
          <div className="rk2-light">
            <div className="ll"><span className="k">Fan</span><span className="ph">Circulation</span></div>
            <div className="rk2-step">
              <button onClick={()=>bumpFan(-1)} aria-label="slower">−</button>
              <div className="val">{fan}<small>/10</small></div>
              <button onClick={()=>bumpFan(1)} aria-label="faster">+</button>
            </div>
          </div>
        </div>
        <div className="rk2-water">
          {watering ? (
            <div className="rk2-pump"><div className="tk"></div><div className="fill"></div><span className="lbl">Pump active · valve open · 45 s cycle</span></div>
          ) : (
            <button className="rk2-wbtn" onClick={onWater}><span className="d"></span>Water Now<span className="d"></span></button>
          )}
          <div className="rk2-actions">
            <button className="rk2-plaque" style={{justifyContent:'center'}}>Disable Shelf</button>
            <button className="rk2-plaque" style={{justifyContent:'center'}}>Maintenance</button>
          </div>
        </div>
      </div>
    </div>
  );
}

function RackDisplay2({ shelves, reservoir, rackStatus, layout, clock, onWater, theme, locked, connection='online', onUnlock }){
  const [open, setOpen] = React.useState(null);
  const [watering, setWatering] = React.useState(false);
  const active = shelves.filter(s=>s.active);
  const statusCol = RS2[rackStatus] || RS2.Operational;
  const current = open ? shelves.find(s=>s.n===open.n) : null;
  const doWater = ()=>{ if(watering) return; setWatering(true); onWater && onWater(current); setTimeout(()=>setWatering(false), 3200); };

  return (
    <div className={"rk2-screen"+(theme==='light'?' light':'')}>
      <div className="rk2-grain"></div>
      <div className="rk2-frame"></div><div className="rk2-frame inner"></div>
      <span className="rk2-corner tl"></span><span className="rk2-corner tr"></span>
      <span className="rk2-corner bl"></span><span className="rk2-corner br"></span>

      <div className="rk2-inner">
        <div className="rk2-mast">
          <div className="rk2-eye"><span className="st"></span>Dept. of Forestry · Field Station<span className="st"></span></div>
          <div className="rk2-ttl">Propagation Unit · No. 01</div>
        </div>
        <div className="rk2-div"><span className="ln"></span><span className="dia"></span><span className="ln"></span></div>
        <div className="rk2-sub">
          <span className="date">Day · <b>06 / 21 / 26</b></span>
          <span className="rk2-status" style={{color:connection==='offline'?'var(--oxblood-l)':statusCol}}><span className="d"></span>{connection==='offline'?'Controller Offline':'System '+rackStatus}</span>
          <span className="time">Time · <b>{clock}</b></span>
        </div>

        <div className={connection==='offline'?'rk-stale':''} style={{flex:1,display:'flex',flexDirection:'column'}}>
        {layout==='roster'
          ? <Ledger shelves={shelves} onOpen={setOpen}/>
          : <div className="rk2-plates">{shelves.map(s=> <Plate key={s.n} s={s} onOpen={setOpen}/> )}</div>}
        </div>

        <div className="rk2-resv">
          <div className="lab"><span className="ln"></span>Reservoir · 120 L Tank<span className="ln"></span></div>
          <div className="rk2-measure"><i style={{width:reservoir+'%'}}></i><div className="tk"></div></div>
          <div className="pct"><b>{reservoir}%</b> on hand · ≈ {Math.round(reservoir/13)} days remaining</div>
        </div>
      </div>

      {current && <Detail2 s={current} watering={watering} theme={theme} onBack={()=>setOpen(null)} onWater={doWater}/>}
      {locked && <LockScreen clock={clock} onUnlock={onUnlock}/>}
    </div>
  );
}

Object.assign(window, { RackDisplay2 });
