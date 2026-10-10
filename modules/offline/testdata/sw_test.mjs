// sw_test.mjs runs the offline worker in a VM against a fake origin and checks
// how it stores and serves the responses of requests that are not navigations,
// such as an img element loading a GET action.
//
// Usage: node sw_test.mjs path/to/sw.js
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const ORIGIN = 'https://app.test';
const script = fs.readFileSync(process.argv[2], 'utf8')
  .replaceAll('__CONFIG__', JSON.stringify({ workerVersion: 1 }));

// worker starts the script with an empty Cache Storage. Every fetch it makes
// is answered by server(), which a test replaces between loads.
function worker() {
  const listeners = {};
  const store = new Map();
  const key = (r) => (typeof r === 'string' ? new URL(r, ORIGIN).href : r.url);
  const cache = {
    async match(r) { const hit = store.get(key(r)); return hit && hit.clone(); },
    async put(r, res) { store.set(key(r), res); },
    async delete(r) { return store.delete(key(r)); },
    async keys() { return [...store.keys()].map((u) => new Request(u)); },
    async add() {},
  };
  const w = {
    server: () => { throw new Error('no server'); },
    stored: (path) => store.has(ORIGIN + path),
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
    Request, Response, Headers, URL, Map, Promise, setTimeout, console,
  }));
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

// A server error on revalidation keeps the stored copy.
{
  const w = worker();
  w.server = ok('v1');
  await w.load('/files/4');
  w.server = status(500);
  await w.load('/files/4');
  assert.deepEqual(await w.load('/files/4'), { status: 200, body: 'v1' });
}
