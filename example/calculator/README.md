# Calculator

A basic calculator with server-side expression evaluation.
Demonstrates the fat-morph pattern: the UI sends the expression to the server,
which evaluates it and returns the updated page.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Mage](https://magefile.org/) build tool
- [Datapages](https://github.com/romshark/datapages) CLI (for `datapages watch`)
- [Maestro](https://maestro.dev) CLI (for E2E tests only):
  [install instructions](https://docs.maestro.dev/maestro-cli/how-to-install-maestro-cli)

## Run

### Server

```sh
go run ./cmd/server
```

Then open http://localhost:8080/.

### Desktop

```sh
go run ./cmd/desktop
```

Opens the calculator in a native webview window (WebKit on macOS, Edge on Windows).

## Develop

```sh
datapages watch
```

Then open http://localhost:7331/.

## E2E Tests

End-to-end UI tests use [Maestro](https://maestro.dev),
a free and open-source ([Apache 2.0](https://github.com/mobile-dev-inc/maestro/blob/main/LICENSE))
E2E testing framework for mobile and web.
Each YAML flow in `.maestro/` is a self-contained test scenario.

```sh
mage maestroTest
```
