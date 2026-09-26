package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const changelog = `# Changelog

Intro.

## [Unreleased]

## [0.11.0] - 2026-10-02

### Added

- Newer entry.

## [0.10.1] - 2026-09-26

### Security

- Older entry.

[Unreleased]: https://github.com/romshark/datapages/compare/v0.11.0...HEAD
[0.11.0]: https://github.com/romshark/datapages/compare/v0.10.1...v0.11.0
[0.10.1]: https://github.com/romshark/datapages/compare/v0.10.0...v0.10.1
`

func TestSection(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		want    string
		ok      bool
	}{
		"ends at next heading":    {version: "0.11.0", want: "### Added\n\n- Newer entry.\n", ok: true},
		"ends at link definition": {version: "0.10.1", want: "### Security\n\n- Older entry.\n", ok: true},
		"missing":                 {version: "0.9.0"},
		"prefix of a version":     {version: "0.1"},
		"empty":                   {version: "Unreleased"},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := section(changelog, tc.version)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLink(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		want    string
		ok      bool
	}{
		"latest": {
			version: "0.11.0",
			want:    "https://github.com/romshark/datapages/compare/v0.10.1...v0.11.0",
			ok:      true,
		},
		"older": {
			version: "0.10.1",
			want:    "https://github.com/romshark/datapages/compare/v0.10.0...v0.10.1",
			ok:      true,
		},
		"missing":             {version: "0.9.0"},
		"prefix of a version": {version: "0.1"},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := link(changelog, tc.version)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestChangelogReleases tests that every released version
// has a valid heading, non-empty notes and a link definition.
func TestChangelogReleases(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "CHANGELOG.md"))
	require.NoError(t, err)

	semver := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	for line := range strings.Lines(string(b)) {
		rest, ok := strings.CutPrefix(line, "## [")
		if !ok {
			continue
		}
		version, _, ok := strings.Cut(rest, "]")
		require.True(t, ok, "unterminated heading %q", line)
		if version == "Unreleased" {
			continue
		}
		require.Regexp(t, semver, version, "heading %q", strings.TrimSpace(line))
		_, ok = section(string(b), version)
		require.True(t, ok, "the section of %s has no notes", version)
		_, ok = link(string(b), version)
		require.True(t, ok, "%s has no link definition", version)
	}
}
