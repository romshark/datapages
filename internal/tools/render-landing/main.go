package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
)

func main() {
	// The default is the root as seen from this module's directory,
	// which is where go run and mage run the tool.
	root := flag.String("root", "../../..",
		"Repository root, which holds docs/ with the docs that docpages wrote")
	flag.Parse()
	if err := run(filepath.Join(*root, "docs")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run writes index.html into site, the root of the GitHub Pages site, which
// holds the docs that docpages wrote into its docs/.
func run(site string) error {
	redirect, err := os.ReadFile(filepath.Join(site, "docs", "index.html"))
	if err != nil {
		return fmt.Errorf("read the docs that docpages writes: %w", err)
	}
	version, err := defaultVersion(redirect)
	if err != nil {
		return err
	}
	l := landing{version: version}
	if err := l.check(site); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := indexPage(l).Render(context.Background(), &b); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(site, "index.html"), b.Bytes(), 0o644)
}

// The paths of the docs that the landing page links, relative to the root of a
// version. They follow the folders and file names of guide/.
const (
	pathInstallation = "get-started/installation"
	pathExamples     = "more/examples"
	pathFAQ          = "more/faq"
	pathChangelog    = "reference/changelog"
	pathLLMsTxt      = "llms.txt"
	pathFavicon      = "favicon.svg"
)

// mainVersion is the version of the docs of the main branch.
const mainVersion = "main"

// landing is what the landing page links, which is the docs of the version
// that docs/docs/index.html of the site redirects to.
type landing struct {
	// version is mainVersion or the feature version of the latest release,
	// as v1.2.
	version string
}

// docs returns the URL of the root of the version, relative to the landing page.
func (l landing) docs() string { return "docs/" + l.version + "/" }

// install returns the version of the go install command. It is main for the
// docs of main, otherwise latest, which is the release that the docs document.
func (l landing) install() string {
	if l.version == mainVersion {
		return mainVersion
	}
	return "latest"
}

// check returns an error for each path that the landing page links and the
// docs of its version under site lack.
func (l landing) check(site string) error {
	var errs []error
	for _, p := range []string{
		pathInstallation, pathExamples, pathFAQ, pathChangelog, pathLLMsTxt, pathFavicon,
	} {
		// GitHub Pages serves a page from the file with ".html" appended.
		file := p
		if path.Ext(p) == "" {
			file += ".html"
		}
		_, err := os.Stat(filepath.Join(site, "docs", l.version, filepath.FromSlash(file)))
		if err != nil {
			errs = append(errs, fmt.Errorf("the landing page links %s%s: %w", l.docs(), p, err))
		}
	}
	return errors.Join(errs...)
}

// refresh matches the redirect of index.html that docpages writes into its
// output folder, as `<meta http-equiv="refresh" content="0; url=v1.2/">`.
var refresh = regexp.MustCompile(`<meta http-equiv="refresh" content="0; url=([^"]*)/">`)

// version matches the feature version of a release, as v1.2.
var version = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)

// defaultVersion returns the version that redirect, the index.html of the
// output folder of docpages, opens.
func defaultVersion(redirect []byte) (string, error) {
	m := refresh.FindSubmatch(redirect)
	if m == nil {
		return "", errors.New("the index.html of docpages has no redirect to a version")
	}
	v := string(m[1])
	if v != mainVersion && !version.MatchString(v) {
		return "", fmt.Errorf("the index.html of docpages redirects to %q, "+
			"neither %s nor a feature version", v+"/", mainVersion)
	}
	return v, nil
}
