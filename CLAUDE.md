# Code Style

- Lines must not exceed 90 columns.
- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective-go)).
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
- `example/classifieds/` - full example application (separate module).

# Commits

- Use conventional commits and prefix with `!` for breaking changes:
  - `feat:` - new feature
  - `fix:` - bug fix
  - `refactor:` - change of code without change of behavior
  - `test:` - testing related changes
  - `chore:` - chores
  - `ci:` - CI/CD related changes
  - `docs:` - documentation related changes.
