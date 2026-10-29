# AI agents

`datapages init` writes instructions and skills for coding agents. `datapages lint` and `datapages gen` report invalid code with source positions and suggested fixes. You can update, replace or omit the generated instructions.

## The files

| File | Content |
| ---- | ------- |
| `AGENTS.md` | The project layout, the build loop, the rules every change follows, a table of the skills and a link to the specification |
| `CLAUDE.md` | `@AGENTS.md`, which makes Claude Code read `AGENTS.md` |
| `GEMINI.md` | `Read and follow AGENTS.md in the repository root.` |
| `.github/copilot-instructions.md` | The same sentence, for GitHub Copilot |
| `.cursor/rules/datapages.mdc` | The same sentence, as a Cursor rule with `alwaysApply: true` |
| `.agents/skills/<name>/SKILL.md` | The 11 skills, at the paths that `AGENTS.md` names |
| `.claude/skills/<name>/SKILL.md` | The same 11 skills, where Claude Code discovers them |

`init` writes `AGENTS.md` for the project as it finds it. Its table of paths names each app package and the generated package below it, each server entry point, the directory of the Web Components when the project has one, and `datapages.yaml`. The rules forbid editing generated files, hardcoding app-internal URLs, writing JavaScript for application logic, opening SSE streams by hand and submitting forms without a Datastar action.

## The skills

Each skill is a Markdown file that starts with a `name` and a `description`. Claude Code picks a skill by its description. Other agents find the skills through the table in `AGENTS.md`, which says when to read each one:

| Skill | An agent reads it for |
| ----- | --------------------- |
| `datapages-architecture` | A new app, or a feature whose constructs are not decided. `AGENTS.md` tells agents to read it before they write code for one. |
| `datapages` | Any Datapages work: the rules, the build loop, naming, security and testing |
| `datapages-pages` | Pages, routes, path and query parameters, error pages and `<head>` |
| `datapages-actions` | GET file actions, POST, PUT, PATCH, DELETE and QUERY handlers, signals, SSE and errors |
| `datapages-events` | Events, subjects, dispatchers, `On` handlers and stream hooks |
| `datapages-state` | Per-tab server state and state-scoped events |
| `datapages-sessions` | Authentication, session data and CSRF |
| `datapages-offline` | Offline pages, cached shims and the service worker |
| `datapages-server` | Server entry points, options, the broker and static assets |
| `datapages-templates` | `.templ` files, the `href` and `action` helpers, Web Components and templ pitfalls |
| `datastar` | `data-*` attributes and the `@get` and `@post` actions of Datastar v1.0.4, which Datapages serves |

`datapages-architecture` decides where a value lives, how a change reaches the browser, which type owns a handler and how many server processes run. The other skills say how to write what it chose. `datapages-templates` sends agents to `datastar` for the attributes.

The `datapages` skill also tells agents not to run `templ generate` while `datapages watch` runs, and how to find a running `datapages watch` through its lock file. A `templ generate` deletes the template files that the dev server renders from, which stops it from rendering until the next `.templ` change.

## The specification link

The last line of `AGENTS.md` before its stamp links the specification of the release that wrote it:

```text
Full reference: https://github.com/romshark/datapages/blob/v0.10.1/SPECIFICATION.md
```

An agent then reads the rules of that release, which is the one `go.mod` requires, and not those of the main branch, which can describe declarations that the release rejects. A CLI built from source links the specification on the main branch. After an upgrade, `datapages init -n` moves the link to the new release.

## Feedback from the CLI

The build loop in `AGENTS.md` runs `datapages gen`, `templ generate` after a `.templ` change, `datapages lint` and `go build ./...`. The Datapages commands check the application model and the `.templ` files of the app package. They report each problem with its file, line and column. Most reports include a `fix:` line:

```text
at app.go:54:18: action handler is missing path comment: PageIndex.POSTSave
fix: Add `// POSTSave is /save`

at app.templ:57:7: template uses relative href: /about
fix: Use href={ href.PageAbout(...) } instead of "/about"

error: parsing app package: 2 error(s)
```

Both commands exit with status 1 on an error, which an agent can check. For an app with errors, `datapages gen` keeps the code it generated before, or writes empty stubs when there is none. `datapages lint` writes no file at all. [Linting](../SPECIFICATION.md#linting) lists the template checks, such as hardcoded links and form actions, and actions called from the template of another page.

## Update the files

Run `init` in the module root:

```sh
datapages init -n
```

In a project that exists, `init` rewrites the agent files and changes nothing else, apart from adding `.env` to `.gitignore` when it is missing there:

```text
Wrote AGENTS.md
Wrote 22 skill files in .claude/skills and .agents/skills
Kept the previous .claude/skills/datapages/SKILL.md as .claude/skills/datapages/SKILL.md.bak
Project already initialized.
```

Run it anywhere in the Go module that has the `datapages.yaml` of the project. Elsewhere `init -n` sets up a new project in the current directory, or fails with `--module is required in non-interactive mode when not inside a Go module`. With `--name` it creates a new project in a new directory. Without `-n`, `init` asks `Write AI agent instructions and skills?` before it writes the files.

`init` writes a file only when its content differs from the CLI's version. Every skill and `AGENTS.md` end with a stamp such as `<!-- written by datapages v0.10.1 sha256:9e363ad63c6a31ab -->`. The stamp records the release and a hash of the preceding content. `init` replaces a file whose content still matches the hash.

For an edited file, `init` first renames the old copy to `<name>.bak`, then `<name>.bak.1`, `<name>.bak.2` and later numbers. It prints `Kept the previous ...`. Merge your edits from the backup. `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md` and `.cursor/rules/datapages.mdc` have no stamp. `init` uses the same backup process when their content differs.

The skills exist 2 times, in `.agents/skills/` and `.claude/skills/`. Edit both copies of a skill, or agents that read the other directory miss your edit.

A release of the CLI notices instructions of another release. While a Datapages skill is missing or carries the stamp of another release, `datapages gen` prints:

```text
Agent instructions differ from this CLI; run `datapages init -n` to update them (edits are backed up).
```

Run `datapages init -n` again after you add an app package or Web Components, which updates the table of paths in `AGENTS.md`.

## Omit the files

Answer No to `Write AI agent instructions and skills?` when you create the project, or pass `--no-ai-skills`:

```sh
datapages init --no-ai-skills
```

`init` then writes no agent file. In a project that has them, No and `--no-ai-skills` leave them as they are. Delete the files to remove them. Every later `init` run asks again, and `init -n` writes them again unless `--no-ai-skills` is set. `datapages gen` does not print its notice in a project without a `datapages` skill in either directory.

## Keep your own AGENTS.md

An `AGENTS.md` that does not end with the Datapages stamp is yours. `init` leaves it alone and prints:

```text
Left AGENTS.md alone: it has no datapages stamp. Delete it to have init write it.
```

Text that you add after the stamp line makes the generated file yours the same way, and later releases no longer update it. Copy the skill table and the specification link of the generated file into your own, or agents that read only `AGENTS.md` do not find the skills.

To add instructions and keep the generated `AGENTS.md`, write a skill of your own with another name, such as `.agents/skills/shop/SKILL.md`. `init` does not touch skills whose names it does not write.

## See also

- [Installation](installation.md): the files `datapages init` writes besides these
- [Upgrading](upgrading.md): when to run `datapages init -n`
- [CLI](cli.md#datapages-init): every flag and step of `datapages init`
- [Linting](../SPECIFICATION.md#linting): the template checks of `datapages gen` and `datapages lint`
- [Application model](application-model.md): what the CLI reads from the app package
