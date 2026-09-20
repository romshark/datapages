// The offline module replaces __CONFIG__ below with the JSON config at serve time.
'use strict';

const CFG = __CONFIG__;
const CACHE = 'datapages-' + CFG.workerVersion;
const OFFLINE_VERSION_HEADER = 'X-Datapages-Offline-Version';
const SHIM_HEADER = 'X-Datapages-Shim';
const HYDRATE_HEADER = 'X-Datapages-Shim-Hydrate';

// Live responses keyed by pathname. The shim requests its URL after loading.
const pendingLive = new Map();

const CROSS_ORIGIN_DESTINATIONS = CFG.crossOriginDestinations || [];

const EXCLUDED = CFG.excludePaths || [];

function isExcluded(path) {
  for (const p of EXCLUDED) if (path.indexOf(p) === 0) return true;
  return false;
}

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

// Use a built-in response when the configured offline page is unavailable.
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

self.addEventListener('activate', function (e) {
  e.waitUntil((async function () {
    const keys = await caches.keys();
    await Promise.all(keys
      .filter(function (k) { return k.indexOf('datapages-') === 0 && k !== CACHE; })
      .map(function (k) { return caches.delete(k); }));
    await self.clients.claim();
  })());
});

self.addEventListener('message', function (e) {
  const d = e.data || {};
  if (d.type !== 'datapages-offline:apply') return;
  // A sender that navigates once the writes are in place sends a port to reply on.
  // postMessage alone only queues the message.
  const port = e.ports && e.ports[0];
  e.waitUntil((async function () {
    try {
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
    } finally {
      if (port) port.postMessage(true);
    }
  })());
});

// Cached pages bypass the middleware. Add its connectivity script when serving
// an entry so a class change does not require rewriting the cache.
async function servePage(res) {
  if (!CFG.netStateJS) return res;
  const html = await res.text();
  const tag = '<script>' + CFG.netStateJS + '</script>';
  const i = html.indexOf('</head>');
  return new Response(
    i === -1 ? html + tag : html.slice(0, i) + tag + html.slice(i),
    { status: res.status, headers: { 'Content-Type': 'text/html; charset=utf-8' } }
  );
}

// Workers have no DOMParser, so extract the element from the response string.
function element(html, tag) {
  const start = html.indexOf('<' + tag);
  const end = html.lastIndexOf('</' + tag + '>');
  if (start === -1 || end === -1) return null;
  return html.slice(start, end + tag.length + 3);
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

// Headers for a fetch the worker makes on the page's behalf.
// The Datastar markers are dropped: the answer has to be a whole document, not a patch.
async function pageFetchHeaders(req, key) {
  const h = new Headers(req.headers);
  h.delete('Datastar-Request');
  h.delete(HYDRATE_HEADER);
  h.set('Accept', 'text/html');
  h.set('X-Datapages-Worker-Version', String(CFG.workerVersion));
  const cache = await caches.open(CACHE);
  const cached = await cache.match(key);
  const held = cached && cached.headers.get(OFFLINE_VERSION_HEADER);
  if (held) h.set(OFFLINE_VERSION_HEADER, held);
  return h;
}

// True for page navigations. Only mode and destination are trusted.
// Datastar fetches also send Accept: text/html; matching on that would misclassify them.
function isNavigation(req) {
  return req.mode === 'navigate' || req.destination === 'document';
}

// True for a request Datastar issued, which it marks with this header on every fetch.
// Such a request is dynamic: an action, a page hydrate or the page's event stream.
// The server reads the same header.
function isDatastarRequest(req) {
  return req.headers.get('Datastar-Request') !== null;
}

// Reading an event stream into the cache would buffer it until the stream ends.
function isEventStream(res) {
  const ct = res.headers.get('Content-Type');
  return ct !== null && ct.indexOf('text/event-stream') !== -1;
}

self.addEventListener('fetch', function (e) {
  const req = e.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  const sameOrigin = url.origin === self.location.origin;
  // Include the query because /list and /list?page=2 can have different entries.
  const key = url.pathname + url.search;

  if (sameOrigin && isNavigation(req)) {
    e.respondWith((async function () {
      const cache = await caches.open(CACHE);

      const cached = await cache.match(key);
      const headers = new Headers(req.headers);
      headers.set('X-Datapages-Worker-Version', String(CFG.workerVersion));
      if (cached) {
        const held = cached.headers.get(OFFLINE_VERSION_HEADER);
        if (held) headers.set(OFFLINE_VERSION_HEADER, held);
      }

      if (cached && cached.headers.get(SHIM_HEADER)) {
        const live = fetch(key, { headers: headers });
        live.catch(function () {});
        pendingLive.set(key, live);
        // Bound entries when a page never sends its hydration request.
        setTimeout(function () { pendingLive.delete(key); }, 30000);
        return await servePage(cached);
      }

      try {
        return await fetch(new Request(req, { headers: headers }));
      } catch (_) {
        if (cached) return await servePage(cached);
        return await offlineResponse();
      }
    })().catch(function () {
      return fetch(req);
    }));
    return;
  }

  if (sameOrigin && req.headers.get(HYDRATE_HEADER)) {
    e.respondWith((async function () {
      let live = pendingLive.get(key);
      if (live) {
        pendingLive.delete(key);
      } else {
        // A worker restart or timeout can remove the saved response. Fetch a
        // replacement because Datastar cannot apply a complete document here.
        live = fetch(key, { headers: await pageFetchHeaders(req, key) });
      }
      try {
        const res = await live;
        const html = await res.text();
        // A complete document has no matching target. Return explicit head and
        // body patches.
        const body = element(html, 'body');
        if (!body) {
          return new Response(html, {
            status: res.status,
            headers: { 'Content-Type': 'text/html; charset=utf-8' },
          });
        }
        // The head contains the title and, for sessions, the CSRF script. Apply
        // it before a binding in the new body can send an action.
        const head = element(html, 'head');
        let frames = '';
        if (head) {
          frames += sseFrame('datastar-patch-elements', [
            ['selector', 'head'], ['mode', 'outer'], ['elements', head],
          ]);
        }
        frames += sseFrame('datastar-patch-elements', [
          ['selector', 'body'], ['mode', 'outer'], ['elements', body],
        ]);
        return new Response(frames, {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream; charset=utf-8' },
        });
      } catch (_) {
        return Response.error();
      }
    })());
    return;
  }

  // Datastar responses are generated per request and always use the network.
  if (isDatastarRequest(req)) return;

  if (sameOrigin && isExcluded(url.pathname)) return;

  // Cache same-origin assets and configured cross-origin destinations.
  // Cross-origin responses can be opaque and remain valid cache entries.
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
      if (hit) {
        // Refresh same-origin assets after responding because their URLs have
        // no content hash. Opaque cross-origin responses cannot be compared.
        if (sameOrigin) e.waitUntil(revalidate(cache, req));
        return hit;
      }
      try {
        const res = await fetch(req);
        if (res && (res.ok || res.type === 'opaque') && !isEventStream(res)) {
          cache.put(req, res.clone()).catch(function () {});
        }
        return res;
      } catch (_) {
        return hit || Response.error();
      }
    })());
  }
});

// Refresh a cached asset without delaying the current response.
async function revalidate(cache, req) {
  try {
    const res = await fetch(req, { cache: 'no-cache' });
    if (res && res.ok && !isEventStream(res)) await cache.put(req, res);
  } catch (_) {}
}
