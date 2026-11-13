// Surface A · Round 2 — the rack instrument face, served at "/ui/touch".
// Fixed 880×528 logical canvas scaled to fill the physical panel (the
// Waveshare 5" is 800×480 — same 5:3 ratio, so it scales cleanly).
import React from 'react';
import ReactDOM from 'react-dom/client';
import './design/pgui.css';
import './live/overrides.css';
import { RackDisplay2 } from './design/rack2.jsx';
import { RackDisplay3 } from './design/rack3.jsx';
import './design/rack3.css';
import { useUnitState } from './live/useUnitState.js';
import { fmtDay } from './live/adapter.js';

const CANVAS_W = 880, CANVAS_H = 528;

const fitScale = () => Math.min(
  window.innerWidth / CANVAS_W,
  window.innerHeight / CANVAS_H,
);

function useScale() {
  // Initial scale computed synchronously so the very first paint fits the
  // panel (kiosk boot shows no flash of unscaled UI).
  const [scale, setScale] = React.useState(fitScale);
  React.useEffect(() => {
    const fit = () => setScale(fitScale());
    window.addEventListener('resize', fit);
    return () => window.removeEventListener('resize', fit);
  }, []);
  return scale;
}

function App() {
  const [state, actions] = useUnitState('manual_touch');
  const scale = useScale();
  const [theme, setTheme] = React.useState(localStorage.getItem('rackTheme') || 'dark');
  const toggleTheme = () => {
    const next = theme === 'light' ? 'dark' : 'light';
    setTheme(next);
    localStorage.setItem('rackTheme', next);
  };
  const layout = localStorage.getItem('rackLayout') || 'gauges';
  // Deep-links for kiosk shortcuts and testing:
  //   /ui/touch#shelf-1       — open a shelf detail
  //   /ui/touch#theme-light   — force a theme (also persists)
  //   /ui/touch#face-plates   — the round-2 three-plate face (also persists)
  //   /ui/touch#face-list     — the round-3 list face (default since 2026-11-13)
  const hash = typeof location !== 'undefined' ? location.hash : '';
  const faceMatch = hash.match(/face-(plates|list)/);
  const face = faceMatch ? faceMatch[1] : (localStorage.getItem('rackFace') || 'list');
  React.useEffect(() => { if (faceMatch) localStorage.setItem('rackFace', faceMatch[1]); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const shelfMatch = hash.match(/shelf-(\d+)/);
  const initialOpen = shelfMatch ? String(shelfMatch[1]).padStart(2, '0') : null;
  const themeMatch = hash.match(/theme-(light|dark)/);
  React.useEffect(() => {
    if (themeMatch && themeMatch[1] !== theme) {
      setTheme(themeMatch[1]);
      localStorage.setItem('rackTheme', themeMatch[1]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (!state.ready) {
    return (
      <div style={{ fontFamily: 'JetBrains Mono, monospace', color: '#A79F8B',
        background: '#14110C', height: '100vh', display: 'flex',
        alignItems: 'center', justifyContent: 'center', letterSpacing: '0.15em' }}>
        CONNECTING…
      </div>
    );
  }

  return (
    <div style={{
      width: '100vw', height: '100vh', overflow: 'hidden',
      background: '#14110C', display: 'flex',
      alignItems: 'center', justifyContent: 'center',
    }}>
      <div style={{
        width: CANVAS_W, height: CANVAS_H, flex: 'none',
        transform: `scale(${scale})`, transformOrigin: 'center center',
      }}>
        {face === 'plates' ? <RackDisplay2
          shelves={state.shelves}
          reservoir={state.reservoir}
          reservoirOnline={state.reservoirOnline}
          rackStatus={state.rackStatus}
          lockouts={state.lockouts}
          lastSync={state.lastSync}
          toast={state.toast}
          onDismissToast={actions.dismissToast}
          layout={layout}
          clock={state.clock}
          day={fmtDay()}
          onWater={actions.water}
          onDrain={actions.drain}
          onPurge={actions.purge}
          onStop={actions.stop}
          onShutdown={actions.shutdown}
          onLight={actions.lights}
          onFan={actions.fans}
          theme={theme}
          onThemeToggle={toggleTheme}
          initialOpen={initialOpen}
          locked={false}
          connection={state.connection}
          onUnlock={() => {}}
          maintenance={state.maintenance}
          onSetMaintenance={actions.setMaintenance}
          onSetEnabled={actions.setShelfEnabled}
        /> : <RackDisplay3
          shelves={state.shelves}
          reservoir={state.reservoir}
          reservoirOnline={state.reservoirOnline}
          rackStatus={state.rackStatus}
          lockouts={state.lockouts}
          lastSync={state.lastSync}
          toast={state.toast}
          onDismissToast={actions.dismissToast}
          clock={state.clock}
          day={fmtDay()}
          onWater={actions.water}
          onDrain={actions.drain}
          onPurge={actions.purge}
          onStop={actions.stop}
          onShutdown={actions.shutdown}
          onLight={actions.lights}
          onFan={actions.fans}
          theme={theme}
          onThemeToggle={toggleTheme}
          initialOpen={initialOpen}
          connection={state.connection}
          maintenance={state.maintenance}
          onSetMaintenance={actions.setMaintenance}
          onSetEnabled={actions.setShelfEnabled}
          pumpOn={state.pumpOn}
          fanOn={state.fanOn}
        />}
      </div>
    </div>
  );
}

document.body.style.margin = '0';
ReactDOM.createRoot(document.getElementById('root')).render(<App />);
