# Code Style

- Follow standard Go conventions (Effective Go).
- Use `require` from testify for test assertions, use `assert` only where it makes sense.
- Use table-driven map-based (to ensure random input ordering) tests where applicable
  with concise name tests as map keys.

# Commands

- Lint: `make lint`
- Format: `make fmt`
- Tidy all Go modules: `make mod-tidy`
- Test (runs lint first): `make test`
- Tidy parser testdata modules: `make mod-tidy-parser-tests`

# Project Structure

- `parser/` - main parser package, parses a Datapages application model from
  a Go source package.
- `parser/model/` - data model of a Datapages application.
- `parser/validate/` - naming convention validation.
- `parser/internal/` - internal utilities (e.g. route pattern parsing).
- `parser/testdata/` - each subdirectory is a self-contained Go module
  used as a test fixture. Prefix `err_` for expected-error cases.
- `example/counter/` - minimal counter example (separate module).
- `example/fancy-counter/` - polished counter with animations (separate module).
- `example/classifieds/` - full example application (separate module).
- `example/tailwindcss/` - minimal static page with Tailwind CSS (separate module).

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
