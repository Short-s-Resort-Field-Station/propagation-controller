// Web Push on the dashboard (Austin, 2026-11-15): "control notifications for
// watering, shelves being below their watering band". One hook owns the
// browser side — permission, the push subscription, and this device's
// switches on the daemon — and the Alerts modal renders it.
import React from 'react';
import { api } from './api.js';

export const PUSH_CATEGORIES = [
  { k: 'watering',  label: 'Watering',              sub: 'A soak starts, finishes, is skipped or fails' },
  { k: 'dry',       label: 'Shelf below its band',  sub: 'A shelf drops under its watering band, whether or not automation may act' },
  { k: 'reservoir', label: 'Reservoir',             sub: 'Pump lockouts: tank empty or level sensor offline, and the all-clear' },
  { k: 'faults',    label: 'Sensors & faults',      sub: 'A probe or device going offline (off by default)' },
];

function b64ToU8(b64) {
  const pad = '='.repeat((4 - (b64.length % 4)) % 4);
  const raw = atob((b64 + pad).replace(/-/g, '+').replace(/_/g, '/'));
  return Uint8Array.from(Array.from(raw, (c) => c.charCodeAt(0)));
}

export function deviceLabel() {
  const ua = navigator.userAgent || '';
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iPhone' : /Mac/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  const br = /CriOS|Chrome/.test(ua) ? 'Chrome' : /Firefox|FxiOS/.test(ua) ? 'Firefox' : /Safari/.test(ua) ? 'Safari' : '';
  return (os + ' ' + br).trim() || 'Browser';
}

export function usePush(session) {
  const supported = typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
  const signedIn = !!session;
  const [st, setSt] = React.useState({ loading: true, enabled: false, prefs: null, permission: supported ? Notification.permission : 'unsupported', error: null, busy: false, tested: false });
  const subRef = React.useRef(null);
  const patch = (p) => setSt((s) => ({ ...s, ...(typeof p === 'function' ? p(s) : p) }));

  const refresh = React.useCallback(async () => {
    if (!supported || !signedIn) { patch({ loading: false, enabled: false, prefs: null }); return; }
    try {
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.getSubscription();
      subRef.current = sub;
      if (!sub) { patch({ loading: false, enabled: false, prefs: null, permission: Notification.permission }); return; }
      const r = await api.pushLookup(sub.endpoint);
      if (r.subscription) patch({ loading: false, enabled: true, prefs: r.subscription.prefs, permission: Notification.permission, error: null });
      else patch({ loading: false, enabled: false, prefs: null, permission: Notification.permission });
    } catch (e) { patch({ loading: false, error: e.message }); }
  }, [supported, signedIn, session && session.name]); // eslint-disable-line react-hooks/exhaustive-deps
  React.useEffect(() => { refresh(); }, [refresh]);

  const enable = async () => {
    if (!supported || !signedIn) return;
    patch({ busy: true, error: null, tested: false });
    try {
      const perm = await Notification.requestPermission();
      if (perm !== 'granted') {
        patch({ busy: false, permission: perm, error: perm === 'denied' ? 'Notifications are blocked for this site in the browser settings.' : null });
        return;
      }
      const { public_key } = await api.pushVapid();
      const reg = await navigator.serviceWorker.ready;
      let sub = await reg.pushManager.getSubscription();
      if (!sub) sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: b64ToU8(public_key) });
      subRef.current = sub;
      const j = sub.toJSON();
      const r = await api.pushUpsert({ endpoint: j.endpoint, keys: j.keys, label: deviceLabel() });
      patch({ busy: false, enabled: true, prefs: r.subscription.prefs, permission: 'granted' });
    } catch (e) { patch({ busy: false, error: e.message }); }
  };

  const disable = async () => {
    patch({ busy: true, error: null, tested: false });
    try {
      const sub = subRef.current;
      if (sub) {
        try { await api.pushDelete(sub.endpoint); } catch { /* the browser side still unsubscribes */ }
        await sub.unsubscribe();
      }
      subRef.current = null;
      patch({ busy: false, enabled: false, prefs: null });
    } catch (e) { patch({ busy: false, error: e.message }); }
  };

  const setPref = async (k, v) => {
    const sub = subRef.current;
    if (!sub) return;
    let prev = null;
    patch((s) => { prev = s.prefs; return { prefs: { ...s.prefs, [k]: v }, error: null }; });
    try {
      const r = await api.pushUpsert({ endpoint: sub.endpoint, prefs: { [k]: v } });
      patch({ prefs: r.subscription.prefs });
    } catch (e) { patch({ prefs: prev, error: e.message }); }
  };

  const test = async () => {
    const sub = subRef.current;
    if (!sub) return;
    patch({ busy: true, error: null, tested: false });
    try { await api.pushTest(sub.endpoint); patch({ busy: false, tested: true }); }
    catch (e) { patch({ busy: false, error: e.message }); }
  };

  return { supported, signedIn, ...st, enable, disable, setPref, test };
}
