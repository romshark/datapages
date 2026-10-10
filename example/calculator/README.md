# Calculator

[Demo video](https://github.com/user-attachments/assets/3806da2c-c07f-4595-8329-334f8014a364)

The calculator evaluates expressions on the server. It uses [`shopspring/decimal`](https://github.com/shopspring/decimal) to avoid binary floating-point rounding. The UI supports buttons, keyboard input, and copy and paste.

## How it works

`GET /` renders the calculator with three Datastar signals: `input`, `fresh` and `num`. A button press sends the signals to `QUERY /input/` with the button in the `btn` query parameter. A paste sets `num` to the pasted number and sends the signals to `QUERY /input/?paste=true`. `PageIndex.QUERYInput` calculates the next values and returns an SSE patch. The patch updates the calculator and its signals.

The server stores no calculator state between requests. Each tab sends its own signals. The QUERY response uses `datapages.SSE`, but the page has no persistent SSE stream or event handler. See [the todo list](../todolist) for server-side per-tab state and events.

Both the HTTP server and the [Wails v3](https://v3.wails.io/) desktop app run the same `app` package. `NewServer` requires a message broker. Both use an in-memory broker even though the calculator dispatches no events. The app's inline JavaScript handles keyboard shortcuts and clipboard input.

## Requirements

- Go 1.27.2 or later
- [Mage](https://magefile.org/)
- `templ` and [Datapages](https://github.com/romshark/datapages) CLIs for code generation and development
- [Maestro](https://maestro.dev/) for UI tests only

## Run

Run the HTTP server and open <http://localhost:8080/>:

```sh
mage runServer
```

Run the desktop app:

```sh
mage runDesktop
```

## Develop

Run `datapages watch` and open <http://localhost:7331/>:

```sh
mage dev
```

## UI tests

Maestro runs the flows in `.maestro/` against the HTTP server:

```sh
mage testUIWorkflows
```
