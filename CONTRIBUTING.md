# Contributing to Datapages

Contributions are welcome. Report bugs, propose large changes before writing them, and run the project checks before opening a pull request.

## Report a bug

[Open an issue](https://github.com/romshark/datapages/issues) with:

- A clear description of the problem.
- Steps to reproduce the issue.
- Expected and actual behavior.
- Go version and OS.
- Relevant error messages or logs.

## Propose a change

[Open an issue](https://github.com/romshark/datapages/issues) before writing a new feature or large change. The issue confirms the scope before you spend time on the implementation. Bug fixes and small improvements can go straight to a pull request.

## Get started

1. Fork the repository and clone your fork.
2. Create a feature branch from `main`.
3. Make your changes.
4. Run `mage test`, which runs linting, formatting checks and tests.
5. Commit and push your branch.
6. Open a pull request against `main`.

See [Development](#development) for setup and commands. See [AGENTS.md](AGENTS.md) for code style, test conventions and commit messages.

## AI

### How this project uses AI

The technical design and architecture of Datapages is authored **by human core contributors**. AI tools are used to write and review parts of the codebase and documentation, but not to make design decisions.

Core contributors review user-facing content and APIs before publication.

### Contribute with AI

You may use AI tools for code, tests, documentation and issue research. You don't need to disclose their use. You remain the author and are responsible for everything you submit.

Review and understand your change before opening a pull request.

Start with a problem you found or a change you propose. Explain it in your own words in the issue, pull request and review discussion.

We may close issues and pull requests that appear to contain unreviewed and unpolished AI output. We may hide comments that appear AI-generated. We do not accept issues or pull requests from automated bots.

## Development

### Prerequisites

- [Go](https://go.dev/dl/) (see version in `go.mod`)
- [Mage](https://magefile.org/) (or use `go run github.com/magefile/mage@latest`)
- A running Docker daemon. Some `mage test` tests use testcontainers to start NATS. They fail without Docker.

### Commands

```sh
mage test          # Lint + test with coverage
mage coverage      # Coverage of the generator and of the code it generates
mage lint          # Format check, module tidy check, datapages lint, golangci-lint
mage fmt           # Format all Go files (gofumpt + gci)
mage modTidy       # Tidy all go.mod files
mage lintDatapages # Run datapages lint on all examples
mage vulncheck     # Run govulncheck on all modules
mage build         # Build CLI and all examples
mage gen           # Generate all (templ + datapages + AI skills + docs)
mage genTempl      # Generate templ templates
mage genDatapages  # Generate datapages code for all examples
mage genAISkills   # Generate the AI agent instructions of every example
mage genDocs       # Generate documentation pages
mage watchDocs     # Serve the documentation pages and reload them on changes, see DEV.md
mage checkGen      # Check that all generated code is current
mage goFix         # Run go fix on all modules
mage all           # Run fmt, modTidy, genTempl, genDocs, test and vulncheck
```

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
