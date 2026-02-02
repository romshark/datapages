import http from 'k6/http';
import { sleep, check } from 'k6';

export const options = {
  vus: 10,
  duration: '5m',
};

const HOST = __ENV.HOST || 'localhost';
const PORT = __ENV.PORT || '8080';
const BASE = `http://${HOST}:${PORT}`;

export default function () {
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
        headers: { 'X-Requested-With': 'Datastar' },
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
