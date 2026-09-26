---
name: review-criteria
description: Classify Datapages findings by reach and severity and write review-*.md reports. Use for code reviews and security audits.
---

# Reach

Reach describes where the cause runs, not where the symptom appears.

- Production: users' deployed servers and their visitors' browsers.
  - The root package, `runtime/`, `modules/`, and `modules/offline/sw.js`.
  - Generated `datapagesgen`, including committed example and acceptance output.
  - Files `datapages init` writes from `internal/generator/skeleton`.
  - `.github/workflows/release.yml` and `.goreleaser.yaml`.
- Development: the developer's machine and CI.
  - The CLI, parser, and generator, including crashes, wrong diagnostics, and generated code that does not compile.
  - `datapages watch` and code gated by `datapages.IsDevMode`.
  - Agent skills under `internal/generator/agentdocs/data`.
- Repository: code no user application runs.
  - Examples and acceptance cases outside `datapagesgen`, `internal/tools/`, `magefiles/`, `docs/`, and tests.

Apply these exceptions:

- A defect found in an example has the reach of its cause. An example XSS caused by `runtime/htmlattr` has production reach.
- A `datapages.IsDevMode` branch has development reach unless a request can enable it; then it has production reach.
- An unsafe example pattern has repository reach. Recommend the documentation or skill that should warn against it.
- Application responsibilities listed in `SECURITY.md` are not framework vulnerabilities. Missing guidance is Info.

# Severity

Rate impact as if the cause ran in production:

- Critical: a remote client needs only network access to run server code, run JavaScript in another visitor's browser, or read or change another session's data.
- High: Critical impact behind an attacker-controlled precondition, such as an account or a documented application pattern; a few requests crash or hang the server; a correct application silently loses a write or misroutes data.
- Medium: a common path fails with a workaround; sustained load grows memory or goroutines without bound; paths, versions, or timing leak without direct value to an attacker.
- Low: unusual input or configuration causes a defect; a diagnostic or log is wrong; code is slow outside a hot path.
- Info: no defect. Use for hardening, test gaps, and unclear documentation.

Lower impact one level for development reach and two for repository reach, but not below Low. Remote code execution through `datapages watch` from a page the developer visits is High. XSS in an example template is Medium.

# Report

Write `review-<topic>.md` in the repository root. `.gitignore` excludes it. For a single-diff review in chat, use the finding fields and omit the report shell.

```markdown
# Review: <scope>

Commit `<hash>`. Scope: <paths>. Method: <what was read, run, fuzzed or measured>.

<Bottom line: the most severe findings and what to fix first.>

| ID | Severity | Reach | Title | Status |
| -- | -------- | ----- | ----- | ------ |
| F1 | ...      | ...   | ...   | ...    |

## F1. <The defect as a sentence>

- Severity: <level>, or <level> (impact <level>, <reach> reach) when lowered
- Reach: Production | Development | Repository
- Type: CWE-<n> <name> | Correctness | Concurrency | Resource leak | Performance | API | Documentation | Test
- Location: `<path>:<line>` at the reviewed commit
- Status: Open

Description: <what the code does and why it is wrong>.

Impact: <who can do what to whom, or what a framework user sees>.

Reproduction: <test, command, or input; expected and actual output>.

Recommendation: <fix and the test that fails without it>.

## Areas reviewed, no finding

- <Component>: <what was checked and the evidence that it is correct>.

## Test coverage gaps

- <What no test exercises, and the finding it would have caught>.
```

- Number findings `F1`, `F2`, ... in discovery order. Never renumber; later passes continue the sequence.
- List findings by severity, then by ID.
- State the defect in the title: "`WithFilterSignals` writes patterns into a regex literal unescaped", not "Regex escaping issue".
- Use a [CWE](https://cwe.mitre.org/) ID as the type of a vulnerability.
- Reproduce every finding with a failing test, command output, or program output.
- Use status Open; Fixed in `<commit>` by `<TestName>`; Won't fix with the reason; Not a defect with evidence; or Unconfirmed with confirmation steps. Use Unconfirmed for findings from reading alone. Before accepting Fixed, `git grep` the cited test at HEAD.
- Close a finding by striking through its title and prefixing it, in the heading and in the table: `✅ ~~<title>~~` when Fixed, `❌ WONTFIX: ~~<title>~~` when Won't fix.
- A Won't fix finding needs a paragraph under its heading explaining why. Labels such as "out of scope" are insufficient.
- State correct behavior only under "Areas reviewed, no finding", with the evidence. This prevents later passes from repeating the work.
