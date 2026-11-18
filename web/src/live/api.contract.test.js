// Contract test: every call api.js can make must have a route on the daemon.
//
// The other frontend tests mock fetch and accept any path, which is how
// POST /lighting/auto and /fans/auto went unserved from the first commit until
// 2026-11-17: the Auto button existed, the test "passed", and the daemon
// answered 404. This test calls every method on `api`, records the method and
// path it would send, and checks each against the daemon's real route table
// (internal/api/testdata/routes.txt, pinned by TestRouteTableGolden in Go).
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { api, BASE, EXPORT_PATHS, exportUrl } from './api.js';

const ROUTES = fs.readFileSync(path.resolve(__dirname, '../../../internal/api/testdata/routes.txt'), 'utf8')
  .split('\n').filter(Boolean).map((l) => {
    const [method, route] = l.split(' ');
    const re = new RegExp('^' + route.replace(/\{[^}]+\}/g, '[^/]+') + '$');
    return { method, route, re };
  });

const served = (method, p) => ROUTES.some((r) => r.method === method && r.re.test(p));

// Arguments for methods whose path depends on an enumerated argument: every
// value the UI can send is listed so each one is checked.
const VARIANTS = {
  lightingSet: [['on', 'manual_web', 120], ['off', 'manual_web'], ['auto', 'manual_web']],
  fansSet: [['on', 'manual_web', 120], ['off', 'manual_web'], ['auto', 'manual_web']],
};

describe('api.js ↔ daemon route table', () => {
  let calls;
  beforeEach(() => {
    calls = [];
    vi.stubGlobal('fetch', vi.fn(async (url, opts = {}) => {
      calls.push({ method: (opts.method || 'GET').toUpperCase(), url: String(url) });
      return { ok: true, status: 200, json: async () => ({}) };
    }));
  });
  afterEach(() => vi.unstubAllGlobals());

  it('every api method posts to a path the daemon serves', async () => {
    const missing = [];
    for (const name of Object.keys(api)) {
      const argSets = VARIANTS[name] || [['x1', 'x2', 'x3', 'x4']];
      for (const args of argSets) {
        calls = [];
        await api[name](...args);
        expect(calls.length, `${name} made no request`).toBeGreaterThan(0);
        for (const c of calls) {
          expect(c.url.startsWith(BASE), `${name} left the API prefix: ${c.url}`).toBe(true);
          const p = '/api/v1' + c.url.slice(BASE.length).split('?')[0];
          if (!served(c.method, p)) missing.push(`${name}(${args.join(', ')}) → ${c.method} ${p}`);
        }
      }
    }
    expect(missing, 'api.js calls with no daemon route:\n' + missing.join('\n')).toEqual([]);
  });

  it('every download link points at a path the daemon serves', () => {
    for (const which of Object.keys(EXPORT_PATHS)) {
      expect(exportUrl(which).startsWith(BASE)).toBe(true);
      expect(served('GET', '/api/v1' + EXPORT_PATHS[which]), `no route for the ${which} download`).toBe(true);
    }
  });

  it('the route table is not empty and knows the scheduled modes', () => {
    expect(ROUTES.length).toBeGreaterThan(40);
    for (const svc of ['lighting', 'fans']) for (const m of ['on', 'off', 'auto']) {
      expect(served('POST', `/api/v1/${svc}/${m}`), `${svc}/${m}`).toBe(true);
    }
  });
});
