# Code Style

- Follow standard Go conventions (Effective Go).
- Use `require` from testify for test assertions, use `assert` only where it makes sense.
- Use table-driven map-based tests where applicable with concise name tests as map keys.

# Commands

- Lint: `make lint`
- Format: `make fmt`
- Tidy all Go modules: `make mod-tidy`
- Test (runs lint first): `make test`
- Tidy parser testdata modules: `make mod-tidy-parser-tests`

# Commits

Use conventional commits, prefix with `!` for breaking changes:
feat: fix: refactor: test: chore: ci: docs:
