# Code Style

- Follow standard Go conventions (Effective Go).
- Use `require` from testify for test assertions, use `assert` only where it makes sense.
- Use table-driven map-based (to ensure random input ordering) tests where applicable
  with concise name tests as map keys.

# Commands

- Lint: `mage lint`
- Format: `mage fmt`
- Tidy all Go modules: `mage modTidy`
- Test (runs lint first): `mage test`
- Build CLI and examples: `mage build`
- Generate templ files: `mage genTempl`
- Generate datapages code: `mage genDatapages`
- Generate all (templ + datapages + docs): `mage gen`
- Run go fix on all modules: `mage goFix`
- Run everything: `mage all`

# Project Structure

- `internal/parser/` - main parser package, parses a Datapages application model
  from a Go source package.
- `internal/parser/model/` - data model of a Datapages application.
- `internal/parser/validate/` - naming convention validation.
- `internal/parser/internal/` - internal utilities (e.g. route pattern parsing).
- `internal/parser/testdata/` - each subdirectory is a self-contained Go module
  used as a test fixture. Prefix `err_` for expected-error cases.
- `example/calculator/` - basic calculator with server-side evaluation (separate module).
- `example/counter/` - minimal counter example (separate module).
- `example/fancy-counter/` - polished counter with animations (separate module).
- `example/classifieds/` - full example application (separate module).
- `example/tailwindcss/` - minimal static page with Tailwind CSS (separate module).
- `example/webcomponents/` - landing page with vanilla and Lit Web Components bundled via esbuild.
- `example/sqlitesessions/` - custom sessmanager.SessionManager backed by SQLite via sqinn-go.
- root package `datapages` (`datapages.go`) - core handler-parameter types
  (`datapages.SSE`); the CLI entrypoint lives in `cmd/datapages/`.
- `internal/generator/` - code generation from parsed model.
- `internal/cmd/` - CLI command implementations.
- `internal/tools/render-pages/` - build-time tool rendering `docs/index.html`.
- `internal/docs-src/` - templ source and CSS for the project's docs page.
- `docs/` - generated GitHub Pages output (committed).
- `magefiles/` - build targets (mage).

Only packages that generated code or application code imports may live outside
`internal/`. Everything the CLI and build tooling use (parser, generator,
render-pages, docs-src) is internal. `cmd/` is reserved for the shipped binary,
so build-time tools go under `internal/tools/`, NOT `cmd/` (which would make them
`go install`-able by users) and NOT `internal/cmd/` (which is the datapages CLI
implementation, `package cmd`, plus its own subpackages such as `config`).
The non-internal packages besides the root are:

- `modules/` - pluggable modules (csrf, msgbroker, sessmanager, sesstokgen),
  imported by application code.
- `hrefcheck/` - imported by generated `href` packages.

These cannot be moved into `internal/`: generated code lives in the user's own
module, which may not import `github.com/romshark/datapages/internal/...`. Note the
`example/*` modules would NOT catch such a mistake, since their paths sit under the
`github.com/romshark/datapages/` prefix and so satisfy the internal-import rule.

Runtime support is generated into `datapagesgen` rather than imported, so it needs
no package of its own and stays out of the public API:

- `writeSSEWrapper` emits the `datapages.SSE` implementation (`newSSE`/`sseWrapper`).

Prefer this over adding a public runtime package: it also removes version skew
between the generator and a separately pinned runtime dependency.

# Datapages Framework

When working with Datapages application code, read and follow these files:

- `.skills/datapages/SKILL.md` — step-by-step guide for writing Datapages apps and using
  the CLI.
- `.skills/datastar/SKILL.md` — Datastar HTML attribute and action reference for
  templates.
- `SPECIFICATION.md` — full parameter, return type, and configuration reference.

# Commits

- Keep the commit title to 50 characters or less.
- Wrap the commit description at 72 characters.
- Use conventional commits and prefix with `!` for breaking changes:
  - `feat:` - new feature
  - `fix:` - bug fix
  - `refactor:` - change of code without change of behavior
  - `test:` - testing related changes
  - `chore:` - chores
  - `ci:` - CI/CD related changes
  - `docs:` - documentation related changes.
