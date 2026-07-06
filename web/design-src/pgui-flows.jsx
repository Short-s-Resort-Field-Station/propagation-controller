// pgui-flows.jsx — the "missing flows" for the Propagation Unit:
// shelf assignment · auth (rack lock + session) · offline states ·
// alerts + escalation · event history + export · config-safety dialogs.
// All paper/instrument styling lives in pgui.css (.fx-* / .asg-* / .al-* / .rk-lock ...).

const fmtH = h => String(h).padStart(2,'0') + ':00';

// ---- small stepper (accepts value or functional updater via set) ----
function Stepper({ v, set, min, max, unit }){
  return (
    <div className="fx-step">
      <button onClick={()=>set(x=>Math.max(min, x-1))} aria-label="decrease">&minus;</button>
      <div className="v">{v}<small>{unit}</small></div>
      <button onClick={()=>set(x=>Math.min(max, x+1))} aria-label="increase">&#43;</button>
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

/* ============ FLOW 1 · SHELF ASSIGNMENT ============ */
function AssignModal({ shelfN, onAssign, onClose }){
  const cat = PG.SPECIES;
  const [i, setI] = React.useState(0);
  const sp = cat[i];
  const [lo,setLo]   = React.useState(sp.lo);
  const [hi,setHi]   = React.useState(sp.hi);
  const [temp,setTemp]=React.useState(sp.temp);
  const [onH,setOnH] = React.useState(sp.onH);
  const [offH,setOffH]=React.useState(sp.offH);
  const [fan,setFan] = React.useState(sp.fan);
  const pick = k => { const s=cat[k]; setI(k); setLo(s.lo); setHi(s.hi); setTemp(s.temp); setOnH(s.onH); setOffH(s.offH); setFan(s.fan); };
  const bandOk = lo < hi;
  const litLeft=(onH/24)*100, litW=Math.max(0,((offH-onH)/24)*100);
  const submit = () => { if(!bandOk) return;
    onAssign(shelfN, { active:true, species:sp.species, tag:sp.tag, moisture:Math.round((lo+hi)/2),
      lo, hi, temp, light:true, on:fmtH(onH), off:fmtH(offH), fan, lastWater:'—', usage:'0.0 L', status:'Healthy', stripe:sp.stripe });
  };
  return (
    <div className="fx-back" onClick={onClose}><div className="fx-modal" style={{maxWidth:'660px'}} onClick={e=>e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">Assign Specimen</span><span className="tag">Shelf {shelfN}</span><span className="x" onClick={onClose}>&times;</span></div>
      <div className="fx-body">
        <div className="fx-field">
          <div className="fx-k">Specimen · native catalog</div>
          <select className="fx-select" value={i} onChange={e=>pick(+e.target.value)}>
            {cat.map((s,k)=><option key={k} value={k}>{s.species}</option>)}
          </select>
          <div className="fx-latin"><em style={{textTransform:'capitalize'}}>{sp.tag.toLowerCase()}</em> &middot; {sp.hab}</div>
        </div>
        <div className="fx-2">
          <div className="fx-field">
            <div className="fx-k">Target moisture band · %RH</div>
            <div className="fx-band"><Stepper v={lo} set={setLo} min={30} max={94} unit="%"/><span className="asg-dash">to</span><Stepper v={hi} set={setHi} min={31} max={95} unit="%"/></div>
            {!bandOk && <div className="fx-warn">Low must be below high</div>}
          </div>
          <div className="fx-field">
            <div className="fx-k">Temperature setpoint · °F</div>
            <Stepper v={temp} set={setTemp} min={60} max={90} unit="°"/>
          </div>
        </div>
        <div className="fx-field">
          <div className="fx-k">Photoperiod</div>
          <div style={{display:'flex',alignItems:'flex-end',gap:'22px',marginBottom:'12px'}}>
            <div><div className="fx-k" style={{marginBottom:'5px'}}>On</div><Stepper v={onH} set={setOnH} min={0} max={12} unit="h"/></div>
            <div><div className="fx-k" style={{marginBottom:'5px'}}>Off</div><Stepper v={offH} set={setOffH} min={13} max={23} unit="h"/></div>
            <div style={{marginLeft:'auto',fontFamily:'Zilla Slab',fontWeight:900,fontSize:'22px'}}>{offH-onH}<span style={{fontSize:'12px',color:'var(--char)'}}> h/day</span></div>
          </div>
          <div className="photo-rail"><div className="photo-track"><div className="lit" style={{left:litLeft+'%',width:litW+'%'}}></div><div className="sun" style={{left:((onH+offH)/2/24)*100+'%'}}></div></div>
            <div className="photo-scale"><span>00:00</span><span>06:00</span><span>12:00</span><span>18:00</span><span>24:00</span></div></div>
        </div>
        <div className="fx-2">
          <div className="fx-field"><div className="fx-k">Fan · circulation</div><Stepper v={fan} set={setFan} min={0} max={10} unit="/10"/></div>
          <div className="fx-field"><div className="fx-k">Department stripe</div><div style={{height:'38px',border:'1.5px solid var(--ink)',borderRadius:'4px',background:sp.stripe}}></div></div>
        </div>
      </div>
      <div className="fx-foot"><span className="note">Brings Shelf {shelfN} online and starts the log.</span><button className="fx-btn" onClick={onClose}>Cancel</button><button className="fx-btn primary" onClick={submit} disabled={!bandOk}>Assign &amp; Activate</button></div>
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
          <EscRow label="Sound the unit buzzer" sub="At the rack" on={esc.buzzer} tog={()=>tog('buzzer')}/>
        </div>
        <div className="fx-2" style={{marginTop:'16px'}}>
          <div><div className="fx-k">Quiet hours (buzzer muted)</div><div className="fx-band"><Stepper v={esc.quietFrom} set={step('quietFrom',18,23)} min={18} max={23} unit="h"/><span className="asg-dash">to</span><Stepper v={esc.quietTo} set={step('quietTo',0,9)} min={0} max={9} unit="h"/></div></div>
          <div><div className="fx-k">Repeat until acknowledged</div><Stepper v={esc.repeatMin} set={step('repeatMin',5,60)} min={5} max={60} unit="m"/></div>
        </div>
      </div>
      <div className="fx-foot"><span className="note">Sensor Fault &amp; Reservoir Low escalate through quiet hours.</span><button className="fx-btn" onClick={onClose}>Close</button><button className="fx-btn primary" onClick={onAckAll} disabled={!alerts.length}>Acknowledge all</button></div>
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

Object.assign(window, { Stepper, PinPad, AssignModal, SignInModal, LockScreen, OfflineBanner, AlertBar, AlertsModal, HistoryModal, Confirm });
