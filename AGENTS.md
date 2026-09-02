# Code Style

- Follow standard Go conventions (Effective Go).
- Comment as described in [code comments](#code-comments).
- Use `require` from testify for test assertions, use `assert` only where it makes sense.
- Use table-driven map-based (to ensure random input ordering) tests where applicable
  with concise name tests as map keys.

# Commands

- Lint: `mage lint`
- Format: `mage fmt`
- Tidy all Go modules: `mage modTidy`
- Test (runs lint first): `mage test`
- Coverage of the generator and of the code it generates: `mage coverage`
- Build CLI and examples: `mage build`
- Generate templ files: `mage genTempl`
- Generate datapages code: `mage genDatapages`
- Generate all (templ + datapages + docs): `mage gen`
- Check that all generated code is current: `mage checkGen`
- Run go fix on all modules: `mage goFix`
- Run everything: `mage all`

# Project Structure

Internal, used by the CLI and build tooling:

- `internal/parser/` - parses the application model from a Go source package.
  - `model/` - the data model.
  - `validate/` - naming convention validation.
  - `errsuggest/` - "did you mean" suggestions for parser errors.
  - `internal/` - method kinds, parameter validation, struct inspection, templ
    linting, type predicates, URL paths.
  - `testdata/` - one self-contained module per fixture. Prefix `err_` for
    expected-error cases.
- `internal/generator/` - code generation from the parsed model.
  `internal/generator/README.md` explains its tests.
- `internal/acceptance/` - one module per case: the application, its committed
  generated code and tests asserting behaviour over HTTP.
  `internal/acceptance/README.md` explains how they run and how to record a
  defect the framework has not fixed yet.
- `internal/gotypes/` - go/types predicates and renderings that know nothing
  about Datapages. Shared by the parser and the generator.
- `internal/routepattern/` - net/http ServeMux route pattern parsing.
- `internal/structtag/` - the `path`, `query`, `json` and `reflectsignal` tags.
- `internal/subject/` - broker subject rules: the token rule and which subjects
  an event claims.
- `internal/serverscan/` - finds the app package and the target package from the
  `datapages.NewServer` calls. AST only, so it reads a `main.go` naming a
  package the first run has not written yet.
- `internal/templatingbench/` - the templating benchmarks `FAQ.md` quotes.
- `internal/cmd/` - CLI command implementations, `package cmd`.
- `internal/tools/render-pages/` - renders `docs/index.html`.
- `internal/docs-src/` - templ source and CSS for the docs page.
- `docs/` - generated GitHub Pages output.
- `magefiles/` - build targets.

Public, imported by application or generated code:

- root package `datapages` (`datapages.go`) - the handler types
  (`SSE`, `Session`, `NewSession`, `Redirect`, `Component`) and the HTTP error
  sentinels. `options.go` holds every server option: `ServerOption` values fill
  a `ServerConfig` that `httpserve.NewCore` reads. None are generated.
- `cmd/datapages/` - the shipped binary.
- `modules/` - pluggable modules: csrf, messaging, sessions.
- `runtime/` - what generated code imports:
  - `httpserve` - the server core a generated one embeds: listener, routes,
    middleware chain, logger, shutdown, redirect, Datastar request check,
    dev-mode cache headers, the assets file system and the HTML document
    writer.
  - `auth` - session cookie, the record behind it and the CSRF check. Generic
    over the application's session data.
  - `sse` - implements `datapages.SSE` on the Datastar generator, which keeps
    datastar out of handler signatures.
  - `stream` - serves the SSE event stream of a page: the broker subscription
    behind it, the session that may end it and the shutdown that closes it.
  - `httpread` - reads cookies and query parameters the way `net/http` and
    `net/url` do, without their allocations. Fuzzed against them.
  - `htmlattr` - escapes values written into Datastar attributes. Fuzzed for
    anything that could end the attribute or the script.
  - `subject` - the escaping a value gets on its way into a subject,
    shared by the parser and the dispatchers.
  - `prom` - Prometheus metrics, registration and middleware.
  - `hrefcheck` - imported by generated `href` packages.
  - `actionexpr` - builds the Datastar action expression: the call, its
    options and the JavaScript around it. The generated `action` package
    forwards to it.

Examples, one module each:

- `example/calculator/` - server-side evaluation.
- `example/counter/` - the same counter twice in one module, `app/simple` and
  `app/fancy`, one entry point each. The multi-application example.
- `example/todolist/` - collaborative todo list with per-tab server-side state
  registered in `StreamOpen` and read by the event handlers.
- `example/classifieds/` - the full application.
- `example/tailwindcss/` - static page with Tailwind CSS.
- `example/webcomponents/` - vanilla and Lit Web Components bundled via esbuild.
- `example/sqlitesessions/` - a `sessions.Manager` on SQLite via
  sqinn-go.

Where code goes:

- Only what generated or application code imports may live outside `internal/`.
  `runtime/` and `modules/` therefore cannot move into `internal/`: generated
  code lives in the user's module, which may not import
  `github.com/romshark/datapages/internal/...`. The `example/*` modules would
  not catch such a mistake, since their paths sit under the
  `github.com/romshark/datapages/` prefix.
- Build-time tools go in `internal/tools/`, not `cmd/`, which users could
  `go install`, and not `internal/cmd/`, which is the CLI implementation.
- What the application model shapes is generated into `datapagesgen` under the
  app package: the dispatchers, the handlers, the subject lists and
  `setupHandlers`. This keeps it out of the public API and free of version skew
  between the generator and a separately pinned dependency.
- What mirrors the standard library lives in `runtime/` and is imported, since
  its correctness follows the Go version rather than the application and it can
  be tested against the standard library directly.
- `datapages.NewServer` is generic over the app type, the session data type and
  the generated `Server`. Generated code contributes the `Init` method
  satisfying `datapages.ServerInitializer`, which is where `httpserve.NewCore` is called:
  the root package cannot import `runtime/`, since `runtime/httpserve` imports
  the root package.
- An option that needs per-app constants carries only what the caller has and
  lets `Init` apply the rest: `WithAssets` carries the `embed.FS`, the
  generated code supplies the subdirectory, the URL prefix and the dev-mode
  path from the app package.

# Dependencies

Prefer the standard library. A new third-party dependency needs a reason that
survives the question: what does it buy over the stdlib plus a few lines here?

- Anything the root package, `runtime/` or `modules/` imports lands in every
  user's module and `go.sum`. Judge those hardest. CLI-only dependencies
  (`cobra`, `huh`, `templier`) never reach a user's application.
- Propose a new direct dependency, don't add it unannounced. Name what it
  replaces and what it pulls in transitively.
- Don't reach for a dependency for what the stdlib already does. `net/http`,
  `text/template`, `log/slog`, `encoding/json` and `go/types` cover most of it.
- Tests use `testify`. Don't add another assertion, mocking or fixture library.
- `example/*` and `internal/acceptance/*` each carry their own `go.mod`. Don't
  add a dependency to one unless the case exists to exercise that dependency.
- After any change to a `go.mod`: `mage modTidy`, then `mage test`.
  `mage vulncheck` runs govulncheck, `mage modUpdate` bumps versions.

# Generated Code

Never edit a generated file. Change the source or the generator, then regenerate.
Generated output is committed, and tests fail when it goes stale.

- `*_templ.go`: written by templ from the `*.templ` next to it.
  `mage genTempl`, not `templ generate` directly.
- `*/datapagesgen/**` in examples and acceptance cases: written by the CLI from
  the app package. `mage genDatapages` builds `cmd/datapages` from source and
  runs `datapages gen` in every example and acceptance module.
- `docs/index.html`: written by `internal/tools/render-pages` from
  `internal/docs-src/`. `mage genDocs`.
- `mage gen` runs all three.

Any change to the generator requires `mage genDatapages` in the same commit.
`TestExamplesAreUpToDate` (`internal/generator/generator_test.go:45`) and the
acceptance tests regenerate and diff against the committed output, and report
`run: mage genDatapages` when it differs, is missing or is no longer generated.

# Datapages Framework

When working with Datapages application code, read and follow these files:

- `.skills/datapages/SKILL.md`: step-by-step guide for writing Datapages apps and using
  the CLI.
- `.skills/datastar/SKILL.md`: Datastar HTML attribute and action reference for
  templates.
- `SPECIFICATION.md`: full parameter, return type, and configuration reference.

# Writing

Applies to chat replies, code comments, documentation and commit messages.
Write like an engineer reporting findings to another engineer:
plain, specific, with facts, non-verbose.

Styles:

- BLUF (bottom line up front), US military staff writing.
- Inverted pyramid, newswire reporting.
- Plain English: Gowers' `Plain Words`, Cutts' `Oxford Guide to Plain English`.
- Orwell's six rules from `Politics and the English Language`.
- Strunk & White: omit needless words, use the active voice.
- IMRaD Results sections: every claim carries a measurement or a citation.
- SRE postmortems: timeline, root cause, action items, no blame, no drama.
- Aviation and maritime logs: one fact per line, interpretation kept separate.
- Unix man pages and RFCs: terse, imperative, everything named exactly.

Rules:

- Prefer simple (near primitive) technical English.
- Lead with the conclusion, then the facts that support it.
- Name concrete things: files, lines, symbols, values, error codes.
  Write `internal/parser/parser.go:142`, not "the place where the parser reads it".
- End with what to do about it, or say plainly that nothing needs doing.
- Don't paraphrase code changes, the diff already shows them. Name the file and
  what it does now. Prose is for the non-obvious what the diff can't show:
  why, what was left out, what could break.
- Say what was verified and how: "`mage test` passes", "not run".
  Never imply a check that didn't happen.
- Report failures, dead ends and skipped work as plainly as successes.
- Answer at the length the question needs. "Is `Cache.Get` safe for
  concurrent use?" is answered by "No, it writes `c.entries` without
  holding `c.mu`." and nothing else. Don't pad a short answer out to look
  thorough, and don't compress a real explanation into three bullets.
- Stop when the information is delivered.
  No preamble, no restating the request, no closing summary of what was just said.

Avoid:

- Suspense and buildup: "here's where it gets interesting", "and this is the
  kicker", "the third one is the most instructive".
- Hype and intensifiers: "not just X, it's THE Y", "load-bearing", "crucial",
  "powerful", "seamless", "robust", "comprehensive", "deep dive".
- Counting the items instead of naming them. "Two things: X and Y" is "X and Y".
  A teaser count with no items after it ("three things jumped out at me") is
  worse.
- Figurative verbs where a plain one fits: "buys", "drives", "unlocks",
  "wins", "kills", "shines". Write "a value receiver prevents mutation",
  not "a value receiver buys us immutability".
  Write "the pointer saves no allocation here",
  not "the pointer buys nothing here".
  Write "the tests that send it requests over HTTP",
  not "the tests that drive it over HTTP".
- Rhetorical questions as headings: "So what does this mean?".
- Filler transitions: "let's dive in", "at the end of the day", "it's worth
  noting that", "as we can see".
- Praise of the user or of the question: "great question", "you're absolutely
  right".
- Typographic drama: spaced-out words, all-caps emphasis, exclamation marks,
  emoji, bold scattered over half the sentences.
- Em-dashes and ", so ..." clauses. Use a colon, a full stop or "which ...".
- Non-ASCII characters where ASCII exists: curly quotes, ellipsis, arrows,
  non-breaking spaces. Write `'`, `"`, `...`, `->` and a plain space.
- Hedging where a check would settle it. Check, then state the answer.
- Apologies and post-mortems after a mistake. Correct it and continue.

Bad:

> Here's where it gets interesting: the retry logic isn't just a nice-to-have,
> it's the load-bearing assumption of the entire sync pipeline. Three things
> jumped out at me, and the third one is the most instructive yet. [...] And
> third, and this is the kicker, the dedupe key includes a timestamp, which
> means retries are never actually deduplicated.

Good:

> The sync pipeline's retry logic has three bugs. `syncQueue.ts:142` swallows
> `ETIMEDOUT` instead of re-queuing the job. `backoff.ts:31` caps the delay at
> 2s, under the 8s p99 LTE reconnect time in `bench/network.json`.
> `dedupe.ts:77` puts a timestamp in the key, which means retries never
> deduplicate. All three reproduce in `syncQueue_test.ts` against the network
> stub. Fix: re-throw the timeout, raise the cap to 30s and strip the timestamp
> from the key.

## Code comments

Write a comment only if it adds value to the reader by providing non-obvious context
information that the reader cannot easily infer from the code itself.

Don't write:

- Comments that repeat the next line: `// increment i` above `i++`,
  `// loop over users`, `// return the result`, `// error handling`.
- Doc comments that only spell out the name: `// UserID is the user ID.`
- Banners and dividers: `// --- helpers ---`, `// BEGIN`, `// END`.
- History: `// added in v2`, `// fixed #142`, `// was int64 before`.
  All history is in Git. Comments must only explain current code at hand,
  unless history is crucial context, e.g. when the code makes no sense without it.
- Commented-out code. Delete it, git has it.
- Comments written for the reviewer of the diff instead of the reader of the
  code: `// Note: now also handles nil`, `// Changed to a map for speed`,
  `// For simplicity we just skip this`.
- Comments that teach Go or the standard library: `// mu guards concurrent
  access` above a plain mutex, `// defer closes the file`, `// ok is false when
  the key is missing`. Assume that the reader is a seasoned software engineer
  with good understanding of Go.
- File and line references: `// see decoder.go:212`. They break as soon as the
  code moves and nothing checks them. Name the symbol with a doc link instead:
  `[Decoder.Next]`.

Do write:

- Why the code works this way, when that isn't clear from reading it:
  why this order, this algorithm, this lock, what breaks without it.
- Where a value comes from: a constant, a timeout, a buffer size. Name the spec
  section, benchmark, RFC or issue and link it.
- Rules the types can't state: what the caller must guarantee,
  what the function assumes.
- Why the obvious approach wasn't used: name it and say what went wrong with it.
- `TODO: ...` with what to do and what unblocks it, not a bare `// TODO`.

Form:

- Respect [writing](#writing).
- Plain sentences, present tense, within the line limit.
- Go doc comments on exported names start with the name. This is the one place
  where repeating the name is required.
- Put the comment above the code, not after it. Comments after the code are for
  short notes on struct fields, enum values and table-test rows.
- Reference other code with Go doc links in square brackets: `[Scan]`,
  `[Parser.Parse]`, `[github.com/romshark/datapages.Subject]`.
- A doc comment on a symbol says what the symbol does:
  `// Close ends the open requests and is safe to call repeatedly.`
- A doc comment on a test explains what scenario is tested:
  `// TestCloseIdempotent tests that repeated Close calls all return nil.`

Bad:

```go
// Timeout for the request.
const timeout = 8 * time.Second

// Loop over the items and check each one.
for _, it := range items {
	// Skip if nil.
	if it == nil {
		continue // skip
	}
}
```

Good:

```go
// timeout covers the p99 LTE reconnect time measured in bench/network.json.
// Below 8s the sync queue retries before the radio is back up.
const timeout = 8 * time.Second

for _, it := range items {
	// [Decoder.Next] emits nil for unknown tags.
	if it == nil {
		continue
	}
}
```

# Git Commits

- Respect [writing](#writing).
- Title: `type: Summary`, imperative, capitalized after the prefix, 50
  characters or less, no trailing period.
- Types: `feat`, `fix`, `perf`, `refactor`, `test`, `chore`, `ci`, `docs`.
  Suffix the type with `!` for a breaking change.
- Wrap the description at 72 characters.
- Imperative mood in the title, never past tense: `Add cache`, not
  `Added cache` or `Adds cache`. The description uses present tense and
  describes the code as it is now: `Move foo to package buzz`, not
  `foo is now in package buzz`. No first person, no `we`.
- Don't paraphrase the code changes, the diff already shows them.
  The description is for what the diff can't show: why, scope, what was left out.
  Leave it empty when the title says everything.
- Don't list the files touched, don't restate the title in longer words and
  don't close with a summary of the commit.
- A commit that bundles several changes lists them as bullets, each with its
  own conventional prefix. List only the main changes. Tests, docs and
  call-site updates that come with a change are part of it, not separate bullets:

  ```
  feat: Add funcFoo <- keep, this is the change
  docs: Add docs for funcFoo <- drop
  test: Add tests for funcFoo <- drop
  refactor: Use funcFoo everywhere <- drop
  test: Add benchmark to test funcFoo against funcBar <- drop
  ```
- A breaking change ends with a `BREAKING:` block. It is the migration
  instruction: name every removed or renamed symbol as `old -> new`, and say
  what the caller has to do, one step per line. A caller must be able to
  migrate from this block alone, without reading the diff. Say plainly when a
  step is automatic (`datapages gen` regenerates it) and when it is manual.
- A `perf:` commit quotes the measurement as `before -> after`.
- No tool attribution or `Co-Authored-By` trailers.

Bad:

```
feat: added caching to the generator and updated some files

This commit adds a cache to the generator so that it is faster. I changed
internal/gen/gen.go, internal/gen/cache.go and internal/cmd/gen.go to add the
cache and wire it up. The cache stores parsed packages in a map keyed by import
path. Overall this makes the generator faster and cleaner.

Co-Authored-By: Some Tool <tool@example.com>
```

Good:

```
perf: Cache parsed packages in the generator

Every `datapages gen` run reparsed all dependencies. The cache keys on
import path and file mod time, which is safe because the loader already
rejects stale build artifacts.

Watch mode still reparses on every event: invalidation for edits inside
the module isn't implemented yet.

40ms -> 12ms on example/classifieds.
```

Good, breaking change:

```
refactor!: Move HTTP core to runtime packages

BREAKING:
- `datapages.NewCore` -> `runtime/httpserve.NewCore`.
- `datapages.CoreConfig` -> `runtime/httpserve.Config`.
- Generated code: run `datapages gen`, no hand edits needed.
- Hand-written callers: import `github.com/romshark/datapages/runtime/httpserve`
  and update the two names above.
```
