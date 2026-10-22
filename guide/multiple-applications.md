# Multiple applications

1 Go module can hold several Datapages applications. Each application has its own app package, generated package and entry point. Applications can share ordinary Go and templ packages, and they can exchange events through a broker.

## Layout

Each application is an app package named by a `datapages.NewServer` call. `datapages gen` writes its generated package directly below the app package. This example module builds a shop and an admin interface. They share an events package and a package of templ components:

```text
.
|-- app/
|   |-- admin/
|   |   |-- app.go
|   |   |-- app.templ
|   |   `-- datapagesgen/
|   `-- frontend/
|       |-- app.go
|       |-- app.templ
|       `-- datapagesgen/
|-- cmd/
|   |-- admin/
|   |   `-- main.go
|   `-- frontend/
|       `-- main.go
|-- events/
|   `-- events.go
|-- ui/
|   `-- layout.templ
|-- datapages.yaml
`-- go.mod
```

The type arguments of each call name the app package and the `Server` type of the `datapagesgen` package below it:

```go title="cmd/admin/main.go"
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging/natscore"

	"example.com/shop/app/admin"
	"example.com/shop/app/admin/datapagesgen"
)

func main() {
	host := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	conn, err := nats.Connect(os.Getenv("NATS_URL"))
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}
	s, err := datapages.NewServer[
		admin.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](&admin.App{}, natscore.New(conn, natscore.Config{}))
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}
	err = s.ListenAndServe(ctx, *host)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
		os.Exit(1)
	}
}
```

`cmd/frontend/main.go` is the same with `frontend` in place of `admin`. The type arguments of different applications are independent. 1 application can declare a session type and count Prometheus metrics while another does neither. Calls that name the same app package must agree on both, also when a test or a second command builds that application. [Project discovery](cli.md#app-packages) lists the rules of the scan. [`example/counter`](../example/counter/) builds the same counter 2 times in this layout, as `app/simple` and `app/fancy`.

## Generate and lint

`datapages gen` and `datapages lint` process every application of the module in 1 run. `datapages gen` parses and checks all of them before it writes any. An application with errors keeps the code that an earlier run generated for it, and the other applications are generated. The command exits with status 1 when any application failed.

In this run, `app/frontend` declares a page without a route comment and without a `GET` method, and `app/admin` gains a page. `datapages gen` prints the errors of the frontend, generates the admin with its new page, and leaves `app/frontend/datapagesgen` as it was:

```text
at broken.go:4:6: page is missing path comment: PageCart
fix: Add `// PageCart is /cart/`

at broken.go:4:6: page is missing the GET handler: PageCart
fix: Add `func (p PageCart) GET(r *http.Request) (body datapages.Component, err error) { return nil, nil }`

error: parsing app package: 2 error(s)
```

Positions are relative to the directory of the app package and do not name it. Each failed application adds 1 `parsing app package` line at the end, in the order of the app directories. Here the module still builds, because the earlier code of `app/frontend/datapagesgen` refers to nothing the failed change removed. `datapages lint` reports the errors of every application in 1 run and writes nothing.

## Run 1 application in development

1 `datapages watch` runs 1 application. In a module with several, pass the directory of the app package to run, relative to the module root:

```sh
datapages watch --app app/admin
```

Without the flag, `datapages watch` stops:

```text
error: this module builds 2 applications (app/admin, app/frontend), pass the directory of the one to run with --app
```

The flag takes the whole directory. `--app admin` fails with `no app package in "admin", the module builds: app/admin, app/frontend`.

The dev server builds and runs the main package that holds the `NewServer` call of the selected application. It reads the `watch` section of the module's 1 `datapages.yaml`, whose settings apply to whichever application runs. Each entry point must therefore accept the arguments in [`flags`](configuration.md#flags) and listen where [`app-host`](configuration.md#app-host) points, `http://localhost:8080` by default. The `-host` flag of the entry points above serves that purpose. Since both applications get the same `flags` and the same `app-host`, 2 watches of 1 module started at the same time compete for that address. The built-in watcher reruns `datapages gen` for every application of the module after a change to any of their Go files, not only for the one that runs. See [`datapages watch`](cli.md#datapages-watch).

## Share events

Applications connected to the same broker share 1 subject namespace. Declare an event that several applications use once, in a package that their app packages import:

```go title="events/events.go"
// Package events provides the events that both applications handle.
package events

// EventAnnouncement is "announcement"
type EventAnnouncement struct {
	Text string `json:"text"`
}
```

The admin application dispatches the event:

```go title="app/admin/app.go"
// POSTAnnounce is /announce
func (PageIndex) POSTAnnounce(
	r *http.Request,
	signals datapages.Signals[struct {
		Text string `json:"text"`
	}],
	announcement datapages.Dispatcher[events.EventAnnouncement],
) error {
	return announcement.Dispatch(events.EventAnnouncement{
		Text: signals.Values.Text,
	})
}
```

The frontend handles it:

```go title="app/frontend/app.go"
func (PageIndex) OnAnnouncement(
	event events.EventAnnouncement, sse datapages.SSE,
) error {
	return sse.PatchElement(announcement(event.Text))
}
```

The handler is named after the type without its `Event` prefix, as for an event of the app package. Datapages reads the subject, the payload and the subject fields from the declaring package. An app package can reach the type through any chain of imports, and an alias such as `type EventAnnouncement = events.EventAnnouncement` names the same event. 1 application must not use 2 event types of the same name, even from different packages.

The event reaches the frontend only through a broker that both servers use. `modules/messaging/natscore` connects server processes through NATS, as in the entry points above. 2 servers in 1 process, such as in a test, can share 1 `modules/messaging/inmem` broker. With both applications served from 1 process on 1 in-memory broker, a `POST /announce/` to the admin server sends this patch on the stream of an open frontend page:

```text
event: datastar-patch-elements
data: elements <p id="announcement">Sale starts at 9</p>
```

Applications that share an event addressed to a user, with a `datapages.SubjectUser` field, must use the same user IDs. See [Events declared outside the application package](../SPECIFICATION.md#events-declared-outside-the-application-package).

### Subject conflicts

2 different event types must not claim the same subject anywhere in the module. A broker that serves both applications would deliver the messages of each to the streams of the other. `datapages gen` and `datapages lint` compare the subjects of all applications. Here `app/admin` and `app/frontend` each declare `// EventOrderPlaced is "order.placed"`:

```text
error: app/frontend.EventOrderPlaced claims subject "order.placed" and app/admin.EventOrderPlaced claims "order.placed": 2 applications of 1 module must not claim the same subject
```

Both applications keep the code that an earlier run generated for them, and any other application of the module is generated. Declare the event once in a shared package, as above, or give each application a subject of its own, such as `admin.order.placed`. An event with subject fields claims every subject below its own, which makes `"notify"` with a subject field conflict with `"notify.user"`.

## Share code

Any Go package of the module can serve several applications, such as a repository or the domain logic behind `App`. Pages, actions, event handlers and stream embeds stay in each app package. A page embeds only stream embeds of its own package. Put logic that several applications need into ordinary functions and call them from the handlers of each.

Templ components can live in a shared package too. `datapages gen` and `datapages lint` check the `.templ` files of every module package that an app package imports, with the rules of the app package, and report an error once for each application that imports the package. A link must come from the generated `href` package, and each application has its own. A shared component therefore cannot write a link from a URL it receives:

```templ title="ui/back.templ"
package ui

templ Back(url string) {
	<a href={ templ.SafeURL(url) }>Back</a>
}
```

```text
at back.templ:4:12: href expression must use href package functions: templ.SafeURL(url)
fix: Use href={ href.Xxx(...) } from the generated href package, or href={ href.External(url) } for external URLs instead of "templ.SafeURL(url)"
```

Let each application render its links and pass them into the shared component instead:

```templ title="ui/layout.templ"
package ui

templ Layout(title string, nav templ.Component) {
	<nav>
		@nav
	</nav>
	<main>
		<h1>{ title }</h1>
		{ children... }
	</main>
}
```

```templ title="app/admin/app.templ"
package admin

import (
	"example.com/shop/app/admin/datapagesgen/action"
	"example.com/shop/app/admin/datapagesgen/href"
	"example.com/shop/ui"
)

templ pageIndex() {
	@ui.Layout("Admin", nav()) {
		<input data-bind:text/>
		<button data-on:click={ action.PageIndex.Announce.POST() }>Announce</button>
	}
}

templ pageUsers() {
	@ui.Layout("Users", nav()) {
		<p>No users yet.</p>
	}
}

templ nav() {
	<a href={ href.PageIndex() }>Admin</a>
	<a href={ href.PageUsers() }>Users</a>
}
```

Pass buttons that call actions the same way. The `action` package, like `href`, belongs to 1 application, and the linter checks that a page renders only its own actions and those of `App`.

## See also

- [Events declared outside the application package](../SPECIFICATION.md#events-declared-outside-the-application-package): the rules for shared event types and subjects.
- [Application model](application-model.md): how the CLI finds an app package and what it generates.
- [CLI](cli.md#datapages-watch): `datapages watch`, its `--app` and `--host` flags and the watch lock.
- [datapages.yaml](configuration.md): the `watch` settings that every application of a module shares.
- [Events](events.md): dispatchers, subjects and event handlers.
- [`example/counter`](../example/counter/): 2 applications in 1 module.
