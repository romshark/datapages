import http from 'k6/http';
import { sleep, check } from 'k6';

export const options = {
  vus: parseInt(__ENV.VUS || '10', 10),
  duration: __ENV.DURATION || '1m',
};

const HOST = __ENV.HOST || 'localhost';
const PORT = __ENV.PORT || '8080';
const SCHEME = __ENV.SCHEME || (PORT === '443' ? 'https' : 'http');
const BASE = `${SCHEME}://${HOST}:${PORT}`;

const CSRF_BYPASS = __ENV.CSRF_DEV_BYPASS || '';

// The server injects a fetch shim into authenticated HTML pages containing
// the per-session CSRF token:
//   h.set("X-CSRF-Token",'<token>')
const CSRF_TOKEN_RE = /X-CSRF-Token"\s*,\s*'([^']+)'/;

function dsHeaders(token) {
  const h = { 'Datastar-Request': 'true' };
  if (CSRF_BYPASS) h['X-CSRF-Token'] = CSRF_BYPASS;
  else if (token) h['X-CSRF-Token'] = token;
  return h;
}

// Test users from testdata.go (username: password).
const USERS = [
  { user: 'testuser', pass: 'testuser' },
  { user: 'julianf92', pass: 'julian123' },
  { user: 'fabiberg', pass: 'fabipass' },
  { user: 'kaiy', pass: 'kaiypass1' },
  { user: 'lorentz553', pass: 'lorentzpw' },
  { user: 'gretschen', pass: 'gretschpw' },
];

// SCENARIO selects which scenario the default export runs.
// Accepted values: "full" (default), "homepage", "search".
const SCENARIO = __ENV.SCENARIO || 'full';

export default function () {
  if (SCENARIO === 'homepage') return homepageSmoke();
  if (SCENARIO === 'search') return searchSmoke();
  return fullFlow();
}

// fullFlow: log in, browse for a while, log out.
function fullFlow() {
  const cred = USERS[__VU % USERS.length];

  // Log in.
  const loginRes = http.post(
    `${BASE}/login/submit/`,
    JSON.stringify({
      emailorusername: cred.user,
      password: cred.pass,
    }),
    {
      headers: {
        ...dsHeaders(''),
        'Content-Type': 'application/json',
      },
      tags: { endpoint: 'login', type: 'session' },
    },
  );
  check(loginRes, {
    'login ok': (r) => r.status < 400,
  });
  sleep(0.5 + Math.random() * 0.5);

  // Fetch index to pick up the CSRF token from the injected fetch shim.
  let csrfToken = '';
  const primer = http.get(`${BASE}/`, {
    tags: { endpoint: 'index', type: 'page' },
  });
  const m = CSRF_TOKEN_RE.exec(primer.body || '');
  if (m) csrfToken = m[1];

  // Browse 5-15 pages while logged in.
  const pages = 5 + Math.floor(Math.random() * 11);
  for (let i = 0; i < pages; i++) {
    let res;
    const r = Math.random();

    if (r < 0.60) {
      res = http.get(`${BASE}/`, {
        tags: { endpoint: 'index', type: 'page' },
      });
    } else if (r < 0.75) {
      res = http.get(`${BASE}/messages/`, {
        tags: { endpoint: 'messages', type: 'page' },
      });
    } else if (r < 0.85) {
      res = http.get(`${BASE}/search/`, {
        tags: { endpoint: 'search', type: 'page' },
      });
    } else if (r < 0.95) {
      res = http.post(
        `${BASE}/cause-500-internal-error/`,
        null,
        {
          headers: dsHeaders(csrfToken),
          tags: { endpoint: 'cause-500', type: 'error' },
        },
      );
    } else {
      res = http.get(`${BASE}/whoops/`, {
        tags: { endpoint: 'whoops', type: 'error-page' },
      });
    }

    check(res, {
      'status is valid': (r) => r.status < 600,
    });

    sleep(0.2 + Math.random() * 0.3);
  }

  // Log out.
  const logoutRes = http.post(
    `${BASE}/sign-out/`,
    null,
    {
      headers: dsHeaders(csrfToken),
      tags: { endpoint: 'sign-out', type: 'session' },
    },
  );
  check(logoutRes, {
    'sign-out ok': (r) => r.status < 400,
  });
  sleep(0.3 + Math.random() * 0.3);
}

// homepageSmoke hammers the unauthenticated index page.
export function homepageSmoke() {
  const res = http.get(`${BASE}/`, {
    tags: { endpoint: 'index', type: 'page' },
  });
  check(res, {
    'homepage 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  sleep(0.1 + Math.random() * 0.2);
}

// Search terms with a mix of hits and misses against the seed dataset.
const SEARCH_TERMS = [
  'bike', 'car', 'laptop', 'table', 'phone',
  'chair', 'book', 'camera', 'guitar', '',
];
const SEARCH_CATEGORIES = ['', 'vehicles', 'electronics', 'furniture'];

// searchSmoke benchmarks /search/ with varied query parameters.
export function searchSmoke() {
  const t = SEARCH_TERMS[Math.floor(Math.random() * SEARCH_TERMS.length)];
  const c = SEARCH_CATEGORIES[Math.floor(Math.random() * SEARCH_CATEGORIES.length)];
  const params = [];
  if (t) params.push(`t=${encodeURIComponent(t)}`);
  if (c) params.push(`c=${encodeURIComponent(c)}`);
  const qs = params.length ? `?${params.join('&')}` : '';
  const res = http.get(`${BASE}/search/${qs}`, {
    tags: { endpoint: 'search', type: 'page' },
  });
  check(res, {
    'search 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  sleep(0.1 + Math.random() * 0.2);
}
