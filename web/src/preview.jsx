// Design harness — renders design components with demo data and NO live event
// stream, so headless Chrome can screenshot them (the app's SSE stream hangs
// headless capture). Not shipped; used only for visual iteration.
//   /preview.html            → the Assign/Edit modal (edit mode)
//   /preview.html#assign     → the modal in assign (new) mode
//   /preview.html#dash       → the dashboard, Shelf Management tab
import React from 'react';
import ReactDOM from 'react-dom/client';
import './design/pgui.css';
import './live/overrides.css';
import { PG } from './design/data.jsx';
import { AssignModal } from './design/flows.jsx';
import { Dashboard } from './design/dashboard.jsx';
import { RackDisplay2 } from './design/rack2.jsx';

// A catalog carrying every field the live buildSpeciesCatalog adds (id +
// EC/temp happy-band defaults) so the standalone modal behaves like production.
PG.SPECIES = [
  { id:'TADI', species:'Bald Cypress', tag:'TAXODIUM DISTICHUM', hab:'Wetland margin', lo:60, hi:78, temp:75, onH:6, offH:19, fan:4, ecLo:500, ecHi:1500, tempLo:68, tempHi:78, stripe:'var(--river)' },
  { id:'QUMA', species:'Bur Oak',      tag:'QUERCUS MACROCARPA',  hab:'Upland edge',   lo:45, hi:65, temp:74, onH:6, offH:19, fan:3, ecLo:600, ecHi:1600, tempLo:66, tempHi:80, stripe:'var(--sage)' },
  { id:'ULCR', species:'Cedar Elm',    tag:'ULMUS CRASSIFOLIA',   hab:'Upland tree',   lo:48, hi:68, temp:74, onH:6, offH:19, fan:4, ecLo:700, ecHi:1700, tempLo:66, tempHi:78, stripe:'var(--sage-deep)' },
];

const demoProfiles = {
  list: (id) => Promise.resolve([
    { id:1, species_id:id, stage:'Cuttings', name:'Bald Cypress · Cuttings',
      settings:{ moisture:{trigger_below:58,recovered_above:72,run_duration_seconds:300,drain_seconds:300,minimum_interval_minutes:240,max_cycles_per_day:4}, ec:{min:500,max:1500}, temp:{min:68,max:78} } },
    { id:2, species_id:id, stage:'Seeds', name:'Bald Cypress · Seeds',
      settings:{ ec:{min:600,max:1600}, temp:{min:70,max:80} } },
  ]),
  save: (b) => Promise.resolve({ id:99, ...b }),
  update: (id,b) => Promise.resolve({ id, ...b }),
};

const editInitial = { species:'Bald Cypress', species2:'', lo:60, hi:78, durMin:5, drainMin:5,
  ecLo:500, ecHi:1500, tempLo:68, tempHi:78, onH:6, lightHrs:13, stage:'Cuttings', profileId:1, coolMin:240, maxCycles:4 };

const demoShelves = [
  { n:'01', active:true, assigned:true, moisture:68, temp:74, ec:1150, lo:60, hi:78, ecBand:[500,1500], tempBand:[68,78],
    light:true, on:'06:00', off:'19:00', lastWater:'14:32', lastWaterDate:'07/09/26', usage:'2.4 min',
    species:'Bald Cypress', tag:'TAXODIUM DISTICHUM', status:'Healthy', stripe:'var(--river)', fan:4, raw:{} },
  { n:'02', active:true, assigned:true, moisture:52, temp:76, ec:1320, lo:58, hi:74, ecBand:[600,1600], tempBand:[70,80],
    light:true, on:'06:00', off:'19:30', lastWater:'09:05', lastWaterDate:'07/09/26', usage:'3.1 min',
    species:'Possumhaw Holly', tag:'ILEX DECIDUA', status:'Dry', stripe:'var(--ochre)', fan:6, raw:{} },
  { n:'03', active:false, assigned:true, moisture:0, temp:0, ec:null, lo:62, hi:80,
    light:false, on:'—', off:'—', lastWater:'11:48', usage:'—',
    species:'Switchgrass', tag:'PANICUM VIRGATUM', status:'Disabled', stripe:'var(--sage)', fan:0, raw:{} },
  { n:'04', active:false, assigned:false, moisture:0, temp:0, ec:null, lo:60, hi:78,
    light:false, on:'—', off:'—', lastWater:'—', usage:'—',
    species:'Unassigned', tag:'EMPTY BAY', status:'Inactive', stripe:'var(--paper-3)', fan:0, raw:{} },
];

const hash = (typeof location !== 'undefined' ? location.hash : '').replace('#','');

function App(){
  // Touch face (dark instrument) — 880×528 canvas. #touch = overview,
  // #touch-detail = a shelf's detail view (where disable/maintenance live).
  //   #touch-home-maint  = 3 plates + maintenance + a toast (audit 3.7 / 3.1)
  //   #touch-busy        = detail while Draining (Stop row, audit 3.2–3.4)
  //   #touch-offline     = detail while the controller is offline (audit 3.6)
  //   #touch-locked      = home with a safety lockout in the header (3.11)
  if (hash.startsWith('touch')) {
    const three = demoShelves.slice(0,3).map((s,i)=> i===0 ? {...s, species:'Bald Cypress + Possumhaw Holly', hasRule:false} : s);
    const busyShelves = three.map((s)=> s.n==='01' ? {...s, status:'Draining', cycle:{state:'draining', duration:300}} : s);
    const variants = {
      'touch':            { shelves:three, rackStatus:'Operational' },
      'touch-detail':     { shelves:three, rackStatus:'Operational', maintenance:true, initialOpen:'01' },
      'touch-maint':      { shelves:three, rackStatus:'Maintenance', maintenance:true, initialOpen:'01' },
      'touch-home-maint': { shelves:three, rackStatus:'Maintenance', maintenance:true, toast:{text:'Water: pump is locked out: pump_max_runtime', at:0} },
      'touch-busy':       { shelves:busyShelves, rackStatus:'Draining', initialOpen:'01' },
      'touch-offline':    { shelves:three, rackStatus:'Operational', connection:'offline', initialOpen:'01', lastSync:'14:21' },
      'touch-locked':     { shelves:three, rackStatus:'Locked Out', lockouts:['pump_max_runtime'], reservoirOnline:false },
    };
    const v = variants[hash] || variants.touch;
    return (
      <div style={{width:880, height:528, background:'#14110C'}}>
        <RackDisplay2 shelves={v.shelves} reservoir={78} reservoirOnline={v.reservoirOnline!==false} rackStatus={v.rackStatus}
          lockouts={v.lockouts||[]} toast={v.toast||null} lastSync={v.lastSync||'14:32'}
          layout="gauges" clock="14:32" day="07 / 09 / 26" theme="dark"
          maintenance={!!v.maintenance} initialOpen={v.initialOpen||null}
          onWater={()=>Promise.resolve(true)} onDrain={()=>Promise.resolve(true)} onPurge={()=>Promise.resolve(true)} onStop={()=>Promise.resolve(true)}
          onLight={()=>{}} onFan={()=>{}} onThemeToggle={()=>{}}
          onSetMaintenance={()=>{}} onSetEnabled={()=>{}} onUnlock={()=>{}} connection={v.connection||'online'} locked={false} />
      </div>
    );
  }
  // Any hash except '' (edit modal) or 'assign' (new modal) renders the
  // dashboard; the Dashboard's own initialTab reads the hash so #shelves,
  // #environmental, #controls, #dashboard land on the right tab.
  //   #dash-estop = e-stop banner + toast + lockout status (audit 3.1 / 3.3)
  if (hash && hash !== 'assign') {
    const estop = hash === 'dash-estop';
    return (
      <div className="live-db">
        <Dashboard shelves={demoShelves} reservoir={78} reservoirOnline={!estop} rackStatus={estop?'Fault':'Operational'} log={PG.initialLog()}
          session={{name:'A. Short', role:'Steward'}} alerts={[]} connection="online" lastSync="14:21"
          estop={estop} onEmergencyStop={()=>{}} onSafetyReset={()=>{}}
          toast={estop?{text:'Water: sign in required to control the unit', at:0}:null}
          getSeries={null} controls={null} profiles={demoProfiles}
          onAssign={()=>{}} onSetEnabled={()=>{}} onUnassign={()=>{}} onSetMaintenance={()=>{}} />
      </div>
    );
  }
  const assignMode = hash === 'assign';
  return (
    <div style={{minHeight:'100vh', background:'#ECE3CE'}}>
      <AssignModal shelfN="01" initial={assignMode ? null : editInitial} profiles={demoProfiles}
        onAssign={()=>{}} onClose={()=>{}} onClear={()=>{}} onDisable={()=>{}} />
    </div>
  );
}

document.body.style.margin = '0';
document.body.style.background = '#ECE3CE';
ReactDOM.createRoot(document.getElementById('root')).render(<App />);
