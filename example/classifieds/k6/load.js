import http from 'k6/http';
import { sleep, check } from 'k6';

export const options = {
  vus: 10,
  duration: '5m',
};

const HOST = __ENV.HOST || 'localhost';
const PORT = __ENV.PORT || '8080';
const BASE = `http://${HOST}:${PORT}`;

const CSRF_BYPASS = __ENV.CSRF_DEV_BYPASS || '';

const DS_HEADERS = {
  'Datastar-Request': 'true',
  ...(CSRF_BYPASS ? { 'X-CSRF-Token': CSRF_BYPASS } : {}),
};

// Test users from testdata.go (username: password).
const USERS = [
  { user: 'testuser', pass: 'testuser' },
  { user: 'julianf92', pass: 'julian123' },
  { user: 'fabiberg', pass: 'fabipass' },
  { user: 'kaiy', pass: 'kaiypass1' },
  { user: 'lorentz553', pass: 'lorentzpw' },
  { user: 'gretschen', pass: 'gretschpw' },
];

// Each VU logs in, browses for a while, then logs out.
export default function () {
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
        ...DS_HEADERS,
        'Content-Type': 'application/json',
      },
      tags: { endpoint: 'login', type: 'session' },
    },
  );
  check(loginRes, {
    'login ok': (r) => r.status < 400,
  });
  sleep(0.5 + Math.random() * 0.5);

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
          headers: DS_HEADERS,
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
      headers: DS_HEADERS,
      tags: { endpoint: 'sign-out', type: 'session' },
    },
  );
  check(logoutRes, {
    'sign-out ok': (r) => r.status < 400,
  });
  sleep(0.3 + Math.random() * 0.3);
}
