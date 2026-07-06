// pgui-app.jsx — top-level app: live clock, tweak state, both surfaces, Tweaks panel.

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "rackLayout": "gauges",
  "themeMode": "dark",
  "accent": "#7FA088",
  "activeShelves": 3,
  "faultShelf2": false,
  "lowReservoir": false,
  "connection": "online",
  "rackLocked": false
}/*EDITMODE-END*/;

function App(){
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [clock, setClock] = React.useState('14:32');
  const [log, setLog] = React.useState(PG.initialLog());
  const [session, setSession] = React.useState({ name:'A. Short', role:'Steward' });
  const [assignments, setAssignments] = React.useState({});
  const [acked, setAcked] = React.useState({});

  // live-ish clock ticking from 14:32
  React.useEffect(()=>{
    let m = 14*60+32;
    const id = setInterval(()=>{
      m = (m+1)%(24*60);
      setClock(String(Math.floor(m/60)).padStart(2,'0')+':'+String(m%60).padStart(2,'0'));
    }, 4000);
    return ()=>clearInterval(id);
  },[]);

  // derive shelves from tweaks
  const shelves = React.useMemo(()=>{
    const base = PG.initialShelves();
    const act = t.activeShelves;
    base.forEach((s,i)=>{
      const idx=i+1;
      if(idx>act){ s.active=false; s.status='Inactive'; s.species='Unassigned'; s.tag='EMPTY BAY'; }
      else if(!s.active){ s.active=true; }
    });
    if(t.faultShelf2 && base[1].active){ base[1].status='Sensor Fault'; base[1].moisture=0; }
    return base.map(s=> assignments[s.n] ? {...s, ...assignments[s.n], active:true} : s);
  }, [t.activeShelves, t.faultShelf2, assignments]);

  const reservoir = t.lowReservoir ? 14 : 78;
  const rackStatus = t.faultShelf2 ? 'Fault' : 'Operational';

  // push accent into instrument operational color
  React.useEffect(()=>{
    document.documentElement.style.setProperty('--i-sage', t.accent);
    document.documentElement.style.setProperty('--rk2-accent', t.accent);
  },[t.accent]);

  const onWater = (shelf)=>{
    if(!shelf) return;
    const e = { ts:'06/21/26 '+clock, ev:`Shelf ${shelf.n} watered — manual cycle 45s`, em:`SHELF ${shelf.n} · PUMP ON · VALVE OPEN`, sev:'var(--river)' };
    setLog(l=>[e, ...l].slice(0,8));
  };

  const alerts = PG.deriveAlerts(shelves, reservoir, clock).filter(a=>!acked[a.id]);
  const onAssign = (n, fields)=>{
    setAssignments(a=>({...a,[n]:fields}));
    const e = { ts:'06/21/26 '+clock, ev:`Shelf ${n} assigned — ${fields.species}`, em:`SHELF ${n} · ${fields.tag} · ONLINE`, sev:'var(--sage)' };
    setLog(l=>[e, ...l].slice(0,8));
  };
  const onAck = (id)=> setAcked(a=>({...a,[id]:true}));
  const onAckAll = ()=> setAcked(a=>{ const n={...a}; alerts.forEach(x=>{ n[x.id]=true; }); return n; });
  const onUnlock = (op)=>{ if(op) setSession(op); setTweak('rackLocked', false); };

  return (
    <div className="wrap">
      <div className="masthead">
        <div>
          <div className="ey">Department of Forestry · Native Planet Ecological Works</div>
          <h1>Propagation Unit No. 01<br/>Interface Standard</h1>
        </div>
        <div className="right">
          Doc · INST-FOR-0001<br/>
          Rev · A / 06.21.26<br/>
          Surfaces · Rack + Web
        </div>
      </div>
      <div className="folio">
        <span>Field Station Instrumentation</span>
        <span>Semi-Automated Nursery Control</span>
      </div>

      <div className="surface-head">
        <span className="tag">A ·</span>
        <span className="ttl">Unit Display</span>
        <span className="ln"></span>
        <span className="meta">4.5″ Touchscreen · 880 × 528 · Quick Status</span>
      </div>
      <div className="rack-stage">
        <RackDisplay shelves={shelves} reservoir={reservoir} rackStatus={rackStatus}
          layout={t.rackLayout} clock={clock} onWater={onWater}
          locked={t.rackLocked} connection={t.connection} alerts={alerts} onUnlock={onUnlock}/>
        <div className="caption">
          <b>Tap a shelf</b> to open its detail screen · <b>Water Now</b> runs a live cycle ·
          layout set to <b>{t.rackLayout==='roster'?'Roster':'Gauges'}</b> via Tweaks
        </div>
      </div>

      <div className="surface-head">
        <span className="tag">A · II ·</span>
        <span className="ttl">Unit Display — Instrument Study</span>
        <span className="ln"></span>
        <span className="meta">Sculptural Panel · Symmetric · Needle Dials</span>
      </div>
      <div className="rk2-stage">
        <RackDisplay2 shelves={shelves} reservoir={reservoir} rackStatus={rackStatus}
          layout={t.rackLayout} clock={clock} onWater={onWater} theme={t.themeMode}
          locked={t.rackLocked} connection={t.connection} onUnlock={onUnlock}/>
        <div className="caption">
          Round 2 · the unit face as a framed apparatus — <b>tap a shelf</b> to open its plate, <b>Water Now</b> runs a live cycle
        </div>
      </div>

      <div className="surface-head">
        <span className="tag">B ·</span>
        <span className="ttl">Web Dashboard</span>
        <span className="ln"></span>
        <span className="meta">Operational Control Center · 4 Views</span>
      </div>
      <Dashboard shelves={shelves} reservoir={reservoir} rackStatus={rackStatus} log={log}
        session={session} onSignIn={setSession} onSignOut={()=>setSession(null)}
        alerts={alerts} onAck={onAck} onAckAll={onAckAll} connection={t.connection}
        assignments={assignments} onAssign={onAssign}/>

      <TweaksPanel>
        <TweakSection label="Unit Display" />
        <TweakRadio label="Home layout" value={t.rackLayout}
          options={[{value:'gauges',label:'Gauges'},{value:'roster',label:'Roster'}]}
          onChange={v=>setTweak('rackLayout',v)}/>
        <TweakRadio label="Round 2 theme" value={t.themeMode}
          options={[{value:'dark',label:'Dark'},{value:'light',label:'Limestone'}]}
          onChange={v=>setTweak('themeMode',v)}/>
        <TweakColor label="Operational accent" value={t.accent}
          options={['#7FA088','#6E97A0','#9FB08C','#C99A45']}
          onChange={v=>setTweak('accent',v)}/>

        <TweakSection label="System State" />
        <TweakSlider label="Active shelves" value={t.activeShelves} min={1} max={4} unit=" / 4"
          onChange={v=>setTweak('activeShelves',v)}/>
        <TweakToggle label="Shelf 02 sensor fault" value={t.faultShelf2}
          onChange={v=>setTweak('faultShelf2',v)}/>
        <TweakToggle label="Low reservoir alert" value={t.lowReservoir}
          onChange={v=>setTweak('lowReservoir',v)}/>

        <TweakSection label="Connection & Access" />
        <TweakRadio label="Controller link" value={t.connection}
          options={[{value:'online',label:'Online'},{value:'offline',label:'Offline'}]}
          onChange={v=>setTweak('connection',v)}/>
        <TweakToggle label="Lock rack screen" value={t.rackLocked}
          onChange={v=>setTweak('rackLocked',v)}/>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
