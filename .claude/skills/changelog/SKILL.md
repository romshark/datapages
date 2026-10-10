---
name: changelog
description: Write CHANGELOG.md entries, including Security entries, and release a version. Use when adding a changelog entry or releasing.
---

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release workflow publishes each tag's section and fails when it's missing. `TestChangelogReleases` validates version headings.

- Add user-visible framework, CLI and generated-code changes under `## [Unreleased]` in the same commit. Omit `docs`, `test`, `chore` and `ci`.
- Sort entries into `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed` and `Security`.
- Within a section, group entries under `####` subheadings by where the change applies, in this order: `` `datapages lint` and `datapages gen` `` for what the app model may contain and what is reported about it; `Generated code`, opened by the line "Run `datapages gen` to apply these."; `Runtime and modules` for what applies on upgrade alone; `` `datapages init` `` for the files it writes. Leave out a subheading with no entries.
- Don't repeat in an entry what its subheading says: no "in `datapages lint` and `datapages gen`" and no closing "Run `datapages gen`."
- Use imperative entries like commit titles: `Reject subject fields tagged json:"-"`. Say enough for users to determine whether they are affected.
- A `Security` entry follows [Security entries](#security-entries).
- A breaking change repeats the migration steps of its `BREAKING:` block.
- Do not add a separate entry for a `runtime/` API change. The entry for the user-visible change that requires it goes under `Generated code`.
- To release, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`, insert an empty `## [Unreleased]`, update the compare links and tag `vX.Y.Z`.

# Security entries

A security entry lets users determine exposure and remediation:

- State who can read or do what to whom: "any signed-in user can read another user's private events", not "a privacy issue".
- Describe affected applications with terms users can find in their code.
- Give exact affected versions, verified against the release history.
- Give upgrade, regeneration and redeploy steps. Include a workaround or say there is none.
- Link the GHSA or CVE when one exists.

Leave severity labels, CVSS scores and exploit steps to the advisory.
