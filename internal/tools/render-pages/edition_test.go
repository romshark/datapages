package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewEdition tests the combinations of -version and -release that the
// Pages workflow and mage pass, and that a release page takes no -release.
func TestNewEdition(t *testing.T) {
	for name, tt := range map[string]struct {
		version, release string
		want             edition
		wantErr          bool
	}{
		"main alone": {
			version: "main",
			want:    edition{version: "main"},
		},
		"main next to a release": {
			version: "main", release: "v1.0.0",
			want: edition{version: "main", release: "v1.0.0"},
		},
		"release": {
			version: "v1.0.0",
			want:    edition{version: "v1.0.0"},
		},
		"no version":             {wantErr: true},
		"neither main nor a tag": {version: "dev", wantErr: true},
		"release with -release": {
			version: "v1.0.0", release: "v1.0.0",
			wantErr: true,
		},
		"-release not a tag": {
			version: "main", release: "latest",
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := newEdition(tt.version, tt.release)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestVersions tests that the version menu of a page offers the same page in
// each version that the site serves, the release first, with the first page
// of the version as the fallback.
func TestVersions(t *testing.T) {
	for name, tt := range map[string]struct {
		e          edition
		base, path string
		want       []versionLink
	}{
		"release": {
			e:    edition{version: "v1"},
			base: "./", path: "events",
			want: []versionLink{
				{Version: "v1", Current: true, Href: "../v1/events", Fallback: "../v1/"},
				{Version: "main", Href: "../main/events", Fallback: "../main/"},
			},
		},
		"first page of a release": {
			e:    edition{version: "v1"},
			base: "./", path: "",
			want: []versionLink{
				{Version: "v1", Current: true, Href: "../v1/", Fallback: "../v1/"},
				{Version: "main", Href: "../main/", Fallback: "../main/"},
			},
		},
		"main next to a release": {
			e:    edition{version: "main", release: "v1"},
			base: "./", path: "events",
			want: []versionLink{
				{Version: "v1", Href: "../v1/events", Fallback: "../v1/"},
				{
					Version: "main", Current: true,
					Href: "../main/events", Fallback: "../main/",
				},
			},
		},
		"main alone": {
			e:    edition{version: "main"},
			base: "./", path: "events",
			want: []versionLink{
				{
					Version: "main", Current: true,
					Href: "../main/events", Fallback: "../main/",
				},
			},
		},
		"page in a directory": {
			e:    edition{version: "v1"},
			base: "../", path: "a/b",
			want: []versionLink{
				{
					Version: "v1", Current: true,
					Href: "../../v1/a/b", Fallback: "../../v1/",
				},
				{Version: "main", Href: "../../main/a/b", Fallback: "../../main/"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.e.versions(tt.base, tt.path))
		})
	}
}

// TestEditionPages tests what a docs page shows of its edition: the version
// menu, the note on a page of main, and noindex on the pages of main next to a
// release.
func TestEditionPages(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil, mainVersion)
	require.NoError(t, err)

	const noindex = `<meta name="robots" content="noindex">`
	for name, tt := range map[string]struct {
		e             edition
		want, wantNot []string
	}{
		"release": {
			e: edition{version: "v1"},
			want: []string{
				`<neo-select value="v1" size="sm" aria-label="Documentation version"`,
				`class="version-select"`,
				`value="v1" data-neo-value="v1" data-fallback="../../v1/"` +
					` data-href="../../v1/docs/events">v1</neo-option>`,
				`value="main" data-neo-value="main" data-fallback="../../main/"` +
					` data-href="../../main/docs/events">main</neo-option>`,
			},
			wantNot: []string{`class="main-note"`, noindex},
		},
		"main next to a release": {
			e: edition{version: "main", release: "v1"},
			want: []string{
				`<neo-select value="main" size="sm"`,
				`data-href="../../v1/docs/events">v1</neo-option>`,
				`<p class="main-note" role="note">` +
					`This page documents the unreleased main branch. ` +
					`Read <a href="../../v1/docs/events" data-fallback="../../v1/">` +
					`the page of v1</a>, the latest release.</p>`,
				noindex,
			},
		},
		"main alone": {
			e: edition{version: "main"},
			want: []string{
				`<neo-select value="main" size="sm"`,
				`value="main" data-neo-value="main" data-fallback="../../main/"` +
					` data-href="../../main/docs/events">main</neo-option>`,
				`<p class="main-note" role="note">` +
					`This page documents the unreleased main branch.`,
			},
			wantNot: []string{"neo-option value=\"v", "the latest release", noindex},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := render(t, docsPage(newView(testPages, docs, 1, tt.e)))
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
			for _, notWant := range tt.wantNot {
				require.NotContains(t, html, notWant)
			}
		})
	}
}

// TestDocsPageLinks tests that the header of a docs page links the landing page
// at the site root and the parts of the site in the version of the page, and
// that its footer links the changelog and llms.txt of that version.
func TestDocsPageLinks(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil, mainVersion)
	require.NoError(t, err)
	html := render(t, docsPage(newView(testPages, docs, 1, edition{version: "v1"})))
	for _, want := range []string{
		`<a class="logo" href="../../../" aria-label="Datapages home">`,
		`<a class="header-link" href="../" aria-current="page">Docs</a>`,
		`<a class="header-link" href="../examples">Examples</a>`,
		`<a class="header-link" href="../specification">Specification</a>`,
		`<a class="icon-link" href="../changelog">`,
		`<a class="icon-link" href="../llms.txt">`,
	} {
		require.Contains(t, html, want)
	}
}

// TestLandingPage tests that the landing page leads to the default version:
// the release next to main, or main alone, and is not indexed next to a
// release only in the copy of main.
func TestLandingPage(t *testing.T) {
	for name, tt := range map[string]struct {
		e             edition
		want, wantNot []string
	}{
		"main next to a release": {
			e: edition{version: "main", release: "v1"},
			want: []string{
				"datapages/cmd/datapages@v1</div>",
				`<a href="docs/v1/installation" class="btn btn-primary">`,
				`<a href="docs/v1/" class="btn btn-secondary">`,
				`<a class="header-link" href="docs/v1/">Docs</a>`,
				`<a class="icon-link" href="docs/v1/changelog">`,
			},
			wantNot: []string{"neo-select"},
		},
		"main alone": {
			e: edition{version: "main"},
			want: []string{
				"datapages/cmd/datapages@main</div>",
				`<a href="docs/main/installation" class="btn btn-primary">`,
			},
			wantNot: []string{"neo-select"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := render(t, indexPage(tt.e))
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
			for _, notWant := range tt.wantNot {
				require.NotContains(t, html, notWant)
			}
		})
	}
}

// TestSiteFiles tests that a run writes the files of its version under
// docs/<version>/ with the assets that the pages load, and that only the run
// for main writes the files outside of the versions.
func TestSiteFiles(t *testing.T) {
	docs, err := readDocs(t.TempDir(), mainVersion, true)
	require.NoError(t, err)
	assets := map[string][]byte{"style.css": []byte("css")}

	for name, tt := range map[string]struct {
		e       edition
		outside []string
	}{
		"release": {e: edition{version: "v1"}},
		"main next to a release": {
			e:       edition{version: "main", release: "v1"},
			outside: []string{"docs/index.html", "index.html", "llms.txt"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := siteFiles(docs, tt.e, assets)
			require.NoError(t, err)
			dir := "docs/" + tt.e.version + "/"
			var outside []string
			for name := range files {
				if !strings.HasPrefix(name, dir) {
					outside = append(outside, name)
				}
			}
			slices.Sort(outside)
			require.Equal(t, tt.outside, outside)
			for _, name := range []string{
				"index.html", "index.md", "events.html", "events.md",
				"specification.html", "search.json", "llms.txt", "llms-full.txt",
			} {
				require.Contains(t, files, dir+name)
			}
			require.Equal(t, []byte("css"), files[dir+"style.css"])
		})
	}
}

// TestDocsIndex tests that docs/ of the site opens the first page of the
// default version.
func TestDocsIndex(t *testing.T) {
	html := render(t, docsIndex(edition{version: "main", release: "v1"}))
	require.Contains(t, html, `<meta http-equiv="refresh" content="0; url=v1/">`)
	require.Contains(t, html, `<a href="v1/">`)
	require.Contains(t, html, `<link rel="icon" href="../favicon.svg"`)
}

// TestRootLlmsTxt tests that the llms.txt of the site root links the llms.txt
// of each version, the release first.
func TestRootLlmsTxt(t *testing.T) {
	require.Equal(t, "# Datapages\n\n> Intro.\n\n"+
		"Each link leads to the llms.txt of a version of the documentation.\n\n"+
		"## Versions\n\n"+
		"- [v1](docs/v1/llms.txt): The latest release\n"+
		"- [main](docs/main/llms.txt): The main branch, unreleased\n",
		string(rootLlmsTxt(edition{version: "main", release: "v1"}, "Intro.")))
	require.NotContains(t,
		string(rootLlmsTxt(edition{version: "main"}, "Intro.")), "latest release")
}

// TestGitHubLinksFollowTheRef tests that the pages of a release link the files
// of the repository and their own file at the release tag, not on main.
func TestGitHubLinksFollowTheRef(t *testing.T) {
	const tree = "https://github.com/romshark/datapages/blob/v1/"
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n\nSee [the FAQ](../FAQ.md) and [examples](../example/).\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil, "v1")
	require.NoError(t, err)

	require.Contains(t, docs[1].body, `<a href="`+tree+`FAQ.md">`)
	require.Contains(t, docs[1].body,
		`<a href="https://github.com/romshark/datapages/tree/v1/example/">`)
	require.Contains(t, string(docs[1].markdown), "("+tree+"FAQ.md)")
	html := render(t, docsPage(newView(testPages, docs, 1, edition{version: "v1"})))
	require.Contains(t, html, `<a class="icon-link" href="`+tree+`guide/events.md">`)
}

// TestMorpheusIconBase tests that a docs page points <neo-icon> at the icons of
// Morpheus on jsDelivr after morpheus.css, whose /static/icons it replaces.
func TestMorpheusIconBase(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil, mainVersion)
	require.NoError(t, err)
	html := render(t, docsPage(newView(testPages, docs, 1, edition{version: "v1"})))

	base := regexp.MustCompile(`<style>:root\{--neo-icon-base:"` +
		`https://cdn\.jsdelivr\.net/gh/romshark/morpheus@[^/"]+` +
		`/internal/site/static/icons"\}</style>`).FindStringIndex(html)
	require.NotNil(t, base, "no icon base")
	css := strings.Index(html, `morpheus.css">`)
	require.Positive(t, css)
	require.Less(t, css, base[0])
}
