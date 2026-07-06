// pgui-rack.jsx — the 4.5" rack touchscreen
// Home screen (two layouts: Gauges / Roster) + shelf detail with Water Now.

function RackHead({ clock }){
  return (
    <div className="rk-head">
      <div className="rk-chev"><i></i><i></i></div>
      <div className="rk-title">
        <div className="t">Propagation Unit No. 01</div>
        <div className="s">Department of Forestry · Field Station</div>
      </div>
      <div className="rk-clock"><b>{clock}</b><br/>06 / 21 / 26</div>
    </div>
  );
}

function ShelfTile({ s, onOpen }){
  const meta = PG.STATUS[s.status];
  if(!s.active) return (
    <div className="shelf-tile inactive">
      <div className="sn">Shelf {s.n}</div>
      <div className="sp">—</div>
      <div style={{margin:'14px 0 6px',fontFamily:'Zilla Slab',fontWeight:900,fontSize:22,color:'var(--lcd-faint)'}}>OFF</div>
      <div className="pill" style={{color:'var(--lcd-faint)'}}>Inactive</div>
    </div>
  );
  return (
    <div className="shelf-tile" onClick={()=>onOpen(s)}>
      <div className="sn">Shelf {s.n}</div>
      <div className="sp">{s.species.split(' ').slice(0,2).join(' ')}</div>
      <div style={{margin:'6px 0 2px'}}>
        <ArcGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={meta.col} size={92}/>
      </div>
      <div className="temp"><span>TEMP</span> {s.temp}°F</div>
      <div className="pill" style={{color:meta.col}}>{s.status}</div>
    </div>
  );
}

function RosterRow({ s, onOpen }){
  const meta = PG.STATUS[s.status];
  if(!s.active) return (
    <div className="roster-row inactive">
      <div className="rn">{s.n}<small>SHELF</small></div>
      <div className="sp">Unassigned<small>Empty Bay</small></div>
      <div style={{gridColumn:'3 / -1',textAlign:'right',fontFamily:'Zilla Slab',fontWeight:900,color:'var(--lcd-faint)'}}>INACTIVE</div>
    </div>
  );
  return (
    <div className="roster-row" onClick={()=>onOpen(s)}>
      <div className="rn">{s.n}<small>SHELF</small></div>
      <div className="sp">{s.species}<small>{s.tag}</small></div>
      <div>
        <div className="lin-track"><div className="lin-fill" style={{width:s.moisture+'%',background:meta.col}}></div></div>
        <div style={{fontSize:9,letterSpacing:'0.1em',color:'var(--lcd-dim)',marginTop:5}}>{s.moisture}% · {s.lo}–{s.hi}</div>
      </div>
      <div className="rd"><small>Temp</small>{s.temp}°F</div>
      <div className="rd" style={{textAlign:'right'}}>
        <small>Status</small>
        <span style={{color:meta.col,fontSize:11,letterSpacing:'0.06em',textTransform:'uppercase'}}>{s.status}</span>
      </div>
    </div>
  );
}

function ShelfDetail({ s, onBack, onWater, watering }){
  const meta = PG.STATUS[s.status];
  const status = watering ? PG.STATUS['Watering'] : meta;
  const statusLabel = watering ? 'Watering' : s.status;
  return (
    <div className="rk-detail">
      <div className="dhead">
        <button className="rk-back" onClick={onBack}>‹ Rack</button>
        <div className="dh-id">Shelf {s.n}<small>{s.tag} · {s.species}</small></div>
        <div style={{marginLeft:'auto'}}>
          <Lamp color={status.col} label={statusLabel.toUpperCase()} sub={status.sev} pulse={watering}/>
        </div>
      </div>
      <div className="dbody">
        <div className="dmetric">
          <div className="readout">
            <div className="k">Current Moisture</div>
            <div className="v">{s.moisture}<span>%RH</span></div>
            <div className="rng">Target range · {s.lo}–{s.hi}%</div>
          </div>
          <div className="readout">
            <div className="k">Current Temperature</div>
            <div className="v">{s.temp}<span>°F</span></div>
            <div className="rng">Setpoint · 72–78°F</div>
          </div>
        </div>
        <div className="dright">
          <div className="dstatusbox" style={{color:status.col}}>
            <div>
              <div className="lbl">{statusLabel}</div>
              <div className="sub">{status.sev}</div>
            </div>
            <ArcGauge value={s.moisture} lo={s.lo} hi={s.hi} accent={status.col} size={78}/>
          </div>
          {watering ? (
            <div className="readout" style={{borderColor:'var(--i-river)'}}>
              <div className="water-anim">
                <span className="drop"></span> Pump activated · valve open · 45s cycle
              </div>
            </div>
          ) : (
            <div className="readout">
              <div className="k">Last Watering</div>
              <div style={{fontFamily:'Zilla Slab',fontWeight:700,fontSize:18,color:'var(--lcd)',marginTop:4}}>{s.lastWater} · {s.usage}</div>
            </div>
          )}
          <div className="dactions">
            <button className="dbtn primary wide" onClick={onWater} disabled={watering}>
              {watering ? 'Watering…' : 'Water Now'}
            </button>
            <button className="dbtn">Disable Shelf</button>
            <button className="dbtn">Maintenance</button>
          </div>
        </div>
      </div>
    </div>
  );
}

function RackDisplay({ shelves, reservoir, rackStatus, layout, clock, onWater, locked, connection='online', alerts=[], onUnlock }){
  const [open, setOpen] = React.useState(null);
  const [watering, setWatering] = React.useState(false);
  const active = shelves.filter(s=>s.active);
  const rs = PG.RACK_STATUS[rackStatus] || PG.RACK_STATUS.Operational;
  const current = open ? shelves.find(s=>s.n===open.n) : null;

  const doWater = () => {
    if(watering) return;
    setWatering(true);
    onWater && onWater(current);
    setTimeout(()=>setWatering(false), 3200);
  };

  return (
    <div className="rk-screen">
      <div className="rk-glow"></div>
      <RackHead clock={clock}/>
      <div className="rk-statusbar">
        <Lamp color={connection==='offline'?'var(--i-oxblood)':rs.col} label={connection==='offline'?'Controller Offline':'System '+rackStatus} sub={connection==='offline'?'Last sync 14:32':`${active.length} of 4 shelves active`} pulse={connection!=='offline'&&rackStatus==='Watering'}/>
        <div style={{fontSize:9.5,letterSpacing:'0.18em',textTransform:'uppercase',color:alerts.length?'var(--i-oxblood)':'var(--lcd-dim)'}}>
          {connection==='offline' ? 'Readings stale' : alerts.length ? `${alerts.length} alert${alerts.length>1?'s':''} · Shelf ${alerts[0].shelf}` : 'Lights On · Pump Idle · No Faults'}
        </div>
      </div>

      <div className={'rk-body'+(connection==='offline'?' rk-stale':'')}>
        {layout==='roster' ? (
          <div className="rk-roster">
            {shelves.map(s=> <RosterRow key={s.n} s={s} onOpen={setOpen}/> )}
          </div>
        ) : (
          <div className="rk-shelves">
            {shelves.map(s=> <ShelfTile key={s.n} s={s} onOpen={setOpen}/> )}
          </div>
        )}
      </div>

      <div className="rk-foot">
        <div className="resv">
          <span className="k">Reservoir</span>
          <div className="resv-bar">
            <i style={{width:reservoir+'%'}}></i>
            <div className="ticks"></div>
          </div>
          <span className="pct">{reservoir}%</span>
        </div>
        <div className="lw">Last Watering<b>{active[0] ? '14:32' : '—'}</b></div>
      </div>

      {current && (
        <ShelfDetail s={current} watering={watering}
          onBack={()=>{ if(!watering) setOpen(null); }}
          onWater={doWater}/>
      )}
      {locked && <LockScreen clock={clock} onUnlock={onUnlock}/>}
    </div>
  );
}

Object.assign(window, { RackDisplay });
