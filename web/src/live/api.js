// Thin REST client for the propagationd API (spec §15).

// The app is served under /propagate/ (public reverse proxy and the Go server's
// mirror mount), so every API call carries the prefix too.
export const BASE = '/propagate/api/v1';

// Audit 3.5: a wedged daemon or black-holed Wi-Fi path must not leave fetches
// pending forever — GETs give up after 4 s, mutations after 8 s.
const GET_TIMEOUT_MS = 4000;
const MUTATION_TIMEOUT_MS = 8000;

// Audit 3.10/3.11: the dashboard registers a handler here so any 401 (expired
// cookie, revoked account) clears its session and reopens sign-in. Login
// itself is exempt — a wrong password is a 401 too and the modal shows it.
let unauthorizedHandler = null;
export function onUnauthorized(fn) { unauthorizedHandler = fn; }

function withTimeout(opts) {
  const o = opts ? { ...opts } : {};
  if (!o.signal && typeof AbortSignal !== 'undefined' && AbortSignal.timeout) {
    const isGet = !o.method || o.method === 'GET';
    o.signal = AbortSignal.timeout(isGet ? GET_TIMEOUT_MS : MUTATION_TIMEOUT_MS);
  }
  return o;
}

async function req(path, opts) {
  let res;
  try {
    res = await fetch(BASE + path, withTimeout(opts));
  } catch (e) {
    // AbortError (timeout) and network failures surface as one plain message.
    throw new Error(e && e.name === 'TimeoutError' ? 'controller not responding' : (e && e.message) || 'network error');
  }
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch { /* keep statusText */ }
    if (res.status === 401 && unauthorizedHandler && path !== '/auth/login') {
      try { unauthorizedHandler(msg); } catch { /* never mask the real error */ }
    }
    const err = new Error(msg);
    err.status = res.status;
    throw err;
  }
  return res.json();
}

const post = (path, body) => req(path, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body || {}),
});

export const api = {
  session: () => req('/auth/session'),
  login: (username, password) => post('/auth/login', { username, password }),
  logout: () => post('/auth/logout'),
  setup: (username, password) => post('/auth/setup', { username, password }),
  listUsers: () => req('/users'),
  createUser: (username, password, role) => post('/users', { username, password, role }),
  updateUser: (username, body) => req(`/users/${encodeURIComponent(username)}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  deleteUser: (username) => req(`/users/${encodeURIComponent(username)}`, { method: 'DELETE' }),

  health: () => req('/health'),
  zones: () => req('/irrigation/zones'),
  sensors: () => req('/sensors'),
  lighting: () => req('/lighting/status'),
  relays: () => req('/relays'),
  events: (limit = 8) => req(`/events?limit=${limit}`),
  safety: () => req('/safety/status'),
  species: () => req('/species'),
  // change 3: species profiles (per-species, per-stage saved settings).
  speciesProfiles: (speciesId) => req('/species-profiles' + (speciesId ? `?species=${encodeURIComponent(speciesId)}` : '')),
  createProfile: (body) => post('/species-profiles', body),
  updateProfile: (id, body) => req(`/species-profiles/${id}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  deleteProfile: (id) => req(`/species-profiles/${id}`, { method: 'DELETE' }),
  cycles: (limit = 50) => req(`/irrigation/cycles?limit=${limit}`),
  moistureControl: (zoneKey) => req(`/irrigation/zones/${zoneKey}/moisture-control`),
  readings: (sensorKey, limit = 500) => req(`/sensors/${sensorKey}/readings?limit=${limit}`),
  aggregates: (sensorKey, fromISO) =>
    req(`/sensors/${sensorKey}/readings?interval=hour&from=${encodeURIComponent(fromISO)}`),
  lightingSchedule: () => req('/lighting/schedule'),
  settings: () => req('/settings'),
  putSettings: (body) => req('/settings', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  // change 2 (maintenance): system-wide suspend of irrigation automation.
  setMaintenance: (active) => req('/settings', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ maintenance: { active } }),
  }),

  runZone: (zoneKey, actor) => post(`/irrigation/zones/${zoneKey}/run`, { actor, reason: 'water now' }),
  drainZone: (zoneKey, actor) => post(`/irrigation/zones/${zoneKey}/drain`, { actor, reason: 'drain tray' }),
  stopZone: (zoneKey, actor) => post(`/irrigation/zones/${zoneKey}/stop`, { actor }),
  patchZone: (zoneKey, body) => req(`/irrigation/zones/${zoneKey}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  putMoistureControl: (zoneKey, body) => req(`/irrigation/zones/${zoneKey}/moisture-control`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  // mode: on | off | auto. expiresMinutes > 0 makes an on/off override lapse
  // back to the schedule on its own (audit 3.10); 0/undefined = until cleared.
  lightingSet: (mode, actor, expiresMinutes) => post(`/lighting/${mode}`,
    expiresMinutes > 0 ? { actor, expires_minutes: expiresMinutes } : { actor }),
  putLightingSchedule: (sch) => req('/lighting/schedule', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(sch),
  }),
  setRelay: (channel, state, actor, reason) => post(`/relays/${channel}/set`, { state, actor, reason }),
  emergencyStop: (actor) => post('/safety/emergency-stop', { actor }),
  // Hidden touch-screen power-down: stops cycles, relays off, then the OS halts.
  shutdown: (actor) => post('/system/shutdown', { actor, reason: 'shutdown from the screen' }),
  safetyReset: (actor) => post('/safety/reset', { actor }),
};
