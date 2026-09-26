---
name: git-commits
description: Write Datapages commit messages, including the breaking-change marker and the BREAKING block. Use when writing a commit message or a changelog entry.
---

- Respect [writing](../../../AGENTS.md#writing).
- Title: `type: Summary`, imperative, capitalized after the prefix, 50 characters or less, no trailing period.
- Types: `feat`, `fix`, `perf`, `refactor`, `test`, `chore`, `ci`, `docs`. Suffix the type with `!` for a breaking change.
- Mark a change as breaking when it incompatibly changes an application-facing API. Changes to `runtime/` APIs are not breaking: they are generator-facing, and `datapages gen` updates generated callers. Treat the root package, `modules/` and generated APIs such as `href` and `action` as public APIs.
- Wrap the description at 72 characters.
- Imperative mood in the title, never past tense: `Add cache`, not `Added cache` or `Adds cache`. The description uses present tense and describes the code as it is now: `Move foo to package buzz`, not `foo is now in package buzz`. No first person, no `we`.
- Don't paraphrase the code changes, the diff already shows them. The description is for what the diff can't show: why, scope, what was left out. Leave it empty when the title says everything.
- Don't list the files touched, don't restate the title in longer words and don't close with a summary of the commit.
- A commit that bundles several changes lists them as bullets, each with its own conventional prefix. List only the main changes. Tests, docs and call-site updates that come with a change are part of it, not separate bullets:

  ```
  feat: Add funcFoo <- keep, this is the change
  docs: Add docs for funcFoo <- drop
  test: Add tests for funcFoo <- drop
  refactor: Use funcFoo everywhere <- drop
  test: Add benchmark to test funcFoo against funcBar <- drop
  ```
- A breaking change ends with a `BREAKING:` block. It is the migration instruction: name every removed or renamed symbol as `old -> new`, and say what the caller has to do, one step per line. A caller must be able to migrate from this block alone, without reading the diff. Say plainly when a step is automatic (`datapages gen` regenerates it) and when it is manual. Every line is a step the caller takes. Never list what did not change: `href is unchanged` is a step to do nothing. When a step could be read as reaching further than it does, narrow the step instead of adding a line: write `EvSubjPref<Event> -> EvPrefix<Event>, the prefix constants only`, not `... unchanged` on a line of its own.
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
