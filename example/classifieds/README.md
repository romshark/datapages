# Demo: Classifieds

A demo application resembling an online classifieds marketplace.

The code you'd write is in
[app](https://github.com/romshark/datapages/tree/main/example/classifieds/app)
(the "source package").
The code that the generator produces is in
[datapagesgen](https://github.com/romshark/datapages/tree/main/example/classifieds/datapagesgen).

## Development Mode

```sh
make dev
```

You can then access:
- Preview: http://localhost:52000/
- Grafana Dashboards: http://localhost:3000/
- Prometheus UI: http://localhost:9091/

You can install [k6](https://k6.io/) and run `make load` in the background
to generate random traffic.
Increase the number of virtual users (`VU`) to apply more load to the server when needed.

## Load Tests

Start the server first with either:

- `make stage` (`./.env.stage`; recommended for smoke testing)
- or `make dev` (`./.env.dev`)

then in another terminal run one of:

| Target                     | What it does                                   |
| -------------------------- | ---------------------------------------------- |
| `make load`                | Full flow: login, browse, sign-out             |
| `make load-smoke-homepage` | Hits `/` only                                  |
| `make load-smoke-search`   | Hits `/search/` with varied query params       |

Pick the env file with `LOAD_ENV` (default `./.env.dev`):

```sh
LOAD_ENV=./.env.stage make load-smoke-search
```

### Tuning

- `VUS` — number of **virtual users**. Each VU is an independent worker that
  runs the scenario in a loop (one request, wait, next request). `VUS=50`
  means 50 such loops running in parallel, so up to 50 requests can be in
  flight at once. This is the main knob for how much load the server sees.
  Default is `10`.
- `DURATION` — how long to run (`30s`, `2m`, `1h`). Default is `1m`.

```sh
LOAD_ENV=./.env.stage VUS=200 DURATION=10s make load-smoke-homepage
```

Use ≥30s to get meaningful p95/p99.

### Env vars read by the script

- `HOST`, `PORT` — server address.
- `SCHEME` — `http` or `https`. Auto-picks `https` when `PORT=443`.
- `CSRF_DEV_BYPASS` — if set, used as `X-CSRF-Token`. Otherwise the full-flow
  script scrapes the token from the page.

For stage (HTTPS with mkcert), run `mkcert -install` once or set
`K6_INSECURE_SKIP_TLS_VERIFY=true`.

## Production Mode

```sh
make stage
```
