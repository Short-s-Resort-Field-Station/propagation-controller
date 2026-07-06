// Thin REST client for the propagationd API (spec §15).

const BASE = '/api/v1';

async function req(path, opts) {
  const res = await fetch(BASE + path, opts);
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch { /* keep statusText */ }
    throw new Error(msg);
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
  cycles: (limit = 50) => req(`/irrigation/cycles?limit=${limit}`),
  moistureControl: (zoneKey) => req(`/irrigation/zones/${zoneKey}/moisture-control`),
  readings: (sensorKey, limit = 500) => req(`/sensors/${sensorKey}/readings?limit=${limit}`),
  aggregates: (sensorKey, fromISO) =>
    req(`/sensors/${sensorKey}/readings?interval=hour&from=${encodeURIComponent(fromISO)}`),
  lightingSchedule: () => req('/lighting/schedule'),

  runZone: (zoneKey, actor) => post(`/irrigation/zones/${zoneKey}/run`, { actor, reason: 'water now' }),
  stopZone: (zoneKey, actor) => post(`/irrigation/zones/${zoneKey}/stop`, { actor }),
  patchZone: (zoneKey, body) => req(`/irrigation/zones/${zoneKey}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  putMoistureControl: (zoneKey, body) => req(`/irrigation/zones/${zoneKey}/moisture-control`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),
  lightingSet: (mode, actor) => post(`/lighting/${mode}`, { actor }),
  putLightingSchedule: (sch) => req('/lighting/schedule', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(sch),
  }),
  setRelay: (channel, state, actor, reason) => post(`/relays/${channel}/set`, { state, actor, reason }),
  emergencyStop: (actor) => post('/safety/emergency-stop', { actor }),
  safetyReset: (actor) => post('/safety/reset', { actor }),
};
