package main

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// reloadStamp is the file that serve rewrites when the file of a [docPage] or a
// file of docs/ changes. templier watches the files of this module only, and a
// custom watcher in templier.yml reloads the browser tabs when this file changes.
const reloadStamp = ".reload"

// serve serves the site on addr for the watcher that templier.yml configures.
// It generates the site on every request for a file that it generates, which
// shows a change to the Markdown files without a rebuild, and serves style.css
// unminified. Paths resolve as on GitHub Pages: a path that ends in "/" serves
// its index.html, and a path without an extension the file with ".html" appended.
func serve(addr, root string, e edition) error {
	var files []string
	for _, p := range docPages {
		if !p.external() {
			files = append(files, filepath.Join(root, filepath.FromSlash(p.file)))
		}
	}
	docsDir := filepath.Join(root, "docs")
	go signalChanges(reloadStamp, files, docsDir)

	generated := func(w http.ResponseWriter, r *http.Request, name string) {
		docs, err := readDocs(root, e.ref(), true)
		var site map[string][]byte
		if err == nil {
			var assets map[string][]byte
			if assets, err = serveAssets(docsDir); err == nil {
				site, err = siteFiles(docs, e, assets)
			}
		}
		if err != nil {
			// templier adds its reload script to HTML only.
			// A plain text error would leave the tab without it,
			// and fixing the Markdown would not reload the tab.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "<!DOCTYPE html><pre>%s</pre>",
				html.EscapeString(err.Error()))
			return
		}
		ctype := mime.TypeByExtension(path.Ext(name))
		switch path.Ext(name) {
		case ".md":
			ctype = "text/markdown; charset=utf-8"
		case ".txt":
			ctype = "text/plain; charset=utf-8"
		}
		content, ok := site[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(content)
	}
	static := http.FileServer(http.Dir(docsDir))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		switch {
		case name == "" || strings.HasSuffix(name, "/"):
			name += "index.html"
		case path.Ext(name) == "":
			name += ".html"
		}
		// docs/ of the site holds generated files only.
		// Those that mage genDocs wrote into the repository may be stale.
		isGenerated := name == "index.html" || name == "llms.txt" ||
			strings.HasPrefix(name, "docs/")
		if isGenerated {
			generated(w, r, name)
			return
		}
		static.ServeHTTP(w, r)
	})
	// docs/style.css is the minified copy that mage genDocs writes.
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "style.css")
	})
	noCache := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Without it, a reload can take style.css from the cache, whose
		// freshness the browser derives from the modification time.
		w.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(w, r)
	})
	return http.ListenAndServe(addr, noCache)
}

// serveAssets reads the files of [versionAssets] for serve:
// style.css unminified from this directory, the others from docsDir.
func serveAssets(docsDir string) (map[string][]byte, error) {
	assets := make(map[string][]byte)
	for _, name := range versionAssets {
		file := filepath.Join(docsDir, name)
		if name == "style.css" {
			file = name
		}
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		assets[name] = content
	}
	return assets, nil
}

// signalChanges writes the time to stamp whenever 1 of
// files or a file in the directory dir changes.
func signalChanges(stamp string, files []string, dir string) {
	last := lastChange(files, dir)
	for range time.Tick(250 * time.Millisecond) {
		t := lastChange(files, dir)
		if t.Equal(last) {
			continue
		}
		last = t
		// templier ignores a change of the modification time alone.
		_ = os.WriteFile(stamp, []byte(t.String()), 0o644)
	}
}

// lastChange returns the latest modification time of
// files and of the files in the directory dir.
func lastChange(files []string, dir string) time.Time {
	var latest time.Time
	consider := func(path string) {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	for _, f := range files {
		consider(f)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			consider(filepath.Join(dir, e.Name()))
		}
	}
	return latest
}
