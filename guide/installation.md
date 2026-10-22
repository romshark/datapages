# Installation

Install the Datapages CLI and templ, then use `datapages init` to create a project. The generated `make dev` target starts its dependencies and development server.

## Requirements

| Tool | Version | Used by |
| ---- | ------- | ------- |
| [Go](https://go.dev/dl/) | 1.27.3 or later, the `go` directive of the Datapages module | Everything |
| [templ](https://templ.guide) | v0.3.1070 | `templ generate`, and `datapages watch`, which runs `templ` from the `PATH` |
| Git | any | `datapages init`, which creates a repository when the directory is not in one |
| [Docker](https://www.docker.com/) with Compose | any | The NATS server of a new project, which `make dev` starts |
| make | any | The targets of the generated `Makefile` |
| [Node.js](https://nodejs.org/) with npm | any | Only `datapages init --web-components typescript`, which installs and runs esbuild |

Use the templ version named by the `github.com/a-h/templ` requirement in `go.mod`. A new project requires v0.3.1070. Generated templ files record that version. The generated CI workflow fails when `templ generate` changes a committed file.

Docker is needed only for NATS. [Run without Docker](#run-without-docker) shows how to start a project with the in-memory modules instead.

## Install the CLI and templ

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
go install github.com/a-h/templ/cmd/templ@v0.3.1070
```

`go install` writes both programs to `$GOBIN`, or to the `bin` directory of `go env GOPATH` when `GOBIN` is unset. Add that directory to your `PATH`. [GitHub Releases](https://github.com/romshark/datapages/releases) also has archives of the CLI for Linux, macOS and Windows on amd64 and arm64.

## Check the installation

```sh
datapages version
templ version
```

```text
0.10.1
v0.3.1070
```

A release of the CLI prints its version without the `v` prefix. A CLI built from source prints `(devel)`. `datapages version --full` adds the build information and the modules the binary was built with.

## Create a project

Run `datapages init` in the directory that should contain the project:

```sh
datapages init
```

`init` asks only what it needs to know:

| Prompt | Asked when | Answer |
| ------ | ---------- | ------ |
| `Not inside a git repository. Create one?` | The current directory is not in a git repository | Yes asks for a directory name, creates the directory in the current directory and runs `git init` in it. No ends `init`. |
| `Directory name` | After Yes | The project directory, such as `myapp` |
| `No go.mod found. Initialize a Go module?` | The project has no `go.mod` | Yes asks for the module path and runs `go mod init`. No ends `init`. |
| `Module path` | After Yes | The default comes from the URL of the `origin` remote, or else the directory name |
| `Set up Web Components?` | `init` writes the app package | `Datastar Rocket`, `TypeScript, bundled by esbuild (needs Node.js)`, `JavaScript modules` or `None`, the default. Choose one when the app needs JavaScript that would be impractical in Datastar attributes, such as a map or a rich text editor. |
| `Run the app now?` | At the end | Yes runs `make dev` in the project |

Inside a git repository, `init` uses the current directory, or the root of the Go module that contains it.

A new project prints:

```text
Initialized git repository in /home/you/myapp
Initialized Go module example.com/myapp
Created datapages.yaml
Created app/app.go
Created app/app.templ
Wrote .cursor/rules/datapages.mdc
Wrote .github/copilot-instructions.md
Wrote AGENTS.md
Wrote CLAUDE.md
Wrote GEMINI.md
Wrote 22 skill files in .claude/skills and .agents/skills
Created .env
Created compose.yaml
Created Makefile
Created .vscode/extensions.json
Created .github/workflows/ci.yml
Required github.com/romshark/datapages v0.10.1
Project initialized successfully.
```

Between the last 2 lines, `init` runs `templ generate`, `go mod tidy` and `datapages gen`. `go.mod` requires the Datapages release of the CLI.

### Create a project without prompts

`-n` asks nothing and takes the answers from flags:

```sh
datapages init -n --name myapp --module example.com/myapp
```

`--name` is required outside a git repository, and `--module` without a `go.mod`. With `-n`, `init` sets up no Web Components and does not start the app. More flags:

- `--web-components rocket|typescript|javascript|none` selects the Web Components setup.
- `--prometheus=false` writes a `cmd/server/main.go` without the Prometheus metrics server.
- `--no-ai-skills` writes no instructions for coding agents.

[CLI](cli.md#datapages-init) lists every flag and every step of `init`.

## What init writes

| File | Content |
| ---- | ------- |
| `app/app.go` | The application model: `App` with `Head` and `RecoverError`, `PageIndex`, `PageError404` and `PageError500` |
| `app/app.templ` | The welcome page |
| `cmd/server/main.go` | The server: NATS broker and session store, sessions, an access log, Prometheus metrics, TLS from `PATH_TLS_CERT` and `PATH_TLS_KEY`, and graceful shutdown |
| `datapages.yaml` | Settings of `datapages gen` and `datapages watch`, see [datapages.yaml](configuration.md) |
| `.env` | `NATS_URL` and a random `SESSION_ENCRYPTION_KEY`, which `cmd/server/main.go` reads. `init` adds `.env` to `.gitignore`. |
| `compose.yaml` | A NATS server with JetStream on port 4222 |
| `Makefile` | The targets `dev`, `up`, `down`, `build`, `gen` and `lint` |
| `.vscode/extensions.json` | The recommended editor extensions |
| `.github/workflows/ci.yml` | A CI job that fails when `templ generate` or `datapages gen` changes a committed file, then runs the tests and builds the server |
| `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, `.cursor/rules/datapages.mdc`, `.agents/skills/`, `.claude/skills/` | Instructions for coding agents, see [AI agents](ai-agents.md) |

With Web Components, `init` also writes an example component, in `web/` for TypeScript and in `app/static/components/` for the other setups.

The files in the table are yours to edit. `init` never deletes a file. A later run of `init` in the project rewrites only the agent instructions, and keeps a copy of each one you edited. It does not restore a deleted `Makefile` or workflow.

The generators own 2 more sets of files, which carry a `DO NOT EDIT` header. `templ generate` rewrites `app/app_templ.go` from `app/app.templ`, and `datapages gen` rewrites everything in `app/datapagesgen/`. Change the source and generate again instead of editing them.

## Run the project

```sh
cd myapp
make dev
```

`make dev` runs `docker compose up -d`, which starts NATS, and then `datapages watch`. Open <http://localhost:7331>, the address of the dev server.

The dev server forwards each request to the app at `localhost:8080`. When a file changes, it regenerates the code, rebuilds and restarts the app and reloads the open tabs. While a step fails, it answers with a page that shows the error. `datapages watch` prints no URL of its own. The address in the log of the app, `127.0.0.1:8080`, serves the app without the reload script.

Stop the dev server with Ctrl+C, and NATS with `make down`.

The app listens on `HOST` and `PORT`, `localhost:8080` by default, and serves Prometheus metrics at <http://localhost:9090/metrics>, set by `HOST_METRICS` and `PORT_METRICS`. When you change the port of the app, change [`watch.app-host`](configuration.md) in `datapages.yaml` to match.

### Run without Docker

For a single process, replace NATS with the in-memory broker and session manager. In `cmd/server/main.go`, replace the line `messageBroker, sessionManager := connectNATS()` with:

```go title="cmd/server/main.go"
messageBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
sessionManager := sessinmem.New[struct{}](sessions.DefaultTokenGenerator{
	Length: sessions.DefaultTokenLen,
})
```

Add these imports:

```go title="cmd/server/main.go"
"github.com/romshark/datapages/modules/messaging"
"github.com/romshark/datapages/modules/messaging/inmem"
sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
```

Delete the function `connectNATS` and the imports that only it uses, which are `encoding/hex`, `github.com/nats-io/nats.go`, `github.com/romshark/datapages/modules/messaging/natscore` and `github.com/romshark/datapages/modules/sessions/natskv`. Then start the dev server without `make dev`, which would start Docker:

```sh
datapages watch
```

The in-memory session manager loses every session when the process exits, which signs everyone out, and the in-memory broker reaches only the pages of its own process. [Deployment](deployment.md) describes the modules for production. To keep NATS without Docker, run a NATS server with JetStream at the `NATS_URL` of `.env` and start `datapages watch` the same way.

## Set up your editor

`.vscode/extensions.json` recommends 2 Visual Studio Code extensions, which the editor offers to install when it opens the project:

- `a-h.templ`, the templ extension, for highlighting, completion and diagnostics in `.templ` files
- `golang.go`, the Go extension

For other editors, templ's [IDE support](https://templ.guide/developer-tools/ide-support) page sets up the templ language server, `templ lsp`, in Neovim, JetBrains IDEs, Vim, Helix and Emacs. To see the errors of the application model and the templates in the editor, run `datapages lint` as an editor task.

## Next steps

Continue with the [tutorial](tutorial.md), which builds on the project that `init` created.

## See also

- [CLI](cli.md#datapages-init): every flag and step of `datapages init`, and the other commands
- [datapages.yaml](configuration.md): the settings of the dev server
- [AI agents](ai-agents.md): the files `init` writes for coding agents
- [Deployment](deployment.md): the broker and session modules for production
- [Upgrading](upgrading.md): moving the project to a newer release
