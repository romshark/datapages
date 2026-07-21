// Datapages offline service worker.
// The CONFIG placeholder below is replaced with the JSON-encoded configuration
// at serve time by the offline module.
'use strict';

const CFG = __CONFIG__;
const CACHE = 'datapages-' + CFG.workerVersion;
const OFFLINE_VERSION_HEADER = 'X-Datapages-Offline-Version';

// Fetch request destinations cached when the request goes to another origin.
const CROSS_ORIGIN_DESTINATIONS = CFG.crossOriginDestinations || [];

// Install: precache the app shell assets, plus the offline fallback page, so
// cached pages render offline and uncached navigations have something to serve.
self.addEventListener('install', function (e) {
  e.waitUntil((async function () {
    const cache = await caches.open(CACHE);
    const urls = (CFG.assets || []).slice();
    if (CFG.offlineURL) urls.push(CFG.offlineURL);
    await Promise.allSettled(urls.map(function (u) {
      return cache.add(new Request(u, { cache: 'reload' })).catch(function () {});
    }));
    await self.skipWaiting();
  })());
});

// The offline fallback page, from cache. Falls back to a minimal built-in
// response when no offline page is configured or precaching it failed.
async function offlineResponse() {
  if (CFG.offlineURL) {
    const cache = await caches.open(CACHE);
    const hit = await cache.match(CFG.offlineURL);
    if (hit) return hit;
  }
  return new Response(
    '<!doctype html><meta charset="utf-8"><title>Offline</title>' +
    '<body><h1>You are offline</h1>',
    { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8' } }
  );
}

// Activate: drop caches from older worker versions and take control.
self.addEventListener('activate', function (e) {
  e.waitUntil((async function () {
    const keys = await caches.keys();
    await Promise.all(keys
      .filter(function (k) { return k.indexOf('datapages-') === 0 && k !== CACHE; })
      .map(function (k) { return caches.delete(k); }));
    await self.clients.claim();
  })());
});

// Apply the deferred offline writes a handler queued (see OfflineCacheWriter).
self.addEventListener('message', function (e) {
  const d = e.data || {};
  if (d.type !== 'datapages-offline:apply') return;
  e.waitUntil((async function () {
    const cache = await caches.open(CACHE);
    if (d.clearAll) {
      const reqs = await cache.keys();
      await Promise.all(reqs.map(function (req) { return cache.delete(req); }));
    }
    for (const url of (d.clears || [])) await cache.delete(url);
    for (const s of (d.sets || [])) {
      const headers = {
        'Content-Type': 'text/html; charset=utf-8',
      };
      headers[OFFLINE_VERSION_HEADER] = String(s.version);
      await cache.put(s.url, new Response(s.html, { status: 200, headers: headers }));
    }
  })());
});

function isNavigation(req) {
  if (req.mode === 'navigate') return true;
  const accept = req.headers.get('accept') || '';
  return req.method === 'GET' && accept.indexOf('text/html') !== -1;
}

self.addEventListener('fetch', function (e) {
  const req = e.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  const sameOrigin = url.origin === self.location.origin;

  // HTML navigations: online serves the live response; offline serves the
  // cached body for this URL, or the offline fallback page.
  if (sameOrigin && isNavigation(req)) {
    e.respondWith((async function () {
      const cache = await caches.open(CACHE);

      // Report the version we hold for this URL, and our worker version.
      const cached = await cache.match(url.pathname);
      const headers = new Headers(req.headers);
      headers.set('X-Datapages-Worker-Version', String(CFG.workerVersion));
      if (cached) {
        const held = cached.headers.get(OFFLINE_VERSION_HEADER);
        if (held) headers.set(OFFLINE_VERSION_HEADER, held);
      }

      try {
        return await fetch(new Request(req, { headers: headers }));
      } catch (_) {
        if (cached) return cached;
        return await offlineResponse();
      }
    })());
    return;
  }

  // Cache-first for assets cached pages reference, so they render fully offline:
  // same-origin static files, plus the cross-origin destinations the application
  // opted in to (see Config.CrossOriginDestinations). Cross-origin responses are
  // often opaque (status 0, res.ok false); cache them anyway, the browser can
  // still replay them for the tag that requested them.
  if (sameOrigin || CROSS_ORIGIN_DESTINATIONS.indexOf(req.destination) !== -1) {
    e.respondWith((async function () {
      const cache = await caches.open(CACHE);
      const hit = await cache.match(req);
      if (hit) return hit;
      try {
        const res = await fetch(req);
        if (res && (res.ok || res.type === 'opaque')) {
          cache.put(req, res.clone()).catch(function () {});
        }
        return res;
      } catch (_) {
        return hit || Response.error();
      }
    })());
  }
});
