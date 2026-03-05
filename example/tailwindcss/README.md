# TailwindCSS Example

Demonstrates how to integrate [TailwindCSS](https://tailwindcss.com) with
Datapages using a custom watcher that rebuilds `output.css` whenever
`.templ` files or `input.css` change.

## How It Works

The `datapages.yaml` defines a custom watcher:

```yaml
custom-watchers:
  - name: "TailwindCSS"
    include:
      - "app/**/*.templ"
      - "input.tw.css"
    cmd: "npx tailwindcss -i ./input.tw.css -o ./app/static/output.css"
    fail-on-error: true
    requires: reload
```

Whenever a watched file changes, Datapages runs the `tailwindcss` command to
regenerate `app/static/output.css`, then triggers a browser reload.

TailwindCSS v4 uses a CSS-first configuration. `input.css` imports the
framework and declares which source files to scan for utility classes:

```css
@import "tailwindcss";
@source "./app/**/*.templ";
```

No `tailwind.config.js` is needed.

## Prerequisites

Install TailwindCSS v4. The `package.json` pins the version:

```sh
# Option A: npm (version pinned in package.json)
# In v4 the CLI is a separate package: @tailwindcss/cli
npm install
npx tailwindcss --version  # verify

# Option B: standalone binary (no Node.js needed)
# Download from https://github.com/tailwindlabs/tailwindcss/releases/latest
# and place `tailwindcss` in your PATH.
```

## Run in Dev Mode

```sh
datapages watch
```

The watcher proxy starts at `http://localhost:7331`. The app itself listens
on `http://localhost:8080` (configure via `HOST`/`PORT` env vars or `.env`).

## Build for Production

```sh
# Generate the final CSS
npx tailwindcss -i ./input.tw.css -o ./app/static/output.css --minify

# Build the server binary
go build -o server ./cmd/server/
```
