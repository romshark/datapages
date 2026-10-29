package main

import (
	"errors"
	"fmt"
	"strings"
)

// mainVersion is the version of the pages of the main branch. It's also the
// branch that these pages link on GitHub.
const mainVersion = "main"

// edition is the version of Datapages that a run documents. The site serves
// the pages of each version under docs/<version>/: those of the latest release
// and those of the main branch. Until a release has these pages, it serves
// main alone. The run for main also writes the files outside of the versions,
// which lead to the default version.
type edition struct {
	// version is mainVersion or the tag of a release.
	version string
	// release is the tag of the release that the site serves next to these
	// pages of main. It's empty for the pages of a release and for main
	// served alone.
	release string
}

// newEdition returns the edition that the -version and -release flags name.
func newEdition(version, release string) (edition, error) {
	switch {
	case version == "":
		return edition{}, errors.New("missing required -version")
	case version != mainVersion && !strings.HasPrefix(version, "v"):
		return edition{}, fmt.Errorf("-version %q is neither %s nor a release tag",
			version, mainVersion)
	case release != "" && version != mainVersion:
		return edition{}, fmt.Errorf("-release requires -version %s", mainVersion)
	case release != "" && !strings.HasPrefix(release, "v"):
		return edition{}, fmt.Errorf("-release %q is not a release tag", release)
	}
	return edition{version: version, release: release}, nil
}

// isMain reports whether the pages document the main branch.
func (e edition) isMain() bool { return e.version == mainVersion }

// indexed reports whether search engines may index the pages. The pages of
// main next to those of a release would compete with them in the results.
func (e edition) indexed() bool { return e.release == "" }

// ref returns the Git ref whose files the pages link on GitHub.
func (e edition) ref() string { return e.version }

// dir returns the directory of the pages of e, relative to the site root.
func (e edition) dir() string { return "docs/" + e.version + "/" }

// defaultVersion returns the version that the landing page and docs/ lead to:
// the release that the site serves next to main, or main alone.
func (e edition) defaultVersion() string {
	if e.release != "" {
		return e.release
	}
	return e.version
}

// versionLink is a version of the site in the version menu of a docs page.
type versionLink struct {
	Version string
	Current bool // Whether the page belongs to this version.
	// Href is the same page in this version, relative to the page.
	Href string
	// Fallback is the first page of this version, which docs.js opens
	// instead of a page that this version does not have.
	Fallback string
}

// versions returns the versions that the menu of the page at path offers, the
// release first. base is the URL of the root of the version relative to the
// page.
func (e edition) versions(base, path string) []versionLink {
	link := func(version string) versionLink {
		root := strings.TrimPrefix(base, "./") + "../" + version + "/"
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

// currentVersion returns the version of versions that the page belongs to.
func currentVersion(versions []versionLink) string {
	for _, l := range versions {
		if l.Current {
			return l.Version
		}
	}
	return ""
}
