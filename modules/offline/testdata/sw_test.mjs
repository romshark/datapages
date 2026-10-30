// sw_test.mjs runs the offline worker in a VM against a fake origin and checks
// how it stores and serves the responses of requests that are not navigations,
// such as an img element loading a GET action, the page entries a
// navigation opens, and the patches that hydrate a shim.
//
// Usage: node sw_test.mjs path/to/sw.js
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const ORIGIN = 'https://app.test';
const source = fs.readFileSync(process.argv[2], 'utf8');

// WorkerRequest resolves a relative URL against the origin, as a worker does.
class WorkerRequest extends Request {
  constructor(input, init) {
    super(typeof input === 'string' ? new URL(input, ORIGIN) : input, init);
  }
}

// worker starts the script with config and an empty Cache Storage. Every fetch
// it makes is answered by server(), which a test replaces between loads.
function worker(config = {}) {
  const script = source.replaceAll('__CONFIG__',
    JSON.stringify({ workerVersion: 1, ...config }));
  const listeners = {};
  const store = new Map();
  const key = (r) => (typeof r === 'string' ? new URL(r, ORIGIN).href : r.url);
  const w = {
    server: () => { throw new Error('no server'); },
    stored: (path) => store.has(ORIGIN + path),
    // puts holds the status of every response the worker offered cache.put.
    puts: [],
  };
  const cache = {
    async match(r) { const hit = store.get(key(r)); return hit && hit.clone(); },
    async put(r, res) {
      w.puts.push(res.status);
      // As Cache.put does, which never reads the body of a partial response.
      if (res.status === 206) throw new TypeError('Partial response is unsupported');
      store.set(key(r), res);
    },
    async delete(r) { return store.delete(key(r)); },
    async keys() { return [...store.keys()].map((u) => new Request(u)); },
    async add(r) {
      const res = await w.server(typeof r === 'string' ? new Request(new URL(r, ORIGIN)) : r);
      if (!res.ok) throw new TypeError('Request failed');
      store.set(key(r), res);
    },
  };
  const self = {
    location: new URL(ORIGIN + '/service-worker.js'),
    addEventListener(type, fn) { listeners[type] = fn; },
    skipWaiting: async () => {},
    clients: { claim: async () => {} },
  };
  vm.runInContext(script, vm.createContext({
    self,
    caches: { async open() { return cache; }, async keys() { return []; }, async delete() {} },
    fetch: async (r) => w.server(r),
    Request: WorkerRequest, Response, Headers, URL, Map, Promise, setTimeout, console,
  }));
  // install runs the install event, which stores the assets of config.
  w.install = async () => {
    const waits = [];
    listeners.install({ waitUntil(p) { waits.push(p); } });
    await Promise.all(waits);
  };
  // load sends the request an img element sends: no-cors, not a navigation,
  // without the Datastar header. It waits for the work the worker extends
  // its lifetime with.
  w.load = async (path) => {
    let responded;
    const waits = [];
    listeners.fetch({
      request: new Request(ORIGIN + path, { mode: 'no-cors' }),
      respondWith(p) { responded = p; },
      waitUntil(p) { waits.push(p); },
    });
    const res = await responded;
    await Promise.all(waits);
    return { status: res.status, body: await res.text() };
  };
  // apply sends the message a page posts with its page cache writes and waits
  // for the worker to store them.
  w.apply = async (writes) => {
    const waits = [];
    listeners.message({
      data: { type: 'datapages-offline:apply', ...writes },
      waitUntil(p) { waits.push(p); },
    });
    await Promise.all(waits);
  };
  // navigate sends the request of a page navigation.
  w.navigate = async (path) => {
    const request = new Request(ORIGIN + path);
    // The Request constructor refuses mode 'navigate'.
    Object.defineProperty(request, 'mode', { value: 'navigate' });
    let responded;
    listeners.fetch({
      request,
      respondWith(p) { responded = p; },
      waitUntil() {},
    });
    const res = await responded;
    return { status: res.status, body: await res.text() };
  };
  // hydrate sends the request a shim sends for its live document
  // and returns the event stream the worker answers with.
  w.hydrate = async (path) => {
    let responded;
    listeners.fetch({
      request: new Request(ORIGIN + path, { headers: { 'X-Datapages-Shim-Hydrate': '1' } }),
      respondWith(p) { responded = p; },
      waitUntil() {},
    });
    return (await responded).text();
  };
  return w;
}

const ok = (body, cacheControl) => () => new Response(body, {
  status: 200,
  headers: cacheControl === undefined
    ? { 'Content-Type': 'image/png' }
    : { 'Content-Type': 'image/png', 'Cache-Control': cacheControl },
});
const status = (code) => () => new Response('refused\n', { status: code });
const offline = () => { throw new TypeError('Failed to fetch'); };

// A response marked no-store is never stored: every load reaches the server,
// and a refusal reaches the page.
{
  const w = worker();
  w.server = ok('private', 'private, no-store');
  assert.deepEqual(await w.load('/files/1'), { status: 200, body: 'private' });
  assert.equal(w.stored('/files/1'), false, 'a no-store response was stored');
  w.server = status(403);
  assert.equal((await w.load('/files/1')).status, 403);
}

// A response marked no-cache is stored for offline use but revalidated before
// every use: the page gets what the server answers, and the stored copy only
// while the network is down.
{
  const w = worker();
  w.server = ok('v1', 'no-cache');
  assert.deepEqual(await w.load('/files/2'), { status: 200, body: 'v1' });
  assert.equal(w.stored('/files/2'), true);
  w.server = ok('v2', 'no-cache');
  assert.deepEqual(await w.load('/files/2'), { status: 200, body: 'v2' });
  w.server = offline;
  assert.deepEqual(await w.load('/files/2'), { status: 200, body: 'v2' });
  w.server = status(403);
  assert.equal((await w.load('/files/2')).status, 403);
  assert.equal(w.stored('/files/2'), false, 'a refusal kept the stored copy');
}

// A refusal on revalidation removes the stored copy of a reusable response.
// The copy answers once more while the worker revalidates in the background.
for (const code of [401, 403, 404, 410]) {
  const w = worker();
  w.server = ok('v1');
  await w.load('/files/3');
  w.server = status(code);
  assert.deepEqual(await w.load('/files/3'), { status: 200, body: 'v1' });
  assert.equal(w.stored('/files/3'), false, `${code} kept the stored copy`);
  assert.equal((await w.load('/files/3')).status, code);
}

// A partial response, which a video element receives, reaches the page and
// is never offered to cache.put, which refuses it.
{
  const w = worker();
  w.server = () => new Response('part', {
    status: 206,
    headers: { 'Content-Type': 'video/mp4', 'Content-Range': 'bytes 0-3/100' },
  });
  assert.deepEqual(await w.load('/files/5'), { status: 206, body: 'part' });
  assert.deepEqual(w.puts, [], 'a 206 response was offered to cache.put');
}

// An asset installation stored stays stored while the server answers with the
// no-cache of dev mode. The worker asks the network first and uses the copy offline.
{
  const w = worker({ assets: ['/static/app.css'] });
  w.server = ok('body {}', 'no-cache');
  await w.install();
  assert.equal(w.stored('/static/app.css'), true);
  assert.deepEqual(await w.load('/static/app.css'), { status: 200, body: 'body {}' });
  assert.equal(w.stored('/static/app.css'), true, 'an online load removed the asset');
  w.server = offline;
  assert.deepEqual(await w.load('/static/app.css'), { status: 200, body: 'body {}' });
}

// A server error on revalidation keeps the stored copy.
{
  const w = worker();
  w.server = ok('v1');
  await w.load('/files/4');
  w.server = status(500);
  await w.load('/files/4');
  assert.deepEqual(await w.load('/files/4'), { status: 200, body: 'v1' });
}

// dated answers with body, cacheControl, a Date secondsAgo in the past and an
// optional Age, as net/http sends a static asset.
const dated = (body, cacheControl, secondsAgo = 0, age = undefined) => () =>
  new Response(body, {
    status: 200,
    headers: {
      'Content-Type': 'text/css',
      'Cache-Control': cacheControl,
      Date: new Date(Date.now() - secondsAgo * 1000).toUTCString(),
      ...(age === undefined ? {} : { Age: String(age) }),
    },
  });

// A same-origin asset younger than its max-age, counted from its Date and Age
// headers, answers without a request. WithAssetsCache with Immutable sends
// such a max-age for assets whose URLs change with their content.
{
  const w = worker();
  w.server = dated('v1', 'public, max-age=31536000, immutable');
  await w.load('/static/app.css');
  let requests = 0;
  w.server = (r) => { requests++; return dated('v2', 'public, max-age=60')(r); };
  assert.deepEqual(await w.load('/static/app.css'), { status: 200, body: 'v1' });
  assert.equal(requests, 0, 'a fresh asset was revalidated');
}

// An asset past its max-age by its Date or its Age, one with max-age=0, which the
// zero AssetsCacheConfig sends, and one without Date answer from the cache and
// are then revalidated.
for (const [name, first] of [
  ['Date past max-age', dated('v1', 'public, max-age=60', 120)],
  ['Age past max-age', dated('v1', 'public, max-age=60', 0, 120)],
  ['max-age=0', dated('v1', 'public, max-age=0')],
  ['no Date', ok('v1', 'public, max-age=3600')],
]) {
  const w = worker();
  w.server = first;
  await w.load('/static/app.css');
  let revalidations = 0;
  w.server = (r) => { revalidations++; return dated('v2', 'public, max-age=60')(r); };
  assert.deepEqual(await w.load('/static/app.css'), { status: 200, body: 'v1' }, name);
  assert.equal(revalidations, 1, `${name}: not revalidated`);
  assert.deepEqual(await w.load('/static/app.css'), { status: 200, body: 'v2' }, name);
}

// A page entry is stored with the trailing slash the server serves its page with,
// whatever URL it was written for. A navigation without the slash finds it:
// online it reports the held version, offline it gets the entry.
{
  const w = worker();
  await w.apply({ sets: [{ url: '/tickets', html: 'snapshot', version: '3' }] });
  assert.equal(w.stored('/tickets/'), true, 'the entry was stored without the slash');
  for (const path of ['/tickets', '/tickets/']) {
    let held;
    w.server = (r) => {
      held = r.headers.get('X-Datapages-Offline-Version');
      return new Response('live');
    };
    assert.deepEqual(await w.navigate(path), { status: 200, body: 'live' });
    assert.equal(held, '3', `${path} reported no version`);
    w.server = offline;
    assert.deepEqual(await w.navigate(path), { status: 200, body: 'snapshot' });
  }
  await w.apply({ clears: ['/tickets'] });
  assert.equal(w.stored('/tickets/'), false, 'a clear without the slash kept the entry');
}

// A navigation to an installed asset, which has no page entry, gets the asset
// offline.
{
  const w = worker({ assets: ['/static/manual.pdf'] });
  w.server = ok('pdf');
  await w.install();
  w.server = offline;
  assert.deepEqual(await w.navigate('/static/manual.pdf'), { status: 200, body: 'pdf' });
}

// A shim hydrates with the attributes of the live html element, which its
// cached copy lacks, except data-nonce. The classes are added to those the page
// has, such as the offline class. A live html element without attributes adds
// no script.
{
  const w = worker();
  w.server = () => new Response('<!DOCTYPE html><html lang="ar" dir="rtl" ' +
    'class="dark a&amp;b &lt;/script&gt;" data-class:x="$x" itemscope="" ' +
    'title="&#34;&#39;" data-nonce="n"><head></head><body>live</body></html>');
  const frames = await w.hydrate('/tickets/');
  const m = /data: elements <script>(.*)<\/script>/.exec(frames);
  assert.ok(m, 'no script applies the html attributes');
  assert.doesNotMatch(m[1], /<\/script>/, 'a value closes the script element');
  const classes = new Set(['is-offline']);
  const set = {};
  const root = {
    classList: { add: (...c) => c.forEach((x) => classes.add(x)) },
    setAttribute: (k, v) => { set[k] = v; },
  };
  let removed = false;
  vm.runInNewContext(m[1], {
    document: { documentElement: root, currentScript: { remove() { removed = true; } } },
  });
  assert.deepEqual(set, {
    lang: 'ar', dir: 'rtl', 'data-class:x': '$x', itemscope: '', title: `"'`,
  });
  assert.deepEqual([...classes], ['is-offline', 'dark', 'a&b', '</script>']);
  assert.equal(removed, true, 'the script stays in the body');

  w.server = () => new Response('<!DOCTYPE html><html><head></head><body>live</body></html>');
  assert.doesNotMatch(await w.hydrate('/tickets/'), /<script>/);
}
