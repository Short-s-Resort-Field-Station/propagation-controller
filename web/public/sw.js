// Minimal service worker: makes the dashboard installable. It deliberately
// caches nothing — a control panel must never show stale state or stale code.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));
self.addEventListener('fetch', () => {});
