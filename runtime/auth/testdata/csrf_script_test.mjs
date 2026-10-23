// csrf_script_test.mjs runs the script of auth.Manager.WriteCSRFScript in a VM
// on a page of a fake origin and checks which fetches carry X-CSRF-Token.
//
// Usage: node csrf_script_test.mjs path/to/script.html
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const ORIGIN = 'https://app.test';
const script = fs.readFileSync(process.argv[2], 'utf8')
  .replace(/^\s*<script[^>]*>/, '')
  .replace(/<\/script>\s*$/, '');

// PageRequest resolves a relative URL against the page, as a browser does.
class PageRequest extends Request {
  constructor(input, init) {
    super(typeof input === 'string' ? new URL(input, ORIGIN + '/page') : input, init);
  }
}

// sent holds the requests the script passes on to the network.
const sent = [];
const page = vm.createContext({
  location: new URL(ORIGIN + '/page'),
  fetch: async (r) => { sent.push(r); return new Response(null, { status: 204 }); },
  Request: PageRequest, Headers, URL,
});
vm.runInContext(script, page);

const datastar = { 'Datastar-Request': 'true' };
for (const [name, [url, init, want]] of Object.entries({
  'POST': ['/action', {
    method: 'POST', headers: datastar
  }, true],
  'POST to an absolute URL': [ORIGIN + '/action', {
    method: 'POST', headers: datastar
  }, true],
  'POST to another origin': ['https://other.test/action', {
    method: 'POST', headers: datastar
  }, false],
  'POST to a scheme-relative URL': ['//other.test/action', {
    method: 'POST', headers: datastar
  }, false],
  'POST to another port': [ORIGIN + ':8443/action', {
    method: 'POST', headers: datastar
  }, false],
  'GET': ['/action', {
    method: 'GET', headers: datastar
  }, false],
  'POST without Datastar-Request': ['/action', {
    method: 'POST'
  }, false],
})) {
  sent.length = 0;
  await page.fetch(url, init);
  assert.equal(sent.length, 1, name);
  assert.equal(sent[0].headers.has('X-CSRF-Token'), want, name);
}
