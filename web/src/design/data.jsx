import React from 'react';

// pgui-data.jsx — shared data, helpers, and small instrument components
// Native Texas restoration species, plausible sensor state, and the
// reusable gauges/lamps/charts used by both surfaces.

const PG = {};

// ---- status vocabulary (calm, institutional; muted colors) ----
PG.STATUS = {
  Healthy:      { col:'var(--i-sage)',    inkCol:'var(--sage-deep)',  sev:'NORMAL' },
  Monitoring:   { col:'var(--i-ochre)',   inkCol:'var(--ochre-deep)', sev:'ATTENTION' },
  Watering:     { col:'var(--i-river)',   inkCol:'var(--river-deep)', sev:'NORMAL' },
  Draining:     { col:'var(--i-river)',   inkCol:'var(--river-deep)', sev:'NORMAL' }, // live-edit (ebb-flow): valve open, pump off, tray draining back
  Queued:       { col:'var(--i-river)',   inkCol:'var(--river-deep)', sev:'WAITING' }, // 2026-11-13: in the one-shelf queue behind another cycle
  Dry:          { col:'var(--i-ochre)',   inkCol:'var(--ochre-deep)', sev:'ACTION REQUIRED' },
  'Sensor Fault':{col:'var(--i-oxblood)', inkCol:'var(--oxblood)',    sev:'ACTION REQUIRED' },
  Inactive:     { col:'var(--lcd-faint)', inkCol:'var(--char)',         sev:'—' },
  Maintenance:  { col:'var(--i-ochre)',   inkCol:'var(--ochre-deep)', sev:'PAUSED' },     // change 2: automation suspended
  Disabled:     { col:'var(--lcd-faint)', inkCol:'var(--char)',         sev:'DISABLED' },   // change 2: assigned but paused
  'Locked Out': { col:'var(--i-oxblood)', inkCol:'var(--oxblood)',    sev:'ACTION REQUIRED' }, // audit 3.11: safety lockout
};

PG.RACK_STATUS = {
  Operational: { col:'var(--i-sage)',   ink:'var(--sage-deep)' },
  Watering:    { col:'var(--i-river)',  ink:'var(--river-deep)' },
  Draining:    { col:'var(--i-river)',  ink:'var(--river-deep)' }, // live-edit (ebb-flow)
  Fault:       { col:'var(--i-oxblood)',ink:'var(--oxblood)' },
  Maintenance: { col:'var(--i-ochre)',  ink:'var(--ochre-deep)' },
  'Locked Out':{ col:'var(--i-oxblood)',ink:'var(--oxblood)' }, // audit 3.11: pump locked out by the safety supervisor
};

// ---- the four shelves ----
PG.initialShelves = () => ([
  { n:'01', active:true,  moisture:68, temp:74, lo:60, hi:78, light:true,  on:'06:00', off:'20:00',
    lastWater:'14:32', usage:'2.4 L', species:'Bald Cypress Cuttings', tag:'TAXODIUM DISTICHUM', status:'Healthy', stripe:'var(--river)', fan:4 },
  { n:'02', active:true,  moisture:52, temp:76, lo:58, hi:74, light:true,  on:'06:00', off:'19:30',
    lastWater:'09:05', usage:'3.1 L', species:'Possumhaw Holly', tag:'ILEX DECIDUA', status:'Monitoring', stripe:'var(--ochre)', fan:6 },
  { n:'03', active:true,  moisture:71, temp:75, lo:62, hi:80, light:true,  on:'06:30', off:'20:00',
    lastWater:'11:48', usage:'1.9 L', species:'Switchgrass Plugs', tag:'PANICUM VIRGATUM', status:'Healthy', stripe:'var(--sage)', fan:3 },
  { n:'04', active:false, moisture:0,  temp:0,  lo:60, hi:78, light:false, on:'—', off:'—',
    lastWater:'—', usage:'—', species:'Unassigned', tag:'EMPTY BAY', status:'Inactive', stripe:'var(--paper-3)', fan:0 },
]);

PG.initialLog = () => ([
  { ts:'06/21/26 14:33', ev:'Target moisture reached', em:'SHELF 02 · 57%', sev:'var(--sage)' },
  { ts:'06/21/26 14:32', ev:'Shelf 02 watered — pump cycle 45s', em:'SHELF 02 · 42% → 57%', sev:'var(--river)' },
  { ts:'06/21/26 12:10', ev:'Lighting period began', em:'SHELF 03 · PHOTOPERIOD 06:30', sev:'var(--ochre)' },
  { ts:'06/21/26 11:48', ev:'Shelf 03 watered — scheduled', em:'SHELF 03 · 64% → 71%', sev:'var(--river)' },
  { ts:'06/21/26 08:02', ev:'Reservoir refilled to 78%', em:'RESERVOIR · +120 L', sev:'var(--sage)' },
  { ts:'06/20/26 17:18', ev:'Sensor recovered', em:'SHELF 03 · MOISTURE PROBE', sev:'var(--sage)' },
]);

// ---- native specimen catalog (assignment flow) ----
PG.SPECIES = [
  { species:'Bald Cypress',      tag:'TAXODIUM DISTICHUM',        hab:'Wetland margin',      lo:62, hi:80, temp:75, onH:6, offH:20, fan:4, stripe:'var(--river)' },
  { species:'Buttonbush',        tag:'CEPHALANTHUS OCCIDENTALIS', hab:'Wetland',             lo:66, hi:85, temp:76, onH:6, offH:19, fan:5, stripe:'var(--river)' },
  { species:'Possumhaw Holly',   tag:'ILEX DECIDUA',              hab:'Understory',          lo:55, hi:72, temp:74, onH:6, offH:19, fan:6, stripe:'var(--ochre)' },
  { species:'Texas Redbud',      tag:'CERCIS CANADENSIS TEXENSIS',hab:'Upland edge',         lo:45, hi:65, temp:73, onH:7, offH:19, fan:5, stripe:'var(--ochre)' },
  { species:'Switchgrass',       tag:'PANICUM VIRGATUM',          hab:'Tallgrass prairie',   lo:45, hi:65, temp:75, onH:6, offH:20, fan:3, stripe:'var(--sage)' },
  { species:'Little Bluestem',   tag:'SCHIZACHYRIUM SCOPARIUM',   hab:'Prairie',             lo:40, hi:60, temp:76, onH:6, offH:20, fan:3, stripe:'var(--sage)' },
  { species:'Inland Sea Oats',   tag:'CHASMANTHIUM LATIFOLIUM',   hab:'Shaded bottomland',   lo:55, hi:72, temp:73, onH:7, offH:18, fan:4, stripe:'var(--sage)' },
  { species:'Lindheimer Muhly',  tag:'MUHLENBERGIA LINDHEIMERI',  hab:'Dry slope',           lo:35, hi:55, temp:77, onH:6, offH:20, fan:2, stripe:'var(--ochre)' },
  { species:'Cedar Elm',         tag:'ULMUS CRASSIFOLIA',         hab:'Upland tree',         lo:48, hi:68, temp:74, onH:6, offH:19, fan:4, stripe:'var(--sage-deep)' },
];

// ---- operators & roles (auth flow) ----
PG.OPERATORS = [
  { name:'Head Steward', role:'Steward', pin:'2468' },
  { name:'M. Reyes',     role:'Grower',  pin:'1357' },
  { name:'J. Alvarez',   role:'Grower',  pin:'0642' },
  { name:'Field Intern', role:'Viewer',  pin:'0000' },
];
PG.ROLES = {
  Steward: { level:3, can:'Full control — assign, configure, water, manage alerts & operators.' },
  Grower:  { level:2, can:'Operate — water, adjust parameters, acknowledge alerts.' },
  Viewer:  { level:1, can:'Read-only — view status, charts & history.' },
};

// ---- reservoir (7-gallon jug = 26.5 L; Austin, 2026-07-06) ----
// EST_DAILY_L is a placeholder consumption estimate until pump flow is
// calibrated on real hardware.
PG.TANK_L = 18.9; // 5-gal bucket reservoir, 13 in full (Austin, 2026-11-10)
PG.EST_DAILY_L = 4;
PG.tankLiters = (pct) => Math.round((pct / 100) * PG.TANK_L * 10) / 10;
PG.tankDays = (pct) => Math.max(0, Math.round(PG.tankLiters(pct) / PG.EST_DAILY_L));

// ---- escalation policy (alerts flow) ----
// No unit buzzer exists (and never will — Austin, 2026-07-06); escalation is
// email/SMS only.
PG.ESCALATION_DEFAULT = { email:true, sms:false, repeatMin:15 };

// derive live alerts from current state
PG.deriveAlerts = (shelves, reservoir, clock) => {
  const out = [];
  shelves.forEach(s => {
    if (!s.active) return;
    if (s.status==='Sensor Fault') out.push({ id:'F'+s.n, ts:clock, shelf:s.n, kind:'Sensor Fault',  sev:'var(--oxblood)', prio:'ACTION REQUIRED', msg:`Shelf ${s.n} moisture probe not reporting.` });
    else if (s.status==='Dry')     out.push({ id:'D'+s.n, ts:clock, shelf:s.n, kind:'Dry',           sev:'var(--ochre)',   prio:'ACTION REQUIRED', msg:`Shelf ${s.n} below its target band.` });
    else if (s.status==='Monitoring') out.push({ id:'M'+s.n, ts:clock, shelf:s.n, kind:'Monitoring', sev:'var(--ochre)',   prio:'ATTENTION',       msg:`Shelf ${s.n} drifting toward the low band.` });
  });
  if (reservoir<=20) out.push({ id:'RES', ts:clock, shelf:'—', kind:'Reservoir Low', sev:'var(--oxblood)', prio:'ACTION REQUIRED', msg:`Reservoir at ${reservoir}% — refill soon.` });
  return out;
};

// deterministic event history (history flow)
PG.history = (idx) => {
  const per = [8,18,30,44][idx] || 18;
  const kinds = [
    ['watered — scheduled cycle','var(--river)','Irrigation'],
    ['reading logged','var(--sage)','Sensor'],
    ['photoperiod began','var(--ochre)','Lighting'],
    ['target moisture reached','var(--sage)','Irrigation'],
    ['reservoir refilled','var(--sage)','Reservoir'],
    ['sensor recovered','var(--sage)','Sensor'],
    ['fan speed adjusted','var(--ochre)','Climate'],
  ];
  const out=[]; let s=idx*7+3;
  for (let i=0;i<per;i++){
    s=(s*9301+49297)%233280; const r=s/233280;
    const k=kinds[Math.floor(r*kinds.length)];
    const sh=['01','02','03'][Math.floor(((s>>3)%3))];
    const day=String(Math.max(1,21-Math.floor(i/3))).padStart(2,'0');
    const hh=String(6+Math.floor(r*13)).padStart(2,'0');
    const mm=String(Math.floor((s>>5)%60)).padStart(2,'0');
    out.push({ ts:`06/${day}/26 ${hh}:${mm}`, ev:`Shelf ${sh} — ${k[0]}`, em:`SHELF ${sh} · ${k[2].toUpperCase()}`, sev:k[1], cat:k[2] });
  }
  return out;
};

// deterministic pseudo-random series for charts
PG.series = (seed, n, base, amp, drift) => {
  let s = seed, out = [];
  for (let i=0;i<n;i++){ s=(s*9301+49297)%233280; const r=s/233280;
    out.push(base + Math.sin(i/n*Math.PI*2 + seed)*amp*0.5 + (r-0.5)*amp*0.5 + drift*(i/n)); }
  return out;
};

// =================== SHARED COMPONENTS ===================

// status lamp (instrument)
function Lamp({ color, label, sub, pulse }){
  return (
    <div className={'lamp'+(pulse?' pulse':'')}>
      <span className="dot" style={{'--accent':color}}></span>
      <span className="lbl">{label}{sub && <small>{sub}</small>}</span>
    </div>
  );
}

// circular arc gauge (SVG) — used in rack tiles
function ArcGauge({ value, lo, hi, accent, size=92, label='MOISTURE' }){
  const r=size/2-9, cx=size/2, cy=size/2, sweep=250, start=145;
  const a0=start*Math.PI/180, a1=(start+sweep)*Math.PI/180;
  const pt=(ang)=>[cx+r*Math.cos(ang), cy+r*Math.sin(ang)];
  const arc=(from,to)=>{ const [x0,y0]=pt(from),[x1,y1]=pt(to);
    const large=(to-from)>Math.PI?1:0; return `M${x0.toFixed(2)} ${y0.toFixed(2)} A${r} ${r} 0 ${large} 1 ${x1.toFixed(2)} ${y1.toFixed(2)}`; };
  const frac=Math.max(0,Math.min(1,value/100));
  const av=a0+(a1-a0)*frac;
  // target band marks
  const aLo=a0+(a1-a0)*(lo/100), aHi=a0+(a1-a0)*(hi/100);
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{display:'block',margin:'0 auto'}}>
      <path d={arc(a0,a1)} fill="none" stroke="#ffffff18" strokeWidth="6" strokeLinecap="round"/>
      <path d={arc(aLo,aHi)} fill="none" stroke="#ffffff30" strokeWidth="6" strokeLinecap="butt"/>
      {value>0 && <path d={arc(a0,av)} fill="none" stroke={accent} strokeWidth="6" strokeLinecap="round"/>}
      <text x={cx} y={cy-1} textAnchor="middle" fontFamily="Zilla Slab" fontWeight="900" fontSize={size*0.30} fill="var(--lcd)">{value>0?value:'—'}</text>
      {value>0 && <text x={cx} y={cy+size*0.16} textAnchor="middle" fontFamily="JetBrains Mono" fontSize="9" letterSpacing="1" fill="var(--lcd-dim)">%VWC</text>}
    </svg>
  );
}

// SVG line/area chart for the dashboard (paper)
function LineChart({ datasets, w=820, h=200, pad=38, yMin=40, yMax=85, fmtY=(v)=>v }){
  const innerW=w-pad*2, innerH=h-pad*1.4;
  const n=datasets[0].data.length;
  const X=(i)=>pad+innerW*(i/(n-1));
  const Y=(v)=>pad*0.5+innerH*(1-(v-yMin)/(yMax-yMin));
  const path=(d)=>d.map((v,i)=>`${i?'L':'M'}${X(i).toFixed(1)} ${Y(v).toFixed(1)}`).join(' ');
  const area=(d)=>path(d)+`L${X(n-1).toFixed(1)} ${(pad*0.5+innerH).toFixed(1)} L${X(0).toFixed(1)} ${(pad*0.5+innerH).toFixed(1)} Z`;
  const gridY=[yMin,(yMin+yMax)/2,yMax];
  return (
    <svg width="100%" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{display:'block',overflow:'visible'}}>
      {gridY.map((g,i)=>(
        <g key={i}>
          <line x1={pad} y1={Y(g)} x2={w-pad} y2={Y(g)} stroke="var(--rule-soft)" strokeWidth="1" strokeDasharray={i===0?'0':'2 4'}/>
          <text x={pad-8} y={Y(g)+3} textAnchor="end" fontFamily="JetBrains Mono" fontSize="9" fill="var(--char)">{fmtY(g)}</text>
        </g>
      ))}
      {datasets.map((ds,k)=>(
        <g key={k}>
          {ds.fill && <path d={area(ds.data)} fill={ds.color} opacity="0.10"/>}
          <path d={path(ds.data)} fill="none" stroke={ds.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round"/>
        </g>
      ))}
    </svg>
  );
}

// watering-event bar strip
function EventBars({ data, w=820, h=120, pad=28, color='var(--river)' }){
  const innerW=w-pad*2, innerH=h-pad*1.2, n=data.length;
  const max=Math.max(...data,1), bw=innerW/n*0.5;
  return (
    <svg width="100%" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{display:'block'}}>
      <line x1={pad} y1={pad*0.4+innerH} x2={w-pad} y2={pad*0.4+innerH} stroke="var(--rule)" strokeWidth="1"/>
      {data.map((v,i)=>{ const x=pad+innerW*(i/(n-1))-bw/2; const bh=innerH*(v/max); return (
        <rect key={i} x={x} y={pad*0.4+innerH-bh} width={bw} height={bh} fill={color} opacity={v?0.85:0.15}/>
      );})}
    </svg>
  );
}

// Department of Forestry & Fire seal (extracted from the propagation
// manual's front page) — replaces the placeholder tree-in-a-square mark.
function ForestMark({ size=44 }){
  return (
    <img src={(import.meta.env && import.meta.env.BASE_URL ? import.meta.env.BASE_URL : '/') + 'seal.png'} width={size} height={size} alt="Dept. of Forestry & Fire"
      style={{display:'block', objectFit:'contain'}}/>
  );
}

export { PG, Lamp, ArcGauge, LineChart, EventBars, ForestMark };
