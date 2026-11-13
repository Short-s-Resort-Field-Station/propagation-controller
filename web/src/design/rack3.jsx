// Surface A · Round 3 — "the list" (Austin, 2026-11-13). The home screen is
// three tabbed rows, one per shelf, each with the engraved lollipop dial,
// common + Latin names, the reading line and a status chip; the reservoir is
// a sight glass down the right edge; lights, fans, the pump relay readout and
// the system status sit along the bottom. Tapping a row slides a water-only
// panel in beside the rack instead of leaving the page. Built from the
// design-exploration mockups (Desktop/Propogation Unit 1 Software/
// design-exploration). The round-2 plates face (rack2.jsx) stays selectable
// via #face-plates.
import React from 'react';
import { PG } from './data.jsx';

// ---- status vocabulary → colour + glyph -----------------------------------
const COL = {
  Healthy: 'var(--sage-l)', Monitoring: 'var(--ochre-l)', Watering: 'var(--river-l)',
  Draining: 'var(--river-l)', Queued: 'var(--river-l)', Dry: 'var(--ochre-l)',
  'Sensor Fault': 'var(--oxblood-l)', Inactive: 'var(--sfaint)', Maintenance: 'var(--ochre-l)',
  Disabled: 'var(--sfaint)', 'Locked Out': 'var(--oxblood-l)', Vacant: 'var(--sfaint)',
};
const WATER = { Watering: 'full', Draining: 'full', Queued: 'hollow' };
const HOLLOW = { Dry: true, Monitoring: true, Maintenance: true, Disabled: true, Inactive: true, Vacant: true };

// The engine's cycle phase, in the panel's own words (same map as rack2).
const CYCLE_LABEL = {
  precheck: 'Prechecks', opening_valve: 'Opening valve', starting_pump: 'Starting pump',
  flooding: 'Flooding · pump on', stopping_pump: 'Stopping pump',
  draining: 'Draining · pump off', closing_valve: 'Closing valve',
};
function cycleText(cycle, fallback) {
  if (!cycle) return fallback;
  const txt = CYCLE_LABEL[cycle.state] || cycle.state || fallback;
  return cycle.duration ? `${txt} · ${cycle.duration} s` : txt;
}
const fmtHM = (iso) => { const d = new Date(iso); return isNaN(d) ? '' : String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'); };
const fmtSecs = (sec) => sec >= 120 ? `${Math.round(sec / 60)} min` : `${sec} s`;

function Drop({ hollow }) {
  return (
    <svg className="rk3-drop" viewBox="0 0 10 14" width="9" height="12" aria-hidden="true">
      <path d="M5 0.8 C5 0.8 1 6 1 9.2 A4 4 0 0 0 9 9.2 C9 6 5 0.8 5 0.8 Z"
        fill={hollow ? 'none' : 'currentColor'} stroke={hollow ? 'currentColor' : 'none'} strokeWidth="1.3"/>
    </svg>
  );
}
function Glyph({ status }) {
  const w = WATER[status];
  if (w) return <Drop hollow={w === 'hollow'}/>;
  return <span className={'rk3-dia' + (HOLLOW[status] ? ' o' : '')}></span>;
}
function Chip({ status, text, style }) {
  const col = COL[status] || 'var(--sfaint)';
  return <span className="rk3-chip" style={{ color: col, borderColor: col, ...style }}><Glyph status={status}/>{text || status}</span>;
}

// ---- Dial G (design-exploration, 2026-11-13) --------------------------------
// Open-bottom 270° bezel with a faint inner ring, twenty fine ticks with five
// majors, numerals at 0 / 50 / 100, the unit low on the face, the target
// band as a filled sage sector at the rim, and a lollipop needle in oxblood.
// Out of band the face (bezel, ticks, numerals, unit) turns ochre; needle and
// band do not change. No probe: no needle, faint hollow hub, no band.
function DialG({ value, lo, hi, active = true, hasBand = true, size = 92, unit = 'VWC' }) {
  const cx = size / 2, cy = size / 2, r = size / 2 - 5, start = 135, sweep = 270, sc = size / 84;
  const pol = (deg, rr) => [cx + rr * Math.cos(deg * Math.PI / 180), cy + rr * Math.sin(deg * Math.PI / 180)];
  const arc = (d0, d1, rr) => { const [x0, y0] = pol(d0, rr), [x1, y1] = pol(d1, rr); return `M${x0.toFixed(1)} ${y0.toFixed(1)} A${rr.toFixed(1)} ${rr.toFixed(1)} 0 ${(d1 - d0) > 180 ? 1 : 0} 1 ${x1.toFixed(1)} ${y1.toFixed(1)}`; };
  const v = Math.max(0, Math.min(100, Number(value) || 0));
  const out = active && hasBand && (v < lo || v > hi);
  const FI = out ? 'var(--ochre-l)' : 'var(--sink)';
  const FL = out ? 'var(--ochre-l)' : 'var(--sline)';
  const FS = out ? 'var(--ochre-l)' : 'var(--sline-soft)';
  const FD = out ? 'var(--ochre-l)' : 'var(--sdim)';
  const ticks = [];
  for (let i = 0; i <= 20; i++) {
    const deg = start + sweep * (i / 20), major = i % 5 === 0;
    const [x1, y1] = pol(deg, r - 3 * sc), [x2, y2] = pol(deg, r - (major ? 10 : 6) * sc);
    ticks.push(<line key={i} x1={x1.toFixed(1)} y1={y1.toFixed(1)} x2={x2.toFixed(1)} y2={y2.toFixed(1)} stroke={major ? FI : FL} strokeWidth={((major ? 1.3 : 0.7) * sc).toFixed(2)} opacity={out && !major ? 0.6 : 1}/>);
  }
  const nums = [[0, '0'], [50, '50'], [100, '100']].map(([t, lab]) => { const [x, y] = pol(start + sweep * t / 100, r - 16 * sc); return <text key={t} x={x.toFixed(1)} y={(y + 2.6 * sc).toFixed(1)} textAnchor="middle" fontFamily="Zilla Slab, serif" fontWeight="700" fontSize={(7.5 * sc).toFixed(1)} fill={FD}>{lab}</text>; });
  let band = null;
  if (active && hasBand) {
    const a0 = start + sweep * lo / 100, a1 = start + sweep * hi / 100, ro = r - 0.6 * sc, ri = r - 7.5 * sc;
    const [x0, y0] = pol(a0, ro), [x1, y1] = pol(a1, ro), [xi1, yi1] = pol(a1, ri), [xi0, yi0] = pol(a0, ri);
    const large = (a1 - a0) > 180 ? 1 : 0;
    band = <path d={`M${x0.toFixed(1)} ${y0.toFixed(1)} A${ro.toFixed(1)} ${ro.toFixed(1)} 0 ${large} 1 ${x1.toFixed(1)} ${y1.toFixed(1)} L${xi1.toFixed(1)} ${yi1.toFixed(1)} A${ri.toFixed(1)} ${ri.toFixed(1)} 0 ${large} 0 ${xi0.toFixed(1)} ${yi0.toFixed(1)} Z`} fill="var(--sage-l)" opacity=".55"/>;
  }
  let needle = null;
  if (active) {
    const ndeg = start + sweep * (v / 100);
    const [tx, ty] = pol(ndeg, r - 9 * sc), [bx, by] = pol(ndeg + 180, 9 * sc);
    needle = (
      <g>
        <line x1={bx.toFixed(1)} y1={by.toFixed(1)} x2={tx.toFixed(1)} y2={ty.toFixed(1)} stroke="var(--oxblood-l)" strokeWidth={(1.7 * sc).toFixed(2)} strokeLinecap="round"/>
        <circle cx={bx.toFixed(1)} cy={by.toFixed(1)} r={(2.8 * sc).toFixed(1)} fill="var(--oxblood-l)"/>
        <circle cx={cx} cy={cy} r={(1.6 * sc).toFixed(1)} fill="var(--rk3-face)" stroke="var(--oxblood-l)" strokeWidth={(1.2 * sc).toFixed(2)}/>
      </g>
    );
  }
  return (
    <svg className="rk3-dial" width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-label={active ? `${v} % ${unit}` : 'no probe'}>
      <path d={arc(start, start + sweep, r)} fill="none" stroke={FL} strokeWidth={(1.2 * sc).toFixed(2)}/>
      <path d={arc(start, start + sweep, r - 3 * sc)} fill="none" stroke={FS} strokeWidth={(0.8 * sc).toFixed(2)}/>
      {band}{ticks}{nums}
      {needle || <circle cx={cx} cy={cy} r={(3 * sc).toFixed(1)} fill="none" stroke="var(--sline)" strokeWidth={(1.2 * sc).toFixed(2)}/>}
      <text x={cx} y={(cy + 16 * sc).toFixed(1)} textAnchor="middle" fontFamily="JetBrains Mono, monospace" fontSize={(6 * sc).toFixed(1)} letterSpacing={(1.2 * sc).toFixed(1)} fill={FD}>{unit}</text>
    </svg>
  );
}

// ---- press-and-hold control (replaces the confirm popup) --------------------
const HOLD_MS = 1500;
function HoldBar({ label, onFire, disabled, className = '' }) {
  const [prog, setProg] = React.useState(0);
  const ref = React.useRef({ t0: 0, raf: 0, done: false });
  const stop = () => { const h = ref.current; if (h.raf) cancelAnimationFrame(h.raf); h.raf = 0; h.t0 = 0; setProg(0); };
  const tick = () => {
    const h = ref.current; if (!h.t0) return;
    const p = Math.min(1, (performance.now() - h.t0) / HOLD_MS);
    setProg(p);
    if (p >= 1) { h.t0 = 0; h.raf = 0; setProg(0); onFire && onFire(); return; }
    h.raf = requestAnimationFrame(tick);
  };
  const start = (e) => { if (disabled) return; e.preventDefault(); ref.current.t0 = performance.now(); ref.current.raf = requestAnimationFrame(tick); };
  React.useEffect(() => () => { if (ref.current.raf) cancelAnimationFrame(ref.current.raf); }, []);
  return (
    <div className={'rk3-hold' + (disabled ? ' off' : '') + (className ? ' ' + className : '')}
      onPointerDown={start} onPointerUp={stop} onPointerLeave={stop} onPointerCancel={stop} onContextMenu={(e) => e.preventDefault()}
      role="button" aria-disabled={!!disabled}>
      <i style={{ width: (prog * 100) + '%' }}></i>
      <span>{prog > 0 ? 'Keep holding…' : label}</span>
    </div>
  );
}

// ---- shelf row ----------------------------------------------------------------
function Row({ s, wide, sel, onOpen, maintenance }) {
  const vacant = !s.active && !s.assigned;
  const status = vacant ? 'Vacant' : s.status;
  const busy = s.status === 'Watering' || s.status === 'Draining';
  const chipText = s.queued ? 'Queued' : (s.status === 'Dry' && s.nextCheck && wide ? `Dry · next ${s.nextCheck}` : status);
  let line;
  if (busy) line = cycleText(s.cycle, s.status);
  else if (s.queued) line = `${s.queued.kind} queued${s.queued.behind ? ` · after Shelf ${s.queued.behind}` : ''}`;
  else if (wide && s.probe) line = `${s.temp > 0 ? s.temp + '°F' : '—'} · EC ${s.ec != null ? s.ec : '—'}`;
  else if (wide && !vacant) line = 'no probe reading';
  else line = '';
  return (
    <div className={'rk3-row' + (sel ? ' sel' : '') + (vacant ? ' vacant' : '')} onClick={() => !vacant && onOpen(s)}>
      <div className="rk3-tab">Shelf · {s.n}</div>
      <DialG value={s.moisture} lo={s.lo} hi={s.hi} active={!!s.probe && !vacant} hasBand={s.hasRule !== false} size={wide ? 92 : 86}/>
      <div className="rk3-info">
        <div className="rk3-nm">{vacant ? 'Vacant Bay' : String(s.species || '').replace(' + ', ' · ')}</div>
        {wide && s.latin ? <div className="rk3-lat">{s.latin}</div> : null}
        <div className="rk3-v">
          <span className={'rk3-num' + (s.status === 'Dry' ? ' ochre' : '')}>{s.probe && !vacant ? s.moisture : '—'}<small>{wide ? '%VWC' : '%'}</small></span>
          {line ? <span className={'rk3-line' + (busy || s.queued ? ' riv' : '')}>{line}</span> : null}
          <Chip status={status} text={chipText} style={{ marginLeft: 'auto' }}/>
        </div>
      </div>
    </div>
  );
}

// ---- sight glass --------------------------------------------------------------
function Glass({ pct, online, note }) {
  return (
    <div className={'rk3-glass' + (note ? ' noted' : '')}>
      <span className="k">Tank</span>
      <div className="tube"><i style={{ height: (online ? pct : 0) + '%' }}></i><div className="tk"></div><div className="floor"></div></div>
      <span className="pc">{online ? pct + '%' : '— %'}</span>
      <span className="sub">{online ? `${PG.tankLiters(pct)} L · ${PG.tankDays(pct)} d` : 'offline'}</span>
      {note ? <span className="sub riv">{note}</span> : null}
    </div>
  );
}

// ---- slide-in shelf panel ------------------------------------------------------
function Panel({ s, onClose, onWater, onDrain, onPurge, onStop, onCancel, pending, busy, maintenance, onSetMaintenance, onSetEnabled, connection, lastSync }) {
  const offline = connection === 'offline';
  const disabled = !s.active;
  const purgeSec = s.raw && s.raw.purgeSec ? s.raw.purgeSec : 0;
  const drainMin = s.raw && s.raw.drainMin ? s.raw.drainMin : 0;
  const durMin = s.raw && s.raw.durMin ? s.raw.durMin : 0;
  const band = (b, unit) => b ? `${b[0]}–${b[1]}${unit || ''}` : 'not set';
  return (
    <div className="rk3-panel">
      <div className="rk3-prow"><span className="rk3-pnm">Shelf {s.n}</span><button className="rk3-close" onClick={onClose}>‹ Close</button></div>
      <div className="rk3-pcm">{String(s.species || '').replace(' + ', ' · ')}{s.latin ? <span className="rk3-plat">{s.latin}</span> : null}</div>
      <div className="rk3-vals">
        <div><span className="rk3-num">{s.probe ? s.moisture : '—'}<small>%</small></span><span className="k">{s.hasRule === false ? 'not set' : `${s.lo}–${s.hi}`}</span></div>
        <div><span className="rk3-num">{s.temp > 0 ? s.temp : '—'}<small>°F</small></span><span className="k">{band(s.tempBand)}</span></div>
        <div><span className="rk3-num">{s.ec != null ? s.ec : '—'}</span><span className="k"><span style={{ textTransform: 'none' }}>µS</span> · {band(s.ecBand)}</span></div>
      </div>
      {offline ? (
        <div className="rk3-plaque ox">Controller offline — last reading {lastSync}</div>
      ) : busy ? (
        <React.Fragment>
          <div className="rk3-phase"><i></i><span>{cycleText(s.cycle, s.status)}</span></div>
          <button className="rk3-btn stop" disabled={pending} onClick={onStop} title="Stop this cycle: pump off, valve closed">Stop</button>
        </React.Fragment>
      ) : s.queued ? (
        <React.Fragment>
          <div className="rk3-phase riv"><span>{s.queued.kind} queued{s.queued.behind ? ` · after Shelf ${s.queued.behind}` : ''}</span></div>
          <button className="rk3-btn stop" disabled={pending} onClick={onCancel} title="Remove this shelf from the irrigation queue">Cancel</button>
        </React.Fragment>
      ) : disabled ? (
        <div className="rk3-plaque">Shelf disabled — enable to water</div>
      ) : (
        <React.Fragment>
          <HoldBar label={`Hold to water${durMin ? ` · ${durMin} min` : ''}${maintenance ? ' · maintenance' : ''}`} disabled={pending} onFire={onWater}/>
          <div className="rk3-acts">
            <button className="rk3-btn q" disabled={pending || !purgeSec} onClick={onPurge} title={purgeSec ? 'Open valve briefly, pump off, to clear trickle-down' : 'Purge is off for this shelf'}>Purge{purgeSec ? ` · ${fmtSecs(purgeSec)}` : ' · off'}</button>
            <button className="rk3-btn q" disabled={pending} onClick={onDrain} title="Open valve for the full drain-back time, pump off">Drain{drainMin ? ` · ${drainMin} min` : ''}</button>
          </div>
        </React.Fragment>
      )}
      <div className="rk3-meta">
        last water {s.lastWater}{s.usage && s.usage !== '—' ? ` · ${s.usage}` : ''}
        {s.purgeDue ? <React.Fragment><br/>auto purge {fmtHM(s.purgeDue.at)} · {fmtSecs(s.purgeDue.seconds)}</React.Fragment> : null}
      </div>
      <div className="rk3-acts low">
        <button className="rk3-btn q sm" disabled={offline || busy} onClick={() => onSetEnabled && onSetEnabled(s.n, disabled)}>{disabled ? 'Enable shelf' : 'Disable shelf'}</button>
        <button className={'rk3-btn q sm' + (maintenance ? ' ochre' : '')} disabled={offline} onClick={() => onSetMaintenance && onSetMaintenance(!maintenance)}>{maintenance ? 'Exit maintenance' : 'Maintenance'}</button>
      </div>
    </div>
  );
}

// ---- the face -------------------------------------------------------------------
function RackDisplay3({ shelves, reservoir, reservoirOnline = true, rackStatus, lockouts = [], clock, day, onWater, onDrain, onPurge, onStop, onShutdown, onLight, onFan, theme, connection = 'online', initialOpen, onThemeToggle, maintenance = false, onSetMaintenance, onSetEnabled, toast = null, onDismissToast, lastSync = '—', pumpOn = false, fanOn = false }) {
  const [open, setOpen] = React.useState(initialOpen ? { n: initialOpen } : null);
  const current = open ? shelves.find((s) => s.n === open.n) : null;
  const wide = !current;
  // Easter egg (Austin, 2026-11-10): hold the title 3 s for the power-down popup.
  const [power, setPower] = React.useState('idle');
  const holdRef = React.useRef(null);
  const holdStart = () => { if (holdRef.current) return; holdRef.current = setTimeout(() => { holdRef.current = null; setPower('ask'); }, 3000); };
  const holdEnd = () => { if (holdRef.current) { clearTimeout(holdRef.current); holdRef.current = null; } };
  const doShutdown = async () => { setPower('down'); const ok = onShutdown ? await onShutdown() : false; if (ok === false) setPower('idle'); };
  // One in-flight irrigation command at a time (audit 3.5).
  const [pending, setPending] = React.useState(false);
  const pendingRef = React.useRef(false);
  const fire = (fn, shelf) => {
    if (pendingRef.current || !fn || !shelf) return;
    pendingRef.current = true; setPending(true);
    Promise.resolve(fn(shelf)).catch(() => {}).then(() => { pendingRef.current = false; setPending(false); });
  };
  const busy = !!(current && (current.status === 'Watering' || current.status === 'Draining'));
  const lightOn = shelves.some((s) => s.light);
  const lightOverride = (shelves.find((s) => s.lightOverride) || {}).lightOverride || null;
  const fansOn = fanOn || shelves.some((s) => s.fan);
  const offline = connection === 'offline';
  const cycleRunning = rackStatus === 'Watering' || rackStatus === 'Draining';
  const statusText = offline ? 'Controller offline'
    : rackStatus === 'Locked Out' ? 'Locked out' + (lockouts.length ? ' · ' + lockouts.join(', ').replace(/_/g, ' ') : '')
    : rackStatus + (maintenance && rackStatus !== 'Maintenance' ? ' · maintenance' : '');
  const statusKey = offline ? 'Sensor Fault' : rackStatus === 'Fault' ? 'Sensor Fault' : rackStatus === 'Operational' ? 'Healthy' : rackStatus;
  const pumpKey = pumpOn ? (cycleRunning ? 'Watering' : 'Sensor Fault') : 'Inactive';
  const pumpText = pumpOn ? (cycleRunning ? 'Pump · on' : 'Pump · on · no cycle') : 'Pump · off';
  const glassNote = rackStatus === 'Draining' ? 'refilling' : '';

  return (
    <div className={'rk2-screen rk3' + (theme === 'light' ? ' light' : '')}>
      <div className="rk2-grain"></div>
      <div className="rk2-frame"></div><div className="rk2-frame inner"></div>
      <span className="rk2-corner tl"></span><span className="rk2-corner tr"></span>
      <span className="rk2-corner bl"></span><span className="rk2-corner br"></span>
      <div className="rk3-in">
        <div className="rk3-hd">
          <div className="rk3-side"><span className="k">Day</span><span className="dt">{day}</span></div>
          <div className="rk3-mast">
            <div className="rk3-eye"><span className="rk3-dia"></span>Dept. of Forestry · Field Station<span className="rk3-dia" style={{ marginLeft: 7, marginRight: 0 }}></span></div>
            <div className="rk3-ttl" onPointerDown={holdStart} onPointerUp={holdEnd} onPointerLeave={holdEnd} onPointerCancel={holdEnd} onContextMenu={(e) => e.preventDefault()}>Propagation Unit · No. 01</div>
          </div>
          <div className="rk3-side r">
            {onThemeToggle && (
              <button className="rk3-tbtn" onClick={onThemeToggle} aria-label={theme === 'light' ? 'Switch to dark' : 'Switch to limestone'}>
                {theme === 'light'
                  ? <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M20 13.5A8 8 0 1 1 10.5 4 6.5 6.5 0 0 0 20 13.5Z"/></svg>
                  : <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><circle cx="12" cy="12" r="4.4"/><path d="M12 2.5v2.6M12 18.9v2.6M2.5 12h2.6M18.9 12h2.6M5 5l1.8 1.8M17.2 17.2 19 19M19 5l-1.8 1.8M6.8 17.2 5 19"/></svg>}
              </button>
            )}
            <div className="tm"><span className="k">Time</span><span className="dt">{clock}</span></div>
          </div>
        </div>

        {toast && (
          <div className="rk2-toast" onClick={onDismissToast} role="status"><span className="d"></span>{toast.text}</div>
        )}

        <div className={'rk3-scene' + (offline ? ' rk-stale' : '')}>
          <div className="rk3-left">
            <div className={'rk3-rows' + (wide ? ' wide' : '')}>
              {shelves.map((s) => <Row key={s.n} s={s} wide={wide} sel={!!(current && current.n === s.n)} onOpen={(sh) => setOpen({ n: sh.n })} maintenance={maintenance}/>)}
            </div>
            <div className="rk3-foot">
              {wide && (
                <React.Fragment>
                  <span className="k">Lights{lightOverride ? ' · manual' : ''}</span>
                  <span className="rk3-seg">
                    <button className={lightOn ? 'on' : ''} disabled={offline} onClick={() => onLight && onLight(true)}>On</button>
                    <button className={!lightOn ? 'on' : ''} disabled={offline} onClick={() => onLight && onLight(false)}>Off</button>
                  </span>
                  <span className="k">Fans</span>
                  <span className="rk3-seg">
                    <button className={fansOn ? 'on' : ''} disabled={offline} onClick={() => onFan && onFan(true)}>On</button>
                    <button className={!fansOn ? 'on' : ''} disabled={offline} onClick={() => onFan && onFan(false)}>Off</button>
                  </span>
                </React.Fragment>
              )}
              <Chip status={pumpKey} text={pumpText} style={pumpOn ? {} : { color: 'var(--sdim)', borderColor: 'var(--sline)' }}/>
              <Chip status={statusKey} text={statusText} style={{ marginLeft: 'auto' }}/>
            </div>
          </div>
          {current
            ? <Panel s={current} onClose={() => setOpen(null)} pending={pending} busy={busy}
                onWater={() => fire(onWater, current)} onDrain={() => fire(onDrain, current)} onPurge={() => fire(onPurge, current)}
                onStop={() => fire(onStop, current)} onCancel={() => fire(onStop, current)}
                maintenance={maintenance} onSetMaintenance={onSetMaintenance} onSetEnabled={onSetEnabled} connection={connection} lastSync={lastSync}/>
            : <Glass pct={reservoir} online={reservoirOnline} note={glassNote}/>}
        </div>
      </div>

      {power !== 'idle' && (
        <div className="rk2-power" onClick={() => { if (power === 'ask') setPower('idle'); }}>
          <div onClick={(e) => e.stopPropagation()}
            style={{ background: '#1B1812', border: '1.5px solid var(--ochre-l)', borderRadius: 10, padding: '22px 26px', maxWidth: 440, textAlign: 'center', color: '#F0E8D4' }}>
            {power === 'ask' ? (
              <React.Fragment>
                <div style={{ fontFamily: '"Zilla Slab",serif', fontWeight: 900, fontSize: 18, marginBottom: 8, color: 'var(--ochre-l)' }}>Power down Unit 01?</div>
                <div style={{ fontFamily: '"Newsreader",serif', fontSize: 15, lineHeight: 1.5, marginBottom: 18 }}>Stops any watering, switches every relay off, and shuts the controller down cleanly. Switch the power off once the screen goes dark.</div>
                <div style={{ display: 'flex', gap: 10, justifyContent: 'center' }}>
                  <button className="rk3-btn q sm" onClick={() => setPower('idle')}>Cancel</button>
                  <button className="rk3-btn q sm ochre" onClick={doShutdown}>Shut Down</button>
                </div>
              </React.Fragment>
            ) : (
              <React.Fragment>
                <div style={{ fontFamily: '"Zilla Slab",serif', fontWeight: 900, fontSize: 18, marginBottom: 8, color: 'var(--ochre-l)' }}>Shutting down</div>
                <div style={{ fontFamily: '"Newsreader",serif', fontSize: 15, lineHeight: 1.5 }}>Relays off. The screen will go dark in a few seconds.</div>
              </React.Fragment>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

export { RackDisplay3, DialG, HoldBar };
