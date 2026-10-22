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
	var version, root, addr string
	flag.StringVar(&version, "version", "", "Version string to render")
	// The default is the root as seen from this module's directory,
	// which is where go run, mage and templier run the tool.
	flag.StringVar(&root, "root", "../../..",
		"Repository root, which holds the Markdown files and docs/")
	flag.StringVar(&addr, "serve", "",
		"Serve the pages on this address instead of writing them to docs/")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "missing required -version")
		os.Exit(2)
	}

	if addr != "" {
		if err := serve(addr, root, version); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			os.Exit(1)
		}
		return
	}

	docs, err := readDocs(root, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read docs: %v\n", err)
		os.Exit(1)
	}
	files, err := siteFiles(docs, version)
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

// siteFiles returns the files that the site generates from docs, by their path
// relative to docs/: the pages, their Markdown copies, search.json, llms.txt
// and llms-full.txt.
func siteFiles(docs []doc, version string) (map[string][]byte, error) {
	pages := map[string]templ.Component{"index.html": indexPage(version)}
	files := map[string][]byte{
		"llms.txt":      llmsTxt(docPages, docs),
		"llms-full.txt": llmsFullTxt(docPages, docs),
	}
	for i, p := range docPages {
		if p.external() {
			continue
		}
		pages[p.outFile()] = docsPage(newView(docPages, docs, i, version))
		files[p.markdownFile()] = docs[i].markdown
	}
	search, err := searchJSON(docs)
	if err != nil {
		return nil, err
	}
	files["search.json"] = search
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
