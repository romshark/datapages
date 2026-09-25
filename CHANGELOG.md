# Changelog

This file records the notable changes of each release in the format of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release
workflow publishes the section of a version as its GitHub release notes.
Releases up to v0.10.0 have their notes on
[GitHub Releases](https://github.com/romshark/datapages/releases) only.

## [Unreleased]

### Security

- Prevent signed-in users from receiving private events addressed to another
  user when their streams share the event's signal value. This affects v0.10.0.
  It also affects v0.7.0 through v0.9.4 when the page handles a public
  signal-scoped event. Upgrade to v0.10.1 and run `datapages gen`.

[Unreleased]: https://github.com/romshark/datapages/compare/v0.10.0...HEAD
