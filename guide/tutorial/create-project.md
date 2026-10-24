# Create the project

Run `datapages init` in a directory outside any Git repository:

```sh
datapages init
```

Answer its questions like this:

| Question | Answer |
| -------- | ------ |
| Not inside a git repository. Create one? | Yes |
| Directory name | `shoplist` |
| No go.mod found. Initialize a Go module? | Yes |
| Module path | `example.com/shoplist` |
| Set up Web Components? | None |
| Run the app now? | No |

This command creates the same project without questions:

```sh
datapages init -n --name shoplist --module example.com/shoplist --web-components none
```

Start the development server:

```sh
cd shoplist
make dev
```

`make dev` starts NATS in Docker with `docker compose up -d`, then runs `datapages watch`. NATS carries the events and stores the sessions of the app. Answering Yes to "Run the app now?" runs `make dev` for you.

Open http://localhost:7331, the address of the dev server. The page shows "Welcome to Datapages". The app itself listens on `localhost:8080`, the default `PORT` of `cmd/server/main.go`, and the dev server forwards each request to it. Only a page loaded through `localhost:7331` reloads after a rebuild and shows build errors. `datapages watch` prints no URL of its own.

You edit 3 files:

- `app/app.go` declares the `App` type, the pages, their actions and the events, which together form the application model.
- `app/app.templ` holds the [templ](https://templ.guide) components that render the HTML.
- `cmd/server/main.go` creates the NATS broker and session manager and starts the server.

Do not edit `app/app_templ.go`, which templ writes, or `app/datapagesgen/`, which `datapages gen` writes.

Keep `make dev` running for the whole tutorial. When you save a file, `datapages watch` does this:

1. For `app/app.templ`, templ writes `app/app_templ.go`.
2. For a Go file in `app/` or `cmd/server/`, `app/app_templ.go` included, `datapages gen` rewrites `app/datapagesgen/`.
3. It builds and restarts the app, then reloads the open tabs.

When a step fails, the tabs reload and show an error page with the output of the failed step, such as `datapages gen` or `go`. The next successful build replaces it. [`datapages watch`](../cli.md#datapages-watch) describes each step, and [datapages.yaml](../configuration.md) its settings.
