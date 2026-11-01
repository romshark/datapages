package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// mainVersion is the version of the main branch pages,
// and the branch they link on GitHub.
const mainVersion = "main"

// releaseTag matches a release tag, as v1.2.3.
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// edition is the version of Datapages that a run documents. The run for main also
// writes the files outside the version directories, which lead to the default version.
type edition struct {
	// version is mainVersion or a release tag.
	version string
	// release is the release tag served next to main.
	// Empty for a release and for main alone.
	release string
}

// newEdition returns the edition of the -version and -release flags.
func newEdition(version, release string) (edition, error) {
	switch {
	case version == "":
		return edition{}, errors.New("missing required -version")
	case version != mainVersion && !releaseTag.MatchString(version):
		return edition{}, fmt.Errorf("-version %q is neither %s nor a release tag",
			version, mainVersion)
	case release != "" && version != mainVersion:
		return edition{}, fmt.Errorf("-release requires -version %s", mainVersion)
	case release != "" && !releaseTag.MatchString(release):
		return edition{}, fmt.Errorf("-release %q is not a release tag", release)
	}
	return edition{version: version, release: release}, nil
}

// isMain reports whether e documents the main branch.
func (e edition) isMain() bool { return e.version == mainVersion }

// indexed reports whether search engines may index the pages. Pages of main
// next to a release would compete with it in the results.
func (e edition) indexed() bool { return e.release == "" }

// ref returns the Git ref that the pages link on GitHub.
func (e edition) ref() string { return e.version }

// dir returns the directory of the pages of e, relative to the site root.
func (e edition) dir() string { return "docs/" + versionDir(e.version) + "/" }

// versionDir returns the directory under docs/ of version, which is main or
// the feature version of a release tag, as v1.2 for v1.2.3. A patch release
// replaces the pages of the release before it.
func versionDir(version string) string {
	if version == mainVersion {
		return version
	}
	return version[:strings.LastIndexByte(version, '.')]
}

// defaultVersion returns the version that the landing page and docs/ lead to,
// the release if the site serves one, or main.
func (e edition) defaultVersion() string {
	if e.release != "" {
		return e.release
	}
	return e.version
}

// versionLink is an entry of the version menu of a docs page.
type versionLink struct {
	Version string
	Current bool // Whether the page belongs to this version.
	// Href is the same page in this version, relative to the page.
	Href string
	// Fallback is the first page of this version, which docs.js opens when
	// this version lacks the page.
	Fallback string
}

// versions returns the version menu of the page at path, the release first.
// base is the URL of the version root relative to the page.
func (e edition) versions(base, path string) []versionLink {
	link := func(version string) versionLink {
		root := strings.TrimPrefix(base, "./") + "../" + versionDir(version) + "/"
		return versionLink{
			Version:  version,
			Current:  version == e.version,
			Href:     root + path,
			Fallback: root,
		}
	}
	switch {
	case !e.isMain():
		return []versionLink{link(e.version), link(mainVersion)}
	case e.release != "":
		return []versionLink{link(e.release), link(mainVersion)}
	}
	return []versionLink{link(mainVersion)}
}

// currentVersion returns the version among versions that the page belongs to.
func currentVersion(versions []versionLink) string {
	for _, l := range versions {
		if l.Current {
			return l.Version
		}
	}
	return ""
}
