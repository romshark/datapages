# Desktop multiwindow

A [Wails v3](https://wails.io/) desktop app with an SSE stream in each window. Each window has its own port, so you can open more than 6.

## The limit

A webview opens at most 6 HTTP/1.1 connections per host and port. The SSE stream of each window holds 1 of them for as long as the window is open. With 6 windows on 1 port, no other request gets a connection. The buttons of every window stop working, and a 7th window never loads.

Both webview engines count the limit per host and port. A page on port A that opens 7 streams to port A and 1 to port B gets streams 1 to 6 on A and the stream on B, in WKWebView on macOS and in Chrome, the engine of WebView2. WebKitGTK uses libsoup, whose connection manager keys its per-host table by host and port (`soup_host_uri_hash` in `soup-connection-manager.c`).

HTTP/2 would carry all streams over 1 connection, but webviews speak HTTP/2 only over TLS, which needs a certificate pinned in each platform's webview.

## How it works

1. [`main.go`](main.go) creates 1 `http.Server`, `windows`, whose handler is [`guard.Handler`](guard/guard.go) in front of the Datapages server. The Datapages server never listens itself.
2. `wailsDesktop.OpenWindow` listens on `127.0.0.1:0` for each window, serves the listener with `windows` and opens a Wails window on `guard.URL`, which carries the launch token. It closes the listener when the window closes. `wailsDesktop.SetTitle` and `wailsDesktop.Focus` find the window by the port it loads from. Wails does not copy `<title>` to the title bar.
3. On shutdown, `s.Shutdown` ends the SSE streams, and `windows.Shutdown` waits for their handlers.
4. [`app/app.go`](app/app.go) lists the connected windows. `StreamOpen` registers the window of a stream with the port of its request and sets its title to its number, and `StreamClose` removes it. Both dispatch `EventWindowsChanged`, which re-renders the list in every window. The list marks a port that several windows share. `POSTPing` dispatches `EventPinged` to all windows. `POSTOpenWindow` calls the `Desktop` that `main.go` implements. Clicking another window in the list calls `POSTFocus`, which brings that window to the front. `GET` returns `datapages.EnableBackgroundStreaming(true)`, which keeps the stream of a minimized window open.

On macOS, [`firstwindow_darwin.go`](firstwindow_darwin.go) opens the first window once the app is active. A window shown earlier joins the active Space, which is the Space of a fullscreen editor that ran `go run .`.

## Access control

Every process on the machine can connect to a port on `127.0.0.1`, under any user. [`guard.Handler`](guard/guard.go) admits only the requests of the app's windows:

- `main.go` makes a launch token with `crypto/rand.Text` on every launch. Only the windows get it, in the URL they open.
- The first request of a window sets the token as an `HttpOnly`, `SameSite=Strict` cookie and redirects to the URL without it. The cookie reaches the ports of all windows, because cookies ignore the port.
- A request without the token gets 403, and the guard logs it.
- A request whose `Host` is not the address of the listener that accepted it gets 403. This rejects a page of another site whose name resolves to `127.0.0.1` (DNS rebinding).

A process that runs as the same user and reads the memory of the app or the webview's cookie storage still gets the token. Windows, and Linux without Yama `ptrace_scope`, let such a process inject code into the app too, which no transport prevents. [`cmd/server`](cmd/server/main.go) has no guard, because it only serves `datapages watch` and a browser during development.

## Trade-offs

- Each port is a separate origin. `localStorage`, IndexedDB, Cache Storage, service workers and the HTTP cache are per window. Server-side state and events are shared. Cookies ignore the port (RFC 6265 section 8.5), which shares a session cookie between windows.
- The limit of 6 connections applies per window. A window that holds its stream, an SSE action response, an upload and a download at once is still limited to 6.

## Requirements

- Go 1.27.4 or later
- A C compiler, for cgo
- On Linux, GTK 4 and WebKitGTK 6.0, such as the Debian packages `libgtk-4-dev` and `libwebkitgtk-6.0-dev`

## Run

Run the desktop app:

```sh
go run .
```

Load all windows from 1 port, where the 6th window takes the last connection:

```sh
go run . -shared-port
```

Run the server for a browser and open <http://localhost:8080/>. The page links to new tabs, which share the port:

```sh
go run ./cmd/server
```

## Develop

`datapages watch` runs `cmd/server`. Open <http://localhost:7331/>:

```sh
datapages watch
```
