# Web Components + esbuild Example

This example demonstrates how to integrate a TypeScript [Web Component](https://developer.mozilla.org/en-US/docs/Web/API/Web_components) bundle into a Datapages project using [esbuild](https://esbuild.github.io) through a custom watcher, so edits to `.ts`, `.templ`, and `.go` all hot-reload through one dev loop.

The example is a minimal "flash sale" landing page with two Web Components, showing both flavors you're likely to want in a real project:

- [`<wc-counter>`](./js/src/wc-counter.ts) — a **vanilla** shadow-DOM custom element with no runtime dependencies.
- [`<wc-particles>`](./js/src/wc-particles.ts) — built with [**Lit**](https://lit.dev).

https://github.com/user-attachments/assets/34554ecd-cb9b-485d-976a-405f036e0528

## How It Works

The `datapages.yaml` defines a custom watcher that runs esbuild whenever any TypeScript source in `js/src/` changes:

```yaml
custom-watchers:
  - name: "Bundle web components (esbuild)"
    include:
      - "js/src/**/*.ts"
    cmd: "cd js && node build.mjs"
    fail-on-error: true
    requires: reload
```

[`js/build.mjs`](./js/build.mjs) bundles
[`js/src/index.ts`](./js/src/index.ts) into `app/static/bundle.js`.
The page references it via `assets.Path("bundle.js")` so the URL is
consistent in dev and production.

In dev mode, static files are served directly from `./app/static/` on disk, so a fresh bundle is picked up on browser reload without restarting the Go server. In production, `app/static/` is embedded into the binary via `//go:embed`.

## Prerequisites

- Go (see `go.mod` for the required version)
- Node.js (for esbuild and Lit)

```sh
# Install JS dependencies (esbuild, TypeScript, Lit).
make npm-install
```

## Run in Dev Mode

```sh
make dev
```

The Datapages watcher proxy starts at `http://localhost:7331`. The app itself listens on `http://localhost:8080` (configure via `HOST`/`PORT` env vars).

Edit any file under `js/src/` — esbuild rebuilds the bundle and the browser reloads.
