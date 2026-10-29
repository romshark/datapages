# CLI

The `datapages` command creates projects, generates server code, checks application models and runs the development server.

Install the command with Go:

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
```

[Installation](installation.md) covers templ and the other tools a project needs.

| Command | What it does |
| ------- | ------------ |
| [`datapages init`](#datapages-init) | Creates a project, or rewrites the agent instructions of an existing one |
| [`datapages gen`](#datapages-gen) | Generates the server, `href` and `action` packages of every app package |
| [`datapages lint`](#datapages-lint) | Checks the application model and writes nothing |
| [`datapages watch`](#datapages-watch) | Runs the dev server, which regenerates, rebuilds and reloads on file changes |
| [`datapages version`](#datapages-version) | Prints the version of the CLI |

`datapages help <command>` and `datapages <command> --help` print the help of a command. There is no `--version` flag and no shell completion command.

The commands write their messages to stderr. The progress lines of `init` and the output of `version` and `help` go to stdout. A command that fails prints a line that starts with `error:` and exits with status 1. A usage error, such as an unknown command or flag, exits with 1 too.

## `datapages init`

```text
datapages init [flags]
```

`init` sets up a Datapages project. When needed, it creates the Git repository and Go module. It writes `datapages.yaml`, the app package, development files and agent instructions. It also pins Datapages in `go.mod` and generates the code. It never deletes a file.

Without `-n`, `init` asks for every setting it needs. With `-n` it asks nothing and takes the settings from the flags.

In a project that is set up, `init` rewrites the agent instructions unless you decline them, adds `.env` to `.gitignore` when it is missing there, and prints `Project already initialized.` A project is set up when its Go module has a `datapages.yaml`.

### Where init creates the project

- Outside a project that is set up, `init` asks `Create a subdirectory for the project?`. Yes asks for the `Directory name` and creates the directory in the current directory. The prompt suggests Yes outside a Go module in a directory that holds files other than dotfiles, such as a home directory. `--name` creates the directory without the prompts. `-n` without `--name` uses the current directory.
- A new subdirectory gets a Go module of its own, even inside another module. Without one, `init` uses the root of the Go module that contains the current directory, and prints `Using the Go module in <dir>` when that is not the current directory. Outside a Go module it uses the current directory. In a project that is set up, `init` runs in the module root and asks nothing about the directory.
- Outside a git repository, `init` asks `Not inside a git repository. Create one?` and runs `git init` in the project directory on Yes. No continues without a repository. `--no-git` answers No and `--no-git=false` answers Yes without the prompt, and `-n` answers Yes. A project that is set up gets no repository and no prompt.
- Without a `go.mod`, `init` asks for the `Module path` and runs `go mod init`. The prompt suggests a path made from the URL of the `origin` remote, or else the directory name, and an empty answer takes the suggestion. `--module` gives the path without the prompt, and `-n` requires it.

### Files init writes

`init` writes these files when they are missing:

| File | Content |
| ---- | ------- |
| `datapages.yaml` | The [scaffold configuration](configuration.md#the-scaffold-file) |
| `app/app.go`, `app/app.templ` | An app package with `PageIndex`, `PageError404` and `PageError500`, written only in a module without a `datapages.NewServer` call |
| `cmd/server/main.go` | The entry point, written by the `datapages gen` run of `init` into the [`cmd`](configuration.md#cmd) directory |
| `.env` | `NATS_URL` and a random `SESSION_ENCRYPTION_KEY` |
| `compose.yaml` | A NATS server for `make up` |
| `Makefile` | The targets `dev`, `up`, `down`, `build`, `gen` and `lint`, and `npm-install` and `bundle` for TypeScript Web Components. `gen` runs `datapages gen`, then `templ generate`. With hidden generated code, `gen` runs them in reverse order and `build` runs `gen` first |
| `.vscode/extensions.json` | The templ and Go editor extensions |
| `.vscode/settings.json` | With hidden generated code, hides `datapagesgen` directories and `*_templ.go` files in VS Code |
| `.github/workflows/ci.yml` | With `--ci github-actions`, a CI job that runs `templ generate` and `datapages gen`, runs the tests and builds `./cmd/server/`. With committed generated code, it fails when a generator changes a file |

`init` sets up the project only on a run that creates the git repository, `go.mod`, `datapages.yaml` or `app/app.go`. Only such a run writes the last 7 files of the table. A run that creates none of the 4 writes none of the 7, even when one of them is missing.

On such a run `init` also takes these steps, in order:

1. Adds the `github.com/romshark/datapages` requirement to `go.mod` when it is missing. A release of the CLI, or a build with a pseudo-version, requires its own version after `go list` confirms that Go can download it. Any other build, such as `(devel)`, a `+dirty` build or a commit that Go cannot download, adds a `replace` directive to the checkout it was built from and warns that the path exists only on this machine. Without a checkout, it warns and lets `go mod tidy` pick a version.
2. Runs `npm install` and `node build.mjs` in `web/` for TypeScript Web Components.
3. Runs `templ generate ./app/` with templ v0.3.1100 through `go run`, then `go mod tidy`.
4. Fails when the selected Datapages version has no importable root package, as releases through v0.9.4.
5. Runs [`datapages gen`](#datapages-gen), which writes `app/datapagesgen/` and `cmd/server/main.go`.
6. With Web Components, writes `app/app.templ` again to load them through the generated `assets` package, and runs `templ generate` again.
7. Runs `go mod tidy` again and prints `Project initialized successfully.`
8. Without `-n`, asks `Run the app now?` and runs `make dev` on yes, which starts NATS with Docker Compose and then [`datapages watch`](#datapages-watch).

On every run, `init` asks `Write AI agent instructions and skills?`. Yes rewrites `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, `.cursor/rules/datapages.mdc` and the Datapages skills in `.agents/skills/` and `.claude/skills/`. `--no-ai-skills` answers No and `--no-ai-skills=false` answers Yes without the prompt, and `-n` answers Yes. `init` keeps a file you edited as `<name>.bak` before it replaces it. It leaves alone an `AGENTS.md` that does not end with the Datapages stamp, and skills with other names. See [AI agents](ai-agents.md).

When `init` writes the app package, `--web-components` or a prompt selects the Web Components setup. Each setup adds an example component to the index page:

| Value | Setup |
| ----- | ----- |
| `rocket` | Datastar Rocket components in `app/static/components/`, loaded with the Rocket bundle of the Datastar release Datapages uses |
| `typescript` | TypeScript in `web/src/`, bundled by esbuild into `app/static/components.js`. Needs `npm` and `node` on the PATH. `datapages.yaml` gets a custom watcher that bundles again on every change |
| `javascript` | JavaScript modules in `app/static/components/`, served as they are |
| `none` | No Web Components |

When `init` sets up the project, `--generated-code` or the prompt `Commit generated files?` selects whether git keeps the generated code, `datapagesgen/` and `*_templ.go`:

| Value | Setup |
| ----- | ----- |
| `committed` | git keeps the generated code. A clone builds without the generators, and so does `go install` of the module. Diffs and merges include the generated files |
| `hidden` | `init` adds `datapagesgen/` and `*_templ.go` to `.gitignore` and writes `.vscode/settings.json`. A clone builds only after `templ generate` and `datapages gen`, which `make build` and the CI workflow run. `go install` of the module fails. On a clone, `datapages gen` fails while the app package uses the generated `href`, `action` or `assets` package, because the app package does not compile without them |

`--ci` or the prompt `Set up CI?` selects the CI system that `init` writes a workflow for:

| Value | Setup |
| ----- | ----- |
| `github-actions` | `.github/workflows/ci.yml` for GitHub Actions |
| `none` | No workflow |

### Flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-n`, `--non-interactive` | `false` | Ask nothing. Uses the current directory unless `--name` is set, creates a git repository unless `--no-git` is set, and requires `--module` outside a Go module. Selects `none` for Web Components, `committed` for the generated code and `github-actions` for CI, writes the agent instructions, and skips `Run the app now?` |
| `--name` | `""` | Directory to create in the current directory and to set up a new project in, with a Go module of its own. Also applies in a project that is set up |
| `--no-git` | `false` | Create no git repository. `--no-git=false` creates one without the prompt. Ignored inside a git repository and in a project that is set up |
| `--module` | `""` | Module path for `go mod init`. Ignored when `go.mod` exists |
| `--web-components` | `""` | `rocket`, `typescript`, `javascript` or `none`. Used only when `init` writes the app package. Otherwise `init` prints `Ignored --web-components: init writes no app package here.` |
| `--generated-code` | `""` | `committed` or `hidden`. Used only when `init` sets up the project. Otherwise `init` prints `Ignored --generated-code: the project is already initialized.` |
| `--ci` | `""` | `github-actions` or `none`. Used only when `init` sets up the project. Otherwise `init` prints `Ignored --ci: the project is already initialized.` |
| `--prometheus` | `true` | Names `datapages.EnablePrometheus` in the `cmd/server/main.go` that `init` writes, which serves metrics on `HOST_METRICS:PORT_METRICS`, `localhost:9090` by default. `--prometheus=false` names `datapages.DisablePrometheus` |
| `--no-ai-skills` | `false` | Write no agent instructions and no skills. `--no-ai-skills=false` writes them without the prompt |
| `-h`, `--help` | | Print the help |

### Exit status

0 when `init` set up the project, and when the project was set up before. 1 when a step fails, such as a missing `--module` with `-n` outside a Go module, an unknown `--web-components`, `--generated-code` or `--ci` value, or a failed `git`, `go`, `npm` or `node` command. After `Run the app now?`, a failed `make dev` ends `init` with 1 too.

### Examples

Create a project in the new directory `shop` without prompts:

```sh
datapages init -n --name shop --module example.com/shop --web-components none
```

Set up a project in the current directory without a git repository and without agent instructions:

```sh
datapages init -n --no-git --no-ai-skills --module example.com/shop
```

Update the agent instructions of a project to the installed CLI:

```sh
datapages init -n
```

## `datapages gen`

```text
datapages gen [flags]
```

`gen` generates code for every app package in the module. It reads `go.mod`, `datapages.yaml`, the module's Go files and the `.templ` files rendered by app packages. A successful run prints nothing except the agent-instructions notice described below. It takes these steps:

1. Finds the module and reads `datapages.yaml`. See [Project discovery](#project-discovery).
2. Compares the CLI with the Datapages requirement in `go.mod`. See [Version checks](#version-checks).
3. Finds the app packages from the `datapages.NewServer` calls of the module. It fails when the directory of an app package does not exist.
4. Raises the Datapages requirement in `go.mod` to the CLI version when the CLI is newer.
5. Parses and checks every app package with the rules of [`datapages lint`](#datapages-lint), before it generates any of them.
6. Writes the generated package of each app package that passed. An app package with errors keeps the code an earlier run wrote. One that was never generated gets stubs, which let its imports of the generated packages resolve while you fix the errors.
7. In a module without a `datapages.NewServer` call, writes `main.go` into the [`cmd`](configuration.md#cmd) directory when that directory holds no Go file.
8. Runs `go mod tidy`.

`gen` writes these files:

| Path | Content |
| ---- | ------- |
| `<app>/datapagesgen/app_gen.go` | The server type, routing and handler wiring |
| `<app>/datapagesgen/href/href_gen.go` | Type-safe URL builders |
| `<app>/datapagesgen/action/action_gen.go` | Type-safe Datastar action expressions |
| `<app>/datapagesgen/assets/assets_gen.go` | Asset paths, when the app package declares static files |
| `<cmd>/main.go` | The entry point, written only in a module without a `datapages.NewServer` call |
| `go.mod`, `go.sum` | The raised requirement and the result of `go mod tidy` |

`<app>` is the directory of an app package, such as `app`. `gen` rewrites the files under `datapagesgen/` on every run and removes an `assets` package the app package no longer needs. Do not edit them.

`gen` does not run `templ generate`. It type-checks the app package together with its `*_templ.go` files. After a change to the application model, run `gen` before `templ generate`, which lets templates use the new `href` and `action` helpers. When generated templ code refers to a helper that does not exist yet, the app package does not compile and `gen` fails. Remove the reference, run `templ generate` and `gen`, then restore the reference and run `templ generate` again. See [Troubleshooting](troubleshooting.md).

A release of the CLI prints this line when the skills of the project come from another version:

```text
Agent instructions differ from this CLI; run `datapages init -n` to update them (edits are backed up).
```

### Flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-h`, `--help` | | Print the help |

### Exit status

0 when every app package was generated and `go mod tidy` succeeded. 1 on any error, such as an error in an application model, a `datapages.NewServer` call the scan rejects, an invalid `datapages.yaml`, a `go.mod` that requires a newer Datapages release, or a failed `go mod tidy`. The app packages without errors are generated even when `gen` exits with 1.

### Examples

Regenerate after changing the application model and the templates:

```sh
datapages gen && templ generate && go build ./...
```

Fail a CI job when the committed generated code is out of date, as the `.github/workflows/ci.yml` of a scaffold with committed generated code does:

```sh
datapages gen
git diff --exit-code
```

## `datapages lint`

```text
datapages lint [flags]
```

`lint` parses and checks every app package of the module with the rules of `datapages gen` and writes nothing. It reports the errors of all app packages in 1 run, and the events of different app packages that claim the same subject. The [specification](../SPECIFICATION.md#linting) lists the rules, the checks of `.templ` files included.

`lint` reads `go.mod`, `datapages.yaml`, the Go files of the module and the `.templ` files that the app packages render. It does not change `go.mod` and does not run `go mod tidy`. It fails when `go.mod` requires a newer Datapages release than the CLI, and when `datapages.yaml` is [invalid](configuration.md#validation).

Unless [`watch.toxiproxy.disabled`](configuration.md#disabled) is set, `lint` also reports each endpoint key of [`watch.toxiproxy.presets`](configuration.md#presets) that names no route of any app package of the module. It skips this check while an app package has errors:

```text
error: watch.toxiproxy.presets["Edge"].endpoints: no endpoint has the pattern "GET /not-found/"; did you mean "GET /not-found/{$}"?
```

Each error in an application model is printed as `at <file>:<line>:<column>: <message>`, often followed by a `fix:` line. The file is named without its directory:

```text
at app.go:34:6: page has invalid path comment: PageIndex
fix: First doc comment line must be `// PageIndex is /`; put a blank `//` line before any description

error: parsing app package: 1 error(s)
```

An error in a `datapages.NewServer` call names the file relative to the module root:

```text
error: cmd/admin/main.go:55:3: datapages.NewServer generates into datapagesgen.Server, but app/admin generates into app/admin/datapagesgen
  fix: name example.com/shop/app/admin/datapagesgen.Server
```

### Flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-h`, `--help` | | Print the help |

### Exit status

0 when no app package has an error. 1 when one has, and when the module, the scan or the version check fails.

### Examples

Check the model in CI before the tests:

```sh
datapages lint
go test ./...
```

Keep the colors in a CI log, which is no terminal:

```sh
FORCE_COLOR=1 datapages lint
```

## `datapages watch`

```text
datapages watch [flags]
```

`watch` runs the development server for 1 app package. It regenerates code, rebuilds and restarts the app after file changes, then reloads open browser tabs. Open the `--host` address, which defaults to `http://localhost:7331`. Pages loaded from the app's own address do not reload or show build errors. Ctrl+C stops the process.

`watch` also serves the app through a Toxiproxy proxy, which simulates slow and failing networks, and a settings page for it. It prints both addresses at startup. See [Slow and failing networks](network.md).

`watch` runs [templier](https://github.com/romshark/templier) v0.13.4 as a library and reads its settings from the [`watch` section](configuration.md#watch) of `datapages.yaml`. It needs `templ` on the PATH, and `golangci-lint` when [`lint`](configuration.md#lint) is on.

Start `watch` in the module root. It runs `go build` in the current directory with the path of the main package relative to the module root, as `./cmd/server/`. From a subdirectory that path names no package and the build fails.

### On startup

1. Starts the [update check](#update-check).
2. Finds the module and writes the [watch lock](#watch-lock).
3. Reads `datapages.yaml`. When neither `datapages.yaml` nor `datapages.yml` exists, it writes the [scaffold file](configuration.md#the-scaffold-file), which applies from the next start.
4. Runs `datapages gen` for every app package. It prints the errors and continues.
5. Selects the app package with `--app`, and the main package to build. That is the main package with the `datapages.NewServer` call for the app package, or the [`cmd`](configuration.md#cmd) directory when no main package has one.
6. Unless [`watch.toxiproxy.disabled`](configuration.md#disabled) is set, starts the [Toxiproxy proxy](network.md) and its settings page, and prints `toxiproxy: <proxy URL> (settings: <settings URL>)`. It warns about endpoint keys of the presets that name no route of the app package.
7. Starts `templ generate --watch` in the module root and the dev server on `--host`.
8. Runs the command of each [custom watcher](configuration.md#watchcustom-watchers) once. The first is the built-in watcher that runs `datapages gen`.
9. Builds the main package with `go build` into a temporary directory and starts it in [`dir-work`](configuration.md#dir-work), with the arguments in [`flags`](configuration.md#flags) and `TEMPL_DEV_MODE=true`, which turns on [dev mode](../SPECIFICATION.md#dev-mode).
10. Sends `OPTIONS` requests to [`app-host`](configuration.md#app-host) every 100 ms until the app answers, then reloads the open tabs.

The dev server forwards each request to `app-host`. It adds a script to every HTML response, which connects to the dev server over a WebSocket at `/__templier/events` and reloads the tab when told to.

### On a file change

| Changed file | What happens |
| ------------ | ------------ |
| A `.templ` file | templ generates its Go code, then tells `watch` to reload the tabs or to rebuild the app |
| A `.go` file in or below the directory of an app package, outside `datapagesgen/`. A `.go` file in the directory of a package with a `datapages.NewServer` call | The built-in watcher runs `datapages gen`, then `watch` rebuilds and restarts the app |
| A file that a custom watcher includes | The command of the watcher runs, then the action of its [`requires`](configuration.md#requires) |
| `datapages.yaml` and `datapages.yml` | Nothing, unless a custom watcher includes them. The app keeps running and keeps its state in memory |
| Any other file that [`exclude`](configuration.md#exclude) does not match | `watch` rebuilds and restarts the app |

`watch` builds the new binary while the app runs. Then it stops the app with an interrupt signal, kills it when it has not exited after 5 seconds, and starts the new binary. `watch` reads `datapages.yaml` once. Restart it after changing the `watch` section. Only the presets of `watch.toxiproxy` are read again, by the Reload button of the [settings page](network.md#change-the-toxics).

Besides the files of `datapages gen`, `watch` writes the [watch lock](#watch-lock), the scaffold `datapages.yaml` when the module has none, the `watch.toxiproxy` section of `datapages.yaml` on a Save of the settings page, and the `*_templ.go` files that templ generates. templ writes the text files of its development mode into the system temporary directory, or into `TEMPL_DEV_MODE_ROOT` when it is set. The app binaries go into a `templier-*` directory in the system temporary directory, which `watch` removes when it exits.

While a step fails, the dev server answers every request with an error page instead of forwarding it. The page shows the output of the failed step under its name, such as `templ`, `go`, `golangci-lint`, `process` for an app that exited while starting, or the name of a custom watcher with [`fail-on-error`](configuration.md#fail-on-error), `datapages gen` included. The open tabs reload when a step fails and when it succeeds again.

### Flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--app` | `""` | Directory of the app package to run, relative to the module root, as `app/frontend`. Required when the module has more than 1 app package. The last element alone, as `frontend`, is an error |
| `--host` | `localhost:7331` | Address the dev server listens on. The Toxiproxy proxy forwards to it, and listens on its host unless `watch.toxiproxy.host` is set. Port 0 turns the proxy off |
| `-h`, `--help` | | Print the help |

### Exit status

1 when `watch` cannot start, such as outside a Go module, with an invalid `datapages.yaml`, with more than 1 app package and no `--app`, with an `--app` that names no app package, without `templ` on the PATH, or with a `--host` address in use. 1 also when `datapages.yaml` has a `watch.toxiproxy` section and the proxy cannot start, as on a port in use or on the port of `--host` or `app-host`. Without the section, `watch` warns and runs without the proxy. A failed `datapages gen` at startup does not end `watch`. After Ctrl+C or SIGTERM, `watch` stops the app, templ and the dev server and exits with 0.

### Examples

Run the dev server of a module with 1 app package, as `make dev` of the scaffold does:

```sh
datapages watch
```

Run the app package `app/admin` of a module with 2 app packages:

```sh
datapages watch --app app/admin
```

Serve the dev server on another address, such as while another program uses port 7331:

```sh
datapages watch --host localhost:7332
```

1 module runs 1 `watch` at a time. Every `watch` of the module forwards to the same [`app-host`](configuration.md#app-host), which 2 apps cannot both listen on.

## `datapages version`

```text
datapages version [flags]
```

`version` prints the version of the CLI to stdout and reads no file. A release prints its version without the `v` prefix, such as `0.10.1`. A CLI built from source prints `(devel)` or a pseudo-version. `--full` adds the commit, the build date, the Go version, the operating system and architecture, and the modules the binary was built with.

### Flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--full` | `false` | Print the build information and the dependencies |
| `-h`, `--help` | | Print the help |

### Exit status

0. 1 for a usage error, such as an extra argument or an unknown flag.

### Examples

Print the version:

```sh
datapages version
```

Print the build information to attach to a bug report:

```sh
datapages version --full
```

## Project discovery

### Module

`gen`, `lint` and `watch` work on the Go module whose `go.mod` is in the current directory or the nearest directory above it. Without one they fail with `not inside a Go module (no go.mod found in any parent directory)`. `--app`, the [`cmd`](configuration.md#cmd) key and the file names in errors about `datapages.NewServer` calls are relative to the module root. `gen` and `lint` run from any directory of the module. `watch` runs from the module root. `init` finds its directory as described in [Where init creates the project](#where-init-creates-the-project).

### App packages

The commands find the app packages through the [`datapages.NewServer`](https://pkg.go.dev/github.com/romshark/datapages#NewServer) calls of the module. They read the calls from the syntax tree, without type-checking, which lets them read a `main.go` that names a generated package that does not exist yet:

```go title="cmd/server/main.go"
s, err := datapages.NewServer[
	app.App,                    // the app package, app/
	struct{},                   // the session data type
	datapages.EnablePrometheus, // the metrics mode
	datapagesgen.Server,        // the generated package, app/datapagesgen/
](a, messageBroker, opts...)
```

The scan reads every `.go` file of the module, test files included. It skips:

- directories named `vendor`, `node_modules` or `testdata`, and directories whose names start with `.` or `_`
- directories with a `go.mod` of their own, which belong to another module
- files whose `//go:build` line excludes them from every build, such as `//go:build ignore`

A file with a syntax error fails the scan. So does a call that breaks these rules:

- The call has 4 or 5 type arguments, and the `datapages` package is imported under a name. A dot import is an error.
- The app type lives in a package of its own in the module, which is neither the module root nor a `datapagesgen` package.
- The fourth type argument is the `Server` type of the `datapagesgen` package directly below the app package. Generated code always goes there.
- The third type argument is `datapages.EnablePrometheus` or `datapages.DisablePrometheus`.
- Calls that name the same app package name the same session data type and the same metrics mode.

The scan also rejects every import of a package under `github.com/romshark/datapages/runtime` in a file that Datapages did not generate.

Each app package that a call names is 1 application. `gen` and `lint` process all of them, and `watch` runs the one that `--app` selects. See [Multiple applications](multiple-applications.md).

A module without a call has 1 app package, `app/`, generated into `app/datapagesgen/`. `gen` then writes the entry point `<cmd>/main.go` with a call, which ends this fallback. Without an `app/` directory, the commands fail with `app: no such directory, the default app package of a module without a datapages.NewServer call`.

### Configuration file

See [datapages.yaml](configuration.md).

## Version checks

A release of the CLI compares its version with the `github.com/romshark/datapages` requirement in `go.mod`:

- When `go.mod` requires a newer release, `gen` and `lint` fail with this error. `watch` prints it without `error:` at startup and shows it on its error page after every regeneration:

  ```text
  error: go.mod requires github.com/romshark/datapages v0.11.0 but you are running v0.10.1
    run: go install github.com/romshark/datapages/cmd/datapages@v0.11.0
  ```

- When the CLI is newer, `gen` and the `gen` runs of `watch` raise the requirement to the version of the CLI and run `go mod tidy`. `lint` changes nothing.

A module without the requirement skips both checks, and so does a CLI without a release version, such as `(devel)`, a pseudo-version or a version with build metadata, as `+dirty`. See [Upgrading](upgrading.md).

## Update check

`datapages watch` checks for a newer release when it starts. The other commands do not.

It requests `https://api.github.com/repos/romshark/datapages/releases/latest` in the background, which does not delay the dev server, and gives up after 5 seconds. When the latest release is newer than the CLI, it prints to stderr:

```text
update available: v0.11.0
  run: go install github.com/romshark/datapages/cmd/datapages@latest
changelog: https://github.com/romshark/datapages/releases/tag/v0.11.0
```

A CLI without a release version skips the check. A failed request, such as one without network or over the GitHub rate limit, prints nothing. No flag or environment variable turns the check off.

## Watch lock

`datapages watch` keeps a lock file while it runs. It tells other tools that a dev server uses the module.

`templ generate` deletes the text files that templ development mode reads. A `templ generate` in a module while `watch` runs on it stops the app from rendering its templates. Restart `watch` to write the files again. `datapages gen` does not run `templ generate`. `datapages init` runs it only on a run that sets up a project.

- Directory: `$DATAPAGES_WATCH_LOCK_DIR`, or `datapages-watch` in the system temporary directory, which is `$TMPDIR` or `/tmp` on Unix.
- File: `<hash>.lock`, where `<hash>` is 16 hexadecimal digits of the SHA-256 of the absolute module path. Each module has 1 file.
- Content: 1 line with the process ID, the module directory and the `--host` address, separated by tabs.
- Refresh: `watch` writes the file again every 2 seconds. Readers treat a file older than 10 seconds as stale, which allows 5 missed refreshes.
- Removal: `watch` deletes the file when it exits. A `watch` killed with SIGKILL leaves the file, which goes stale 10 seconds later.
- Failure: when the file cannot be written, `watch` prints `warning:` with the cause and runs without the lock.

No `datapages` command reads the lock. The mage targets of the Datapages repository that run `templ generate` refuse to start while a fresh lock exists. A script of your own can make the same check. A `.lock` file in the directory that changed within the last 10 seconds means that a `watch` runs.

## Environment variables

The CLI reads these variables:

| Variable | Commands | Effect |
| -------- | -------- | ------ |
| `NO_COLOR` | all | Any non-empty value turns ANSI colors off. It overrides the next 2 |
| `FORCE_COLOR` | all | Any non-empty value, `0` included, turns colors on when stderr is no terminal |
| `CLICOLOR_FORCE` | all | `1` turns colors on when stderr is no terminal |
| `DATAPAGES_WATCH_LOCK_DIR` | `watch` | Directory of the [watch lock](#watch-lock) files |
| `DATAPAGES_LOADCACHE` | `gen`, `lint`, `watch`, `init` | `off` turns the load cache off. An absolute path moves it. A relative path turns it off. The default is `datapages/load` in the user cache directory, such as `~/Library/Caches` on macOS and `~/.cache` on Linux |
| `DATAPAGES_LOADCACHE_DEBUG` | `gen`, `lint`, `watch`, `init` | Any non-empty value prints `loadcache:` lines with hits, misses and timings to stderr |
| `GOPACKAGESDRIVER` | `gen`, `lint`, `watch`, `init` | A value other than `off` turns the load cache off. So does a `gopackagesdriver` on the PATH while the variable is unset |

Without these variables, colors are on when stderr is a terminal. Colors apply to the `error:` label and to the errors in application models. The update notice of `watch` is colored when stdout is a terminal, unless `NO_COLOR` is set or `TERM` is `dumb`.

The load cache keeps the package graph and the compiled type information of the standard library and the downloaded modules from the last load of each app package. A later load whose inputs did not change reuses them and runs no `go list`.

The `go` commands that the CLI runs, as `go mod tidy` and `go build`, read the Go environment variables, such as `GOFLAGS` and `GOPROXY`.

`watch` sets these variables for the processes it starts:

| Variable | Value | Set for |
| -------- | ----- | ------- |
| `TEMPL_DEV_MODE_WATCH_ROOT` | The module root | `watch` and every process it starts. templ development mode then applies only to the templates of the module, not to templ components of dependencies |
| `TEMPL_DEV_MODE` | `true` | The app. It turns on templ development mode and Datapages [dev mode](../SPECIFICATION.md#dev-mode) |
| `FORCE_COLOR`, `CLICOLOR_FORCE` | `1` | `go build`, `golangci-lint` and the commands of custom watchers, `datapages gen` included. The error page shows their colors |

## See also

- [datapages.yaml](configuration.md): the `cmd` key and every `watch` setting.
- [Slow and failing networks](network.md): the Toxiproxy proxy of `watch` and its settings page.
- [Installation](installation.md): install the CLI, templ and the tools a project needs.
- [Multiple applications](multiple-applications.md): build several app packages in 1 module and select 1 with `--app`.
- [Upgrading](upgrading.md): move a project and its `go.mod` to a new release.
- [Troubleshooting](troubleshooting.md): generation errors and dev server problems.
- [Linting](../SPECIFICATION.md#linting): the rules that `gen` and `lint` apply to templates.
