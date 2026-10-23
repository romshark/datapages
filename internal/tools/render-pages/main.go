package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/a-h/templ"
)

func main() {
	var version, release, root, addr string
	flag.StringVar(&version, "version", "",
		"Version to document: main, or the tag of a release")
	flag.StringVar(&release, "release", "",
		"With -version main, the release that the site serves next to main, "+
			"which the landing page and docs/ lead to")
	// The default is the root as seen from this module's directory,
	// which is where go run, mage and templier run the tool.
	flag.StringVar(&root, "root", "../../..",
		"Repository root, which holds the Markdown files and docs/")
	flag.StringVar(&addr, "serve", "",
		"Serve the pages on this address instead of writing them to docs/")
	flag.Parse()

	e, err := newEdition(version, release)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if addr != "" {
		if err := serve(addr, root, e); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			os.Exit(1)
		}
		return
	}

	docs, err := readDocs(root, e.ref(), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read docs: %v\n", err)
		os.Exit(1)
	}
	assets := make(map[string][]byte)
	for _, name := range versionAssets {
		file := filepath.Join(root, "docs", name)
		if assets[name], err = os.ReadFile(file); err != nil {
			fmt.Fprintf(os.Stderr, "read asset: %v\n", err)
			os.Exit(1)
		}
	}
	files, err := siteFiles(docs, e, assets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
	for name, content := range files {
		path := filepath.Join(root, "docs", filepath.FromSlash(name))
		if err := writeFile(path, content); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", name, err)
			os.Exit(1)
		}
	}
}

// versionAssets are the files of docs/ that the pages of a version load from
// their own directory, which keeps the pages of a release on the stylesheet
// and the script that its templates were written for.
var versionAssets = []string{"style.css", "docs.js", "favicon.svg"}

// siteFiles returns the files that the site generates from docs in edition e,
// by their path relative to docs/ of the repository, the site root.
// The directory of the version holds the pages, their Markdown copies,
// search.json, llms.txt, llms-full.txt and assets, the contents of
// [versionAssets]. For main, the landing page, docs/index.html and the
// llms.txt of the site root lead to the default version.
func siteFiles(
	docs []doc, e edition, assets map[string][]byte,
) (map[string][]byte, error) {
	dir := e.dir()
	pages := make(map[string]templ.Component)
	files := map[string][]byte{
		dir + "llms.txt":      llmsTxt(docPages, docs),
		dir + "llms-full.txt": llmsFullTxt(docPages, docs),
	}
	for name, content := range assets {
		files[dir+name] = content
	}
	if e.isMain() {
		pages["index.html"] = indexPage(e)
		pages["docs/index.html"] = docsIndex(e)
		files["llms.txt"] = rootLlmsTxt(e, docs[0].description)
	}
	for i, p := range docPages {
		if p.external() {
			continue
		}
		pages[dir+p.outFile()] = docsPage(newView(docPages, docs, i, e))
		files[dir+p.markdownFile()] = docs[i].markdown
	}
	search, err := searchJSON(docs)
	if err != nil {
		return nil, err
	}
	files[dir+"search.json"] = search
	for name, page := range pages {
		var b bytes.Buffer
		if err := page.Render(context.Background(), &b); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = b.Bytes()
	}
	return files, nil
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
