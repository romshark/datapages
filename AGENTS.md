# Code Style

- Follow standard Go conventions (Effective Go).
- Comment as described in [code comments](#code-comments).
- Use `require` from testify for test assertions, use `assert` only where it makes sense.
- Use table-driven map-based (to ensure random input ordering) tests where applicable with concise name tests as map keys.

# Commands

- `mage lint`: lint.
- `mage fmt`: format.
- `mage modTidy`: tidy every Go module.
- `mage test`: lint, then test.
- `mage coverage`: measure the generator and generated-code coverage.
- `mage build`: build the CLI and examples.
- `mage genTempl`: generate templ files.
- `mage genDatapages`: generate Datapages code.
- `mage genAISkills`: generate AI agent instructions for every example.
- `mage genOfflineWorker`: minify the offline service worker.
- `mage gen`: generate templ files, Datapages code, AI agent instructions, documentation and the offline service worker.
- `mage checkGen`: check that generated code is current.
- `mage checkWatch`: report active `datapages watch` processes and fail if any exist.
- `mage goFix`: run `go fix` on every module.
- `mage all`: run `fmt`, `modTidy`, `genTempl`, `genDocs`, `test` and `vulncheck`.

Do not run `templ generate` or any target that wraps it (`genTempl`, `genDocs`, `gen`, `checkGen`, `all`) while `datapages watch` is running. Watch mode serves application template strings from temporary files. A separate generation deletes those files when it exits, which prevents the server from rendering until the next `.templ` change.

`datapages watch` stores one lock file per module in the system temporary directory under `datapages-watch` and refreshes it every 2s. `genTempl` and `genDocs` run `mage checkWatch` before generating. SIGKILL prevents lock cleanup, but a stale lock expires 10s after its last write. `TestWatch` skips while a lock is held. See the `datapages` skill for recovery instructions.

# Project Structure

Internal, used by the CLI and build tooling:

- `internal/parser/` - parses the application model from a Go source package.
  - `model/` - the data model.
  - `validate/` - naming convention validation.
  - `errsuggest/` - "did you mean" suggestions for parser errors.
  - `internal/` - method kinds, parameter validation, struct inspection, templ linting, type predicates, URL paths.
  - `testdata/` - one self-contained module per fixture. Prefix `err_` for expected-error cases.
- `internal/generator/` - code generation from the parsed model. `internal/generator/README.md` explains its tests.
- `internal/acceptance/` - one module per case: the application, its committed generated code and tests asserting behaviour over HTTP. `internal/acceptance/README.md` explains how they run and how to record a defect the framework has not fixed yet.
- `internal/gotypes/` - go/types predicates and renderings that know nothing about Datapages. Shared by the parser and the generator.
- `internal/logsample/` - the throttling `slog.Handler` behind `datapages.WithLogSampling`.
- `internal/routepattern/` - net/http ServeMux route pattern parsing.
- `internal/structtag/` - the `path`, `query`, `json` and `reflectsignal` tags.
- `internal/subject/` - broker subject rules: the token rule and which subjects an event claims.
- `internal/serverscan/` - finds the app package and the target package from the `datapages.NewServer` calls. AST only, so it reads a `main.go` naming a package the first run has not written yet.
- `internal/templatingbench/` - the templating benchmarks `FAQ.md` quotes.
- `internal/cmd/` - CLI command implementations, `package cmd`.
- `internal/tools/render-pages/` - renders `docs/index.html`.
- `internal/tools/release-notes/` - prints the section of `CHANGELOG.md` that the release workflow publishes for a tag.
- `internal/docs-src/` - templ source and CSS for the docs page.
- `docs/` - generated GitHub Pages output.
- `magefiles/` - build targets.

Public, imported by application or generated code:

- root package `datapages` (`datapages.go`) - the handler types (`SSE`, `Session`, `NewSession`, `Redirect`, `Component`) and the HTTP error sentinels. `options.go` holds every server option: `ServerOption` values fill a `ServerConfig` that `httpserve.NewCore` reads. None are generated.
- `cmd/datapages/` - the shipped binary.
- `modules/` - pluggable modules: csrf, messaging, offline, sessions.
- `runtime/` - what generated code imports:
  - `httpserve` - the server core a generated one embeds: listener, routes, middleware chain, logger, shutdown, redirect, Datastar request check, dev-mode cache headers, the assets file system and the HTML document writer.
  - `auth` - session cookie, the record behind it and the CSRF check. Generic over the application's session data.
  - `sse` - implements `datapages.SSE` on the Datastar generator, which keeps datastar out of handler signatures.
  - `stream` - serves the SSE event stream of a page: the broker subscription behind it, the session that may end it and the shutdown that closes it.
  - `httpread` - reads cookies and query parameters the way `net/http` and `net/url` do, without their allocations. Fuzzed against them.
  - `htmlattr` - escapes values written into Datastar attributes. Fuzzed for anything that could end the attribute or the script.
  - `subject` - the escaping a value gets on its way into a subject, shared by the parser and the dispatchers.
  - `prom` - Prometheus metrics, registration and middleware.
  - `hrefcheck` - imported by generated `href` packages.
  - `actionexpr` - builds the Datastar action expression: the call, its options and the JavaScript around it. The generated `action` package forwards to it.

Examples, one module each:

- `example/calculator/` - server-side evaluation.
- `example/counter/` - the same counter twice in one module, `app/simple` and `app/fancy`, one entry point each. The multi-application example.
- `example/todolist/` - collaborative todo list with per-tab server-side state registered in `StreamOpen` and read by the event handlers.
- `example/classifieds/` - the full application.
- `example/tailwindcss/` - static page with Tailwind CSS.
- `example/webcomponents/` - vanilla and Lit Web Components bundled via esbuild.
- `example/sqlitesessions/` - a `sessions.Manager` on SQLite via sqinn-go.
- `example/fast-shim/` - instant loads: cached shims morphed by Datastar.
- `example/offline-cache/` - service-worker offline support, handler-written cache and a `PageOffline` fallback, on a ticketing app.
- `example/file-upload/` - chunked, pausable and resumable uploads stored on disk, downloaded through middleware below the asset prefix, both directions paced by a server-side rate limit.

Where code goes:

- Only what generated or application code imports may live outside `internal/`. `runtime/` and `modules/` therefore cannot move into `internal/`: generated code lives in the user's module, which may not import `github.com/romshark/datapages/internal/...`. The `example/*` modules would not catch such a mistake, since their paths sit under the `github.com/romshark/datapages/` prefix.
- Build-time tools go in `internal/tools/`, not `cmd/`, which users could `go install`, and not `internal/cmd/`, which is the CLI implementation.
- What the application model shapes is generated into `datapagesgen` under the app package: the dispatchers, the handlers, the subject lists and `setupHandlers`. This keeps it out of the public API and free of version skew between the generator and a separately pinned dependency.
- What mirrors the standard library lives in `runtime/` and is imported, since its correctness follows the Go version rather than the application and it can be tested against the standard library directly.
- The page cache is generated, not imported: `writePageCache` emits the `datapages.PageCacheWriter` implementation (`newPageCache`/`pageCacheWriter`) and its delivery lifecycle (`flush`, `embedInto`, `redirectScript`). It renders cached bodies through the application's own `writeHTML`, so it is shaped by the app model, not by the Go version.
- `datapages.NewServer` is generic over the app type, the session data type and the generated `Server`. Generated code contributes the `Init` method satisfying `datapages.ServerInitializer`, which is where `httpserve.NewCore` is called: the root package cannot import `runtime/`, since `runtime/httpserve` imports the root package.
- An option that needs per-app constants carries only what the caller has and lets `Init` apply the rest: `WithAssets` carries the `embed.FS`, the generated code supplies the subdirectory, the URL prefix and the dev-mode path from the app package.

# Dependencies

Prefer the standard library. Propose a new direct third-party dependency before adding it. State what it replaces, why the standard library plus local code is insufficient and which transitive dependencies it adds.

- Scrutinize imports in the root package, `runtime/` and `modules/`. They enter every user's module and `go.sum`. CLI-only dependencies such as `cobra`, `huh` and `templier` do not enter application modules.
- Do not add a dependency for behavior already provided by `net/http`, `text/template`, `log/slog`, `encoding/json`, `go/types` or another standard library package.
- Use `testify` for tests. Do not add another assertion, mocking or fixture library.
- Each `example/*` and `internal/acceptance/*` directory has its own `go.mod`. Add a dependency only when the case exercises it.
- After changing any `go.mod`, run `mage modTidy`, then `mage test`.
- `mage vulncheck` runs `govulncheck`. `mage modUpdate` updates dependency versions.

# Generated Code

Never edit a generated file. Change the source or the generator, then regenerate. Generated output is committed, and tests fail when it goes stale.

- `*_templ.go`: written by templ from the `*.templ` next to it. `mage genTempl`, not `templ generate` directly.
- `*/datapagesgen/**` in examples and acceptance cases: written by the CLI from the app package. `mage genDatapages` builds `cmd/datapages` from source and runs `datapages gen` in every example and acceptance module.
- In every `example/*` module: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, `.cursor/rules/datapages.mdc` and everything under `.agents/skills/` and `.claude/skills/`. Written by `datapages init` from `internal/generator/agentdocs/data`. `mage genAISkills` runs it in every example to catch drift.
- `docs/index.html`: written by `internal/tools/render-pages` from `internal/docs-src/`. `mage genDocs`.
- `modules/offline/sw.min.js`: minified from `sw.js` next to it. `mage genOfflineWorker`. The module embeds the minified file, which is why it is committed: `go build` cannot run the target.
- `mage gen` runs all of them.

Any change to the generator requires `mage genDatapages` in the same commit. `TestExamplesAreUpToDate` (`internal/generator/generator_test.go:45`) and the acceptance tests regenerate and diff against the committed output, and report `run: mage genDatapages` when it differs, is missing or is no longer generated.

# Datapages Framework

When working with Datapages application code, read and follow these files:

- `internal/generator/agentdocs/data/skills/datapages/SKILL.md`: guide for writing Datapages apps and using the CLI.
- `internal/generator/agentdocs/data/skills/datastar/SKILL.md`: Datastar HTML attribute and action reference for templates.
- `SPECIFICATION.md`: full parameter, return type, and configuration reference.

# Writing

Applies to chat replies, code comments, documentation and commit messages. Write like an engineer reporting findings to another engineer: plain, specific, with facts, non-verbose.

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
- Name concrete things: files, lines, symbols, values, error codes. Write `internal/parser/parser.go:142`, not "the place where the parser reads it".
- End with what to do about it, or say plainly that nothing needs doing.
- Don't paraphrase code changes, the diff already shows them. Name the file and what it does now. Prose is for the non-obvious what the diff can't show: why, what was left out, what could break.
- Say what was verified and how: "`mage test` passes", "not run". Never imply a check that didn't happen.
- Report failures, dead ends and skipped work as plainly as successes.
- Answer at the length the question needs. "Is `Cache.Get` safe for concurrent use?" is answered by "No, it writes `c.entries` without holding `c.mu`." and nothing else. Don't pad a short answer out to look thorough, and don't compress a real explanation into three bullets.
- Stop when the information is delivered. No preamble, no restating the request, no closing summary of what was just said.

Avoid:

- Suspense and buildup: "here's where it gets interesting", "and this is the kicker", "the third one is the most instructive".
- Hype and intensifiers: "not just X, it's THE Y", "load-bearing", "crucial", "powerful", "seamless", "robust", "comprehensive", "deep dive".
- Counting the items instead of naming them. "Two things: X and Y" is "X and Y". "One dispatch, three streams: A, B and C" is "A, B and C". A teaser count with no items after it ("three things jumped out at me") is worse.
- A verbless lead-in before a colon: "One dispatch, three streams: ...", "Two values, one subject: ...". In running text, the words before a colon must stand alone as a sentence. Labels such as `TODO:` and `BREAKING:`, and a line that introduces a code block or a list, are exempt.
- Figurative language where a plain word fits: "buys", "drives", "unlocks", "wins", "kills", "shines", "leaves the reader hunting". These are examples, not the whole set. The test is whether the sentence says what literally happens: nothing buys, drives or hunts (unless it literally does). Write "a value receiver prevents mutation", not "a value receiver buys us immutability". Write "the pointer saves no allocation here", not "the pointer buys nothing here". Write "the tests that send it requests over HTTP", not "the tests that drive it over HTTP". Write "the reader cannot tell what is meant", not "it leaves the reader hunting for what was meant".
- Restating a general principle the facts already show. Start with the example.
- Rhetorical questions as headings: "So what does this mean?".
- Filler transitions: "let's dive in", "at the end of the day", "it's worth noting that", "as we can see".
- Praise of the user or of the question: "great question", "you're absolutely right".
- Typographic drama: spaced-out words, all-caps emphasis, exclamation marks, emoji, bold scattered over half the sentences.
- Em-dashes and ", so ..." clauses. Use a full stop, "which ...", or a colon after a complete sentence.
- Non-ASCII characters where ASCII exists: curly quotes, ellipsis, arrows, non-breaking spaces. Write `'`, `"`, `...`, `->` and a plain space.
- Stating what did not change, stayed, or was already correct.
- Hedging where a check would settle it. Check, then state the answer.
- Apologies and post-mortems after a mistake. Correct it and continue.

Bad:

> Here's where it gets interesting: the retry logic isn't just a nice-to-have, it's the load-bearing assumption of the entire sync pipeline. Three things jumped out at me, and the third one is the most instructive yet. [...] And third, and this is the kicker, the dedupe key includes a timestamp, which means retries are never actually deduplicated.

Good:

> The sync pipeline's retry logic has three bugs. `syncQueue.ts:142` swallows `ETIMEDOUT` instead of re-queuing the job. `backoff.ts:31` caps the delay at 2s, under the 8s p99 LTE reconnect time in `bench/network.json`. `dedupe.ts:77` puts a timestamp in the key, which means retries never deduplicate. All three reproduce in `syncQueue_test.ts` against the network stub. Fix: re-throw the timeout, raise the cap to 30s and strip the timestamp from the key.

## Code comments

Write a comment only if it adds value to the reader by providing non-obvious context information that the reader cannot easily infer from the code itself.

Don't write:

- Comments that repeat the next line: `// increment i` above `i++`, `// loop over users`, `// return the result`, `// error handling`.
- Doc comments that only spell out the name: `// UserID is the user ID.`
- Banners and dividers: `// --- helpers ---`, `// BEGIN`, `// END`.
- History: `// added in v2`, `// fixed #142`, `// was int64 before`. All history is in Git. Comments must only explain current code at hand, unless history is crucial context, e.g. when the code makes no sense without it.
- Commented-out code. Delete it, git has it.
- Comments written for the reviewer of the diff instead of the reader of the code: `// Note: now also handles nil`, `// Changed to a map for speed`, `// For simplicity we just skip this`.
- Comments that teach Go or the standard library: `// mu guards concurrent access` above a plain mutex, `// defer closes the file`, `// ok is false when the key is missing`. Assume that the reader is a seasoned software engineer with good understanding of Go.
- File and line references: `// see decoder.go:212`. They break as soon as the code moves and nothing checks them. Name the symbol with a doc link instead: `[Decoder.Next]`.
- Positional references: `// here and not above`, `// unlike the check below`, `// as mentioned earlier`. The reader cannot tell what is meant, and the words stop being true when the code moves. Name the symbol, or state the fact on its own.

Do write:

- Why the code works this way, when that isn't clear from reading it: why this order, this algorithm, this lock, what breaks without it.
- Where a value comes from: a constant, a timeout, a buffer size. Name the spec section, benchmark, RFC or issue and link it.
- Rules the types can't state: what the caller must guarantee, what the function assumes.
- Why the obvious approach wasn't used: name it and say what went wrong with it.
- `TODO: ...` with what to do and what unblocks it, not a bare `// TODO`.

Form:

- Respect [writing](#writing).
- Plain sentences, present tense, within the line limit.
- Go doc comments on exported names start with the name. This is the one place where repeating the name is required.
- Put the comment above the code, not after it. Comments after the code are for short notes on struct fields, enum values and table-test rows.
- Reference other code with Go doc links in square brackets: `[Scan]`, `[Parser.Parse]`, `[github.com/romshark/datapages.Subject]`.
- A doc comment on a symbol says what the symbol does: `// Close ends the open requests and is safe to call repeatedly.`
- A doc comment on a test explains what scenario is tested: `// TestCloseIdempotent tests that repeated Close calls all return nil.`

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

Read `.claude/skills/git-commits/SKILL.md` before writing a commit message or a changelog entry.

# Changelog

Read `.claude/skills/changelog/SKILL.md` before writing a CHANGELOG entry or releasing.

Fix released vulnerabilities in a GitHub advisory's private fork. Publish the advisory, release and entry together so disclosure does not precede an upgrade.

# Code Review

Read `.claude/skills/review-criteria/SKILL.md` before reviewing code or writing a `review-*.md` report.
