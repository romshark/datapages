# Agent Configuration Review

## 1. Is this the correct way to do it?

**Yes, the file locations are all correct** for their respective tools:

- `CLAUDE.md` — Claude Code (auto-loaded)
- `.github/copilot-instructions.md` — GitHub Copilot
- `.cursor/rules` — Cursor
- `.skills/datapages/SKILL.md` — Claude Code skill (auto-activated by trigger)
- `.skills/datastar/SKILL.md` — Claude Code skill

**One gap:** The skills (`.skills/`) are a Claude Code feature only. Copilot and Cursor
agents won't see them, so they have no access to the step-by-step framework guide or the
Datastar attribute reference. They only get the minimal config files, which don't teach
how to use Datapages at all.

For **OpenAI Codex**, you're missing a `codex.md` or `AGENTS.md` file entirely.

## 2. Consistency Issues

### A. Project Structure divergence

| Entry                      | CLAUDE.md      | .cursor/rules  | copilot-instructions.md |
|----------------------------|----------------|----------------|-------------------------|
| Project Structure section  | Full (8 items) | Partial (5)    | **Missing entirely**    |
| `example/counter/`         | Yes            | **No**         | —                       |
| `example/fancy-counter/`   | Yes            | **No**         | —                       |
| `example/tailwindcss/`     | Yes            | **No**         | —                       |

### B. Commits section divergence

`CLAUDE.md` has detailed rules (50-char title, 72-char wrap, each prefix explained).
Both `copilot-instructions.md` and `.cursor/rules` compress this to a single line:

```
Use conventional commits, prefix with `!` for breaking changes:
feat: fix: refactor: test: chore: ci: docs:
```

This means Copilot/Cursor agents won't follow the title length or description wrap rules.

### C. SKILL.md vs SPECIFICATION.md conflicts

These are the most important issues since they'll cause agents to generate incorrect code:

**`Head` method signature mismatch:**
- `SPECIFICATION.md:13-19`: returns `templ.Component` (single value), accepts optional
  `sessionToken` and `session` params
- `SKILL.md:439-441`: returns `(body templ.Component, err error)`, only shows
  `r *http.Request`

One of these is wrong. An agent following the SKILL will write code the parser rejects,
or vice versa.

**Parameter ordering:**
- `SPECIFICATION.md:59`: "Parameters may be in any order."
- `SKILL.md:224-232`: "Use them in this order" with `r *http.Request // always first`
  and `dispatch ... // always last`

These directly contradict. If the parser truly accepts any order, the SKILL shouldn't
say "always first/last."

**Action handler return values — missing `head`:**
- `SPECIFICATION.md:137-138` includes `head templ.Component` as an optional return value
  for non-SSE action handlers
- `SKILL.md:241-247` omits `head` entirely from the action return type list

**`OnXXX` handler parameters:**
- `SPECIFICATION.md:149-162` shows only `event`, `sse`, and `session` as parameters
- `SKILL.md:348-350` adds `sessionToken string` and `signals struct{...}` as optional
  params

If the parser supports these, the spec should document them. If not, the skill is wrong.

**App-level actions:**
- `SKILL.md:206-217` documents `func (*App) POSTSignOut(...)` style global actions
- `SPECIFICATION.md` doesn't mention App-level actions at all

## 3. Missing Things

- **No framework guide for Copilot/Cursor agents.** They get code style and commands but
  zero information about how Datapages works. They won't know about the
  `// PageXXX is /route` comment convention, required `App *App` field, event types, etc.
  Consider either duplicating key parts of the SKILL into those files, or adding a
  reference to SPECIFICATION.md.

- **No Codex config.** OpenAI Codex reads `codex.md` or `AGENTS.md`.

- **The SKILL doesn't mention `datapages.yaml` contents or structure.** After
  `datapages init`, agents may need to modify this file but have no guidance.

- **No guidance on `.templ` file conventions** (naming, where to put them, relationship
  to page types). Agents will guess.

## 4. Other Issues

- **Experimental `metrics` section in SPECIFICATION.md** (line 508) says "not implemented
  yet" but is presented alongside real features. An agent reading the spec may try to use
  it and fail. Consider moving it to a separate document or clearly gating it.

- **The datastar SKILL** (line 15) tells agents to "fetch from
  `https://context7.com/...`" — this assumes the agent has web access, which many don't.
  Consider inlining the essential Datastar attributes directly.

- **SKILL.md references `../../SPECIFICATION.md`** — this relative link works for Claude
  Code which reads local files, but wouldn't work for tools that don't resolve relative
  markdown links.

## Summary of Recommended Actions

1. **Sync the three config files** (CLAUDE.md, copilot, cursor) — make Project Structure
   and Commits identical, or generate them from a single source
2. **Resolve SKILL.md vs SPECIFICATION.md conflicts** — especially the `Head` signature,
   parameter ordering, missing `head` return, and `OnXXX` params
3. **Add Datapages framework basics to Copilot/Cursor configs** or at minimum point them
   to SPECIFICATION.md
4. **Add a `codex.md`** for OpenAI Codex
5. **Mark or remove the experimental `metrics` section** to prevent agent confusion
