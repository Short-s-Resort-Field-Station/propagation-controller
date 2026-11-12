import React from 'react';

// timechart.jsx — the Environmental tab's timeline chart (Austin, 2026-11-11:
// back to line charts, "more detailed": a real date axis that scales with the
// window, a min–max band around each trace, the period's peak and low
// labelled, and a hover readout with the timestamp and every shelf's value).
// Points are { t: epoch ms, v: value, lo: bucket min, hi: bucket max }.

const H = 3600e3, D = 24 * H;
const pad2 = (n) => String(n).padStart(2, '0');
const MON = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
const DOW = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
const fmtClock = (d) => `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
const fmtMD = (d) => `${d.getMonth() + 1}/${d.getDate()}`;
const fmtMonDay = (d) => `${MON[d.getMonth()]} ${d.getDate()}`;

// Axis ticks that scale with the window, on the viewer's local clock:
// every 3 h for a day (midnight carries the date), each day for a week,
// every 5 days for a month, weekly for a season. `major` ticks draw solid.
export function timeTicks(fromMs, toMs) {
  const span = toMs - fromMs;
  const out = [];
  const d0 = new Date(fromMs);
  if (span <= 36 * H) {
    const firstH = Math.ceil((d0.getHours() + d0.getMinutes() / 60) / 3) * 3;
    const t = new Date(d0.getFullYear(), d0.getMonth(), d0.getDate(), firstH, 0, 0);
    for (; t.getTime() <= toMs; t.setHours(t.getHours() + 3)) {
      const mid = t.getHours() === 0;
      out.push({ t: t.getTime(), label: fmtClock(t), sub: mid ? `${DOW[t.getDay()]} ${fmtMD(t)}` : '', major: mid });
    }
    return out;
  }
  const step = span <= 10 * D ? 1 : span <= 45 * D ? 5 : 7;
  const t = new Date(d0.getFullYear(), d0.getMonth(), d0.getDate() + 1, 0, 0, 0);
  for (; t.getTime() <= toMs; t.setDate(t.getDate() + step)) {
    if (step === 1) out.push({ t: t.getTime(), label: DOW[t.getDay()], sub: fmtMD(t), major: t.getDay() === 1 });
    else out.push({ t: t.getTime(), label: fmtMonDay(t), sub: '', major: t.getDate() <= step });
  }
  return out;
}

// Hover timestamp: weekday, date and clock time.
export function fmtStamp(ms) {
  const d = new Date(ms);
  return `${DOW[d.getDay()]} ${fmtMonDay(d)} · ${fmtClock(d)}`;
}

function useWidth(ref, fallback = 820) {
  const [w, setW] = React.useState(fallback);
  React.useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === 'undefined') return undefined;
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) { const cw = e.contentRect.width; if (cw > 0) setW(cw); }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return w;
}

// Nearest point to time t in a t-sorted array.
export function nearest(points, t) {
  let lo = 0, hi = points.length - 1;
  while (lo < hi) { const mid = (lo + hi) >> 1; if (points[mid].t < t) lo = mid + 1; else hi = mid; }
  if (lo > 0 && Math.abs(points[lo - 1].t - t) < Math.abs(points[lo].t - t)) lo -= 1;
  return points[lo];
}

const extremes = (points) => {
  let mx = points[0], mn = points[0];
  for (const p of points) { if (p.hi > mx.hi) mx = p; if (p.lo < mn.lo) mn = p; }
  return { mx, mn };
};

// Window average of a trace (equal weight per bucket).
export const mean = (points) => points.reduce((a, p) => a + p.v, 0) / points.length;

// `runs` = waterings of this shelf: { t0, t1, mins, source } — each drawn as
// a water drop on the baseline where it started (Austin, 2026-11-11: "a
// symbolic drop of water"), named in the hover readout.
const DROP = 'M0,-9 C3.6,-4.2 5.6,-1.4 5.6,1.6 A5.6,5.6 0 1 1 -5.6,1.6 C-5.6,-1.4 -3.6,-4.2 0,-9 Z';
export function TimeChart({ series = [], runs = [], fromMs, toMs, yMin = 0, yMax = 100, fmtY = (v) => v, unit = '', h = 230, bands = true, averages = true }) {
  const wrap = React.useRef(null);
  const w = useWidth(wrap);
  const [hover, setHover] = React.useState(null);
  const padL = 46, padR = 58, padT = 18, padB = 40;
  const iw = Math.max(10, w - padL - padR), ih = h - padT - padB;
  const span = Math.max(1, toMs - fromMs);
  const X = (t) => padL + iw * ((t - fromMs) / span);
  const Y = (v) => padT + ih * (1 - (v - yMin) / ((yMax - yMin) || 1));
  const ticks = timeTicks(fromMs, toMs);
  const gridY = Array.from({ length: 5 }, (_, i) => yMin + (yMax - yMin) * i / 4);
  const line = (pts) => pts.map((p, i) => `${i ? 'L' : 'M'}${X(p.t).toFixed(1)} ${Y(p.v).toFixed(1)}`).join(' ');
  const band = (pts) => pts.map((p, i) => `${i ? 'L' : 'M'}${X(p.t).toFixed(1)} ${Y(p.hi).toFixed(1)}`).join(' ')
    + ' ' + [...pts].reverse().map((p) => `L${X(p.t).toFixed(1)} ${Y(p.lo).toFixed(1)}`).join(' ') + ' Z';

  const onMove = (e) => {
    const r = wrap.current.getBoundingClientRect();
    const pt = e.touches && e.touches[0] ? e.touches[0] : e;
    const x = pt.clientX - r.left;
    if (x < padL || x > w - padR) { setHover(null); return; }
    setHover(fromMs + span * ((x - padL) / iw));
  };
  const drawn = series.filter((s) => s.points && s.points.length);
  const rows = hover != null ? drawn.map((s) => ({ ...s, p: nearest(s.points, hover) })) : [];
  const hx = rows.length ? X(rows[0].p.t) : null;
  // A run counts as hovered inside its strip, or within a few pixels of it.
  const tol = span * (7 / iw);
  const hoverRun = hover != null ? runs.find((r) => hover >= r.t0 - tol && hover <= Math.max(r.t1, r.t0 + tol)) : null;
  const flip = hx != null && hx > w * 0.62;
  const fmt = (v) => fmtY(Math.round(v * 10) / 10);

  return (
    <div ref={wrap} className="tc-wrap" onMouseMove={onMove} onMouseLeave={() => setHover(null)} onTouchStart={onMove} onTouchMove={onMove}>
      <svg width={w} height={h} style={{ display: 'block', overflow: 'visible' }}>
        {gridY.map((g, i) => (
          <g key={i}>
            <line x1={padL} y1={Y(g)} x2={w - padR} y2={Y(g)} stroke="#1B181322" strokeWidth="1" strokeDasharray={i === 0 ? '0' : '2 4'}/>
            <text x={padL - 8} y={Y(g) + 3} textAnchor="end" fontFamily="JetBrains Mono" fontSize="9" fill="#1B181399">{fmtY(Math.round(g * 10) / 10)}</text>
          </g>
        ))}
        {ticks.map((tk) => (
          <g key={tk.t}>
            <line x1={X(tk.t)} y1={padT} x2={X(tk.t)} y2={padT + ih} stroke={tk.major ? '#1B181340' : '#1B181318'} strokeWidth="1" strokeDasharray={tk.major ? '0' : '2 4'}/>
            <text x={X(tk.t)} y={padT + ih + 14} textAnchor="middle" fontFamily="JetBrains Mono" fontSize="9" fill={tk.major ? '#1B1813' : '#1B181399'}>{tk.label}</text>
            {tk.sub && <text x={X(tk.t)} y={padT + ih + 26} textAnchor="middle" fontFamily="JetBrains Mono" fontSize="8.5" fill="#1B181399">{tk.sub}</text>}
          </g>
        ))}
        {runs.filter((r) => r.t0 >= fromMs && r.t0 <= toMs).map((r, k) => (
          <g key={'run' + k} transform={`translate(${X(r.t0).toFixed(1)} ${(padT + ih - 7).toFixed(1)})`}>
            <path d={DROP} fill="var(--river-deep)" stroke="var(--paper)" strokeWidth="1" opacity={hoverRun === r ? 1 : 0.85}/>
          </g>
        ))}
        {/* Ideal band per shelf (Austin, 2026-11-11): the shelf's target
            range for this metric, shaded across the whole window. */}
        {drawn.map((s, k) => s.target ? (
          <g key={'tgt' + k}>
            <rect x={padL} y={Y(s.target[1])} width={iw} height={Math.max(1, Y(s.target[0]) - Y(s.target[1]))} fill={s.color} opacity="0.09"/>
            <line x1={padL} y1={Y(s.target[1])} x2={w - padR} y2={Y(s.target[1])} stroke={s.color} strokeWidth="1" strokeDasharray="2 3" opacity="0.5"/>
            <line x1={padL} y1={Y(s.target[0])} x2={w - padR} y2={Y(s.target[0])} stroke={s.color} strokeWidth="1" strokeDasharray="2 3" opacity="0.5"/>
            <text x={padL + 4} y={Y(s.target[1]) + 9 + k * 10} fontFamily="JetBrains Mono" fontSize="8.5" fill={s.color} opacity="0.9">ideal {fmt(s.target[0])}–{fmt(s.target[1])}</text>
          </g>
        ) : null)}
        {drawn.map((s, k) => {
          const { mx, mn } = extremes(s.points);
          return (
            <g key={k}>
              {bands && s.points.length > 1 && <path d={band(s.points)} fill={s.color} opacity="0.12"/>}
              <path d={line(s.points)} fill="none" stroke={s.color} strokeWidth="1.8" strokeLinejoin="round" strokeLinecap="round"/>
              <circle cx={X(mx.t)} cy={Y(mx.hi)} r="2.5" fill={s.color}/>
              <text x={X(mx.t)} y={Y(mx.hi) - 6} textAnchor="middle" fontFamily="JetBrains Mono" fontSize="9" fill={s.color}>{fmt(mx.hi)}</text>
              <circle cx={X(mn.t)} cy={Y(mn.lo)} r="2.5" fill={s.color}/>
              <text x={X(mn.t)} y={Y(mn.lo) + 13} textAnchor="middle" fontFamily="JetBrains Mono" fontSize="9" fill={s.color}>{fmt(mn.lo)}</text>
            </g>
          );
        })}
        {/* Window average per shelf (Austin, 2026-11-11): a straight line
            through the chart, labelled at the right edge. */}
        {averages && drawn.map((s, k) => {
          const m = mean(s.points);
          return (
            <g key={'avg' + k}>
              <line x1={padL} y1={Y(m)} x2={w - padR} y2={Y(m)} stroke={s.color} strokeWidth="1" strokeDasharray="6 4" opacity="0.8"/>
              <text x={w - padR + 3} y={Y(m) + 3 + k * 10} textAnchor="start" fontFamily="JetBrains Mono" fontSize="8.5" fill={s.color}>avg {fmt(m)}</text>
            </g>
          );
        })}
        {rows.length > 0 && (
          <g>
            <line x1={hx} y1={padT} x2={hx} y2={padT + ih} stroke="#1B1813" strokeWidth="1" strokeDasharray="3 3"/>
            {rows.map((r, k) => <circle key={k} cx={X(r.p.t)} cy={Y(r.p.v)} r="3.5" fill="var(--paper)" stroke={r.color} strokeWidth="2"/>)}
          </g>
        )}
        {!drawn.length && (
          <text x={padL + iw / 2} y={padT + ih / 2} textAnchor="middle" fontFamily="Newsreader" fontStyle="italic" fontSize="13" fill="var(--char)">Awaiting readings for this window</text>
        )}
      </svg>
      {rows.length > 0 && (
        <div className="tc-tip" style={{ left: flip ? undefined : hx + 12, right: flip ? (w - hx + 12) : undefined, top: padT }}>
          <div className="tc-tip-t">{fmtStamp(rows[0].p.t)}</div>
          {hoverRun && (
            <div className="tc-tip-run">Watered {hoverRun.mins} min · {fmtStamp(hoverRun.t0).split(' · ')[1]}{hoverRun.source ? ` · ${hoverRun.source}` : ''}</div>
          )}
          {rows.map((r, k) => (
            <div key={k} className="tc-tip-r">
              <i style={{ background: r.color }}/>
              <span>{r.label}</span>
              <b>{fmt(r.p.v)}<small>{unit}</small></b>
              {r.p.lo !== r.p.hi && <em>{fmt(r.p.lo)} – {fmt(r.p.hi)}</em>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
