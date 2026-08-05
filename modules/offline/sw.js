// Datapages offline service worker.
// The offline module replaces __CONFIG__ below with the JSON config at serve time.
'use strict';

const CFG = __CONFIG__;
const CACHE = 'datapages-' + CFG.workerVersion;
const OFFLINE_VERSION_HEADER = 'X-Datapages-Offline-Version';
// Marks an entry the worker may serve while online (see SetShim).
const SHIM_HEADER = 'X-Datapages-Shim';

// Prefetched live responses, keyed by pathname. The shim requests its own URL
// right after painting; that request is answered from here.
const pendingLive = new Map();

// Fetch request destinations cached when the request goes to another origin.
const CROSS_ORIGIN_DESTINATIONS = CFG.crossOriginDestinations || [];

// Install: precache the app shell and the offline fallback page.
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

// Offline fallback page from cache. Uses a built-in default if none is set.
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

// Apply the writes a handler queued (see PageCacheWriter).
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
      if (s.shim) headers[SHIM_HEADER] = '1';
      await cache.put(s.url, new Response(s.html, { status: 200, headers: headers }));
    }
  })());
});

// Extract <body>...</body> from a document. Workers have no DOMParser; done on
// the string.
function bodyElement(html) {
  const start = html.indexOf('<body');
  const end = html.lastIndexOf('</body>');
  if (start === -1 || end === -1) return null;
  return html.slice(start, end + '</body>'.length);
}

// Build one Datastar SSE event. Multi-line values repeat the key.
function sseFrame(eventName, kvs) {
  const lines = ['event: ' + eventName];
  for (const kv of kvs) {
    const str = String(kv[1]);
    if (str.indexOf('\n') !== -1) {
      for (const line of str.split('\n')) lines.push('data: ' + kv[0] + ' ' + line);
    } else {
      lines.push('data: ' + kv[0] + ' ' + str);
    }
  }
  return lines.join('\n') + '\n\n';
}

// True for page navigations. Only mode and destination are trusted. Datastar
// fetches also send Accept: text/html; matching on that would misclassify them.
function isNavigation(req) {
  return req.mode === 'navigate' || req.destination === 'document';
}

self.addEventListener('fetch', function (e) {
  const req = e.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  const sameOrigin = url.origin === self.location.origin;

  // HTML navigations. A cached shim is served at once. Otherwise online serves
  // live, offline serves the cached body or the fallback page.
  if (sameOrigin && isNavigation(req)) {
    e.respondWith((async function () {
      const cache = await caches.open(CACHE);

      // Report held version and worker version.
      const cached = await cache.match(url.pathname);
      const headers = new Headers(req.headers);
      headers.set('X-Datapages-Worker-Version', String(CFG.workerVersion));
      if (cached) {
        const held = cached.headers.get(OFFLINE_VERSION_HEADER);
        if (held) headers.set(OFFLINE_VERSION_HEADER, held);
      }

      if (cached && cached.headers.get(SHIM_HEADER)) {
        // Serve the shim now, fetch live in parallel. The shim requests this URL
        // below.
        const live = fetch(url.pathname, { headers: headers });
        live.catch(function () {}); // handled below
        pendingLive.set(url.pathname, live);
        // Drop it if the page never asks.
        setTimeout(function () { pendingLive.delete(url.pathname); }, 30000);
        return cached;
      }

      try {
        return await fetch(new Request(req, { headers: headers }));
      } catch (_) {
        if (cached) return cached;
        return await offlineResponse();
      }
    })().catch(function () {
      // Never break navigation on a worker bug. Fall back to the network.
      return fetch(req);
    }));
    return;
  }

  // Shim requesting its live contents. Answer from the in-flight response.
  if (sameOrigin && pendingLive.has(url.pathname)) {
    e.respondWith((async function () {
      const live = pendingLive.get(url.pathname);
      pendingLive.delete(url.pathname);
      try {
        const res = await live;
        const html = await res.text();
        // Datastar matches by id without a selector; a whole document matches
        // nothing. Target the body.
        const body = bodyElement(html);
        if (!body) {
          return new Response(html, {
            status: res.status,
            headers: { 'Content-Type': 'text/html; charset=utf-8' },
          });
        }
        return new Response(sseFrame('datastar-patch-elements', [
          ['selector', 'body'], ['mode', 'outer'], ['elements', body],
        ]), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream; charset=utf-8' },
        });
      } catch (_) {
        return Response.error();
      }
    })());
    return;
  }

  // Cache-first for assets. Same-origin static files, plus the cross-origin
  // destinations opted in via Config.CrossOriginDestinations. Cross-origin
  // responses are often opaque (status 0, res.ok false); cache them anyway.
  if (sameOrigin || CROSS_ORIGIN_DESTINATIONS.indexOf(req.destination) !== -1) {
    e.respondWith((async function () {
      const cache = await caches.open(CACHE);
      const hit = await cache.match(req);
      // Page entries are HTML bodies, not subresources. Serving one here would
      // answer a page's hydrate request with the shim it is replacing.
      if (hit && (hit.headers.get(SHIM_HEADER) ||
        hit.headers.get(OFFLINE_VERSION_HEADER))) {
        return fetch(req);
      }
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
