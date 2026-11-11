// Hook tests — audit 3.1 (rejections become a toast), 3.5 (timeouts are
// attached, in-flight guard), and the per-shelf zone-key routing (3.8).
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useUnitState, TOAST_MS } from './useUnitState.js';
import { BASE } from './api.js';

const NOW = Date.now();
const iso = (agoMs) => new Date(NOW - agoMs).toISOString();

const GETS = {
  '/health': { status: 'ok' },
  '/irrigation/zones': {
    zones: [{
      zone_key: 'zone_3', name: 'Shelf 3', enabled: true, soil_sensor_key: 'soil_zone_3_moisture',
      config: { species: 'Possumhaw Holly' }, latest: { moisture: 41, read_at: iso(2e3) },
    }],
    engine: { active_cycle: null, queue: [] },
  },
  '/sensors': { sensors: [{ type: 'reservoir_sensor', measurements: [{ name: 'level', value: 61, read_at: iso(2e3) }] }] },
  '/lighting/status': { relay_state: 'off', schedule: { on_time: '06:00', off_time: '19:00' } },
  '/relays': { relays: [] },
  '/events?limit=10': { events: [] },
  '/safety/status': { emergency_stop: { active: false }, lockouts: [] },
  '/settings': { settings: { automation: { enabled: true, watering_window: { start: '07:45', end: '11:00' } } } },
  '/fans/status': { state: 'manual_on', relay_state: 'on', override: { mode: 'on', until: null } },
  '/species': { species: [] },
  '/irrigation/zones/zone_3/moisture-control': { trigger_below: 45, recovered_above: 60, run_duration_seconds: 300 },
};

const json = (status, body) => Promise.resolve({
  ok: status >= 200 && status < 300, status, statusText: 'x',
  json: () => Promise.resolve(body),
});

let calls;
function installFetch(onPost) {
  calls = [];
  global.fetch = vi.fn((url, opts = {}) => {
    const path = url.replace(BASE, '');
    calls.push({ path, opts });
    if (opts.method && opts.method !== 'GET') return onPost(path, opts);
    const body = GETS[path];
    return body ? json(200, body) : json(404, { error: 'not found ' + path });
  });
}

beforeEach(() => { vi.useRealTimers(); });
afterEach(() => { vi.restoreAllMocks(); delete global.fetch; });

describe('useUnitState', () => {
  it('loads a snapshot and attaches a timeout signal to every GET', async () => {
    installFetch(() => json(200, {}));
    const { result } = renderHook(() => useUnitState('manual_touch'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    const [state] = result.current;
    expect(state.shelves).toHaveLength(1);
    expect(state.shelves[0].n).toBe('03');
    expect(state.reservoir).toBe(61);
    expect(state.reservoirOnline).toBe(true);
    expect(state.lastSync).toMatch(/^\d\d:\d\d$/);
    expect(state.connection).toBe('online');
    expect(state.toast).toBeNull();
    const gets = calls.filter((c) => !c.opts.method || c.opts.method === 'GET');
    expect(gets.length).toBeGreaterThan(0);
    for (const c of gets) expect(c.opts.signal).toBeInstanceOf(AbortSignal);
  });

  it('turns a rejected command into a toast with the backend message, and resolves false', async () => {
    installFetch((path) => {
      if (path.endsWith('/run')) return json(423, { error: 'pump is locked out: pump_max_runtime' });
      return json(200, {});
    });
    const { result } = renderHook(() => useUnitState('manual_touch'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));

    let ok;
    await act(async () => { ok = await result.current[1].water(result.current[0].shelves[0]); });
    expect(ok).toBe(false);
    expect(result.current[0].toast).toBeTruthy();
    expect(result.current[0].toast.text).toContain('pump is locked out');
    expect(typeof result.current[0].toast.at).toBe('number');
    // the POST went to the shelf's own zone key, attributed to the surface
    const post = calls.find((c) => c.path === '/irrigation/zones/zone_3/run');
    expect(post).toBeTruthy();
    expect(JSON.parse(post.opts.body).actor).toBe('manual_touch');
    expect(post.opts.signal).toBeInstanceOf(AbortSignal);
  });

  it('clears the toast after TOAST_MS and on dismiss', async () => {
    installFetch((path) => (path.endsWith('/drain') ? json(409, { error: 'zone already running' }) : json(200, {})));
    const { result } = renderHook(() => useUnitState('manual_web'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    await act(async () => { await result.current[1].drain(result.current[0].shelves[0]); });
    expect(result.current[0].toast.text).toContain('already running');
    act(() => { result.current[1].dismissToast(); });
    expect(result.current[0].toast).toBeNull();

    await act(async () => { await result.current[1].drain(result.current[0].shelves[0]); });
    expect(result.current[0].toast).toBeTruthy();
    await waitFor(() => expect(result.current[0].toast).toBeNull(), { timeout: TOAST_MS + 2000 });
  }, 10000);

  it('a successful command resolves true and refreshes without a toast', async () => {
    installFetch(() => json(200, { accepted: true }));
    const { result } = renderHook(() => useUnitState('manual_touch'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    const before = calls.length;
    let ok;
    await act(async () => { ok = await result.current[1].stop(result.current[0].shelves[0]); });
    expect(ok).toBe(true);
    expect(result.current[0].toast).toBeNull();
    expect(calls.find((c, i) => i >= before && c.path === '/irrigation/zones/zone_3/stop')).toBeTruthy();
    expect(calls.filter((c, i) => i >= before && c.path === '/health').length).toBeGreaterThan(0);
  });

  it('dashboard lighting override carries expires_minutes; auto clears it', async () => {
    installFetch(() => json(200, {}));
    const { result } = renderHook(() => useUnitState('manual_web'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    await act(async () => { await result.current[1].lights(true, { expiresMinutes: 120 }); });
    const on = calls.find((c) => c.path === '/lighting/on');
    expect(JSON.parse(on.opts.body)).toEqual({ actor: 'manual_web', expires_minutes: 120 });
    await act(async () => { await result.current[1].lights(false); });
    expect(JSON.parse(calls.find((c) => c.path === '/lighting/off').opts.body)).toEqual({ actor: 'manual_web' });
    await act(async () => { await result.current[1].lights('auto'); });
    expect(calls.find((c) => c.path === '/lighting/auto')).toBeTruthy();
  });

  it('routes enable/disable by the live zone key, not the shelf number', async () => {
    installFetch(() => json(200, {}));
    const { result } = renderHook(() => useUnitState('manual_touch'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    await act(async () => { await result.current[1].setShelfEnabled('03', false); });
    const patch = calls.find((c) => c.opts.method === 'PATCH');
    expect(patch.path).toBe('/irrigation/zones/zone_3');
    expect(JSON.parse(patch.opts.body)).toEqual({ enabled: false });
  });

  it('goes offline when the snapshot fails, keeping the last readings', async () => {
    installFetch(() => json(200, {}));
    const { result } = renderHook(() => useUnitState('manual_touch'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    const sync = result.current[0].lastSync;
    global.fetch = vi.fn(() => Promise.reject(new TypeError('Failed to fetch')));
    await act(async () => { await result.current[1].refresh(); });
    expect(result.current[0].connection).toBe('offline');
    expect(result.current[0].shelves).toHaveLength(1);
    expect(result.current[0].lastSync).toBe(sync);
  });

  // 2026-11-10: fans ride their own scheduler; a manual toggle is a timed
  // override via /fans/{on,off}, 'auto' clears it; automation + window are
  // read from settings.
  it('reads fan override + automation, and toggles fans through /fans', async () => {
    installFetch((path) => json(200, { state: 'off', relay_state: 'off' }));
    const { result } = renderHook(() => useUnitState('manual_web'));
    await waitFor(() => expect(result.current[0].ready).toBe(true));
    const [state, actions] = result.current;
    expect(state.fanOn).toBe(true);
    expect(state.fanOverride).toEqual({ mode: 'on', until: null });
    expect(state.automation).toEqual({ enabled: true, window: { start: '07:45', end: '11:00' } });
    await act(async () => { await actions.fans(false); });
    const off = calls.find((c) => c.path === '/fans/off');
    expect(off).toBeTruthy();
    expect(JSON.parse(off.opts.body)).toMatchObject({ actor: 'manual_web', expires_minutes: 120 });
    await act(async () => { await actions.fans('auto'); });
    expect(calls.some((c) => c.path === '/fans/auto')).toBe(true);
    await act(async () => { await actions.setAutomation(false); });
    const s = calls.find((c) => c.path === '/settings' && c.opts.method === 'PUT');
    expect(JSON.parse(s.opts.body)).toEqual({ automation: { enabled: false } });
  });
});
