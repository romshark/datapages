package main

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

// reloadStamp is the file that serve rewrites when the file of a [docPage] or a
// file of docs/ changes. templier watches the files of this module only, and a
// custom watcher in templier.yml reloads the browser tabs when this file
// changes.
const reloadStamp = ".reload"

// serve serves the site on addr for the watcher that templier.yml configures.
// It generates the site on every request for a file that it generates, which
// shows a change to the Markdown files without a rebuild, and serves
// style.css unminified.
func serve(addr, root, version string) error {
	var files []string
	for _, p := range docPages {
		if !p.external() {
			files = append(files, filepath.Join(root, filepath.FromSlash(p.file)))
		}
	}
	docsDir := filepath.Join(root, "docs")
	go signalChanges(reloadStamp, files, docsDir)

	generated := func(w http.ResponseWriter, name string) {
		docs, err := readDocs(root, true)
		var site map[string][]byte
		if err == nil {
			site, err = siteFiles(docs, version)
		}
		if err != nil {
			// templier adds its reload script to HTML only. A plain text
			// error would leave the tab without it, and fixing the Markdown
			// would not reload the tab.
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
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(site[name])
	}
	mux := http.NewServeMux()
	handle := func(pattern, name string) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			generated(w, name)
		})
	}
	handle("GET /{$}", "index.html")
	for _, name := range []string{"search.json", "llms.txt", "llms-full.txt"} {
		handle("GET /"+name, name)
	}
	for _, p := range docPages {
		if p.external() {
			continue
		}
		pattern := "GET /" + p.path
		if pattern[len(pattern)-1] == '/' {
			pattern += "{$}"
		}
		handle(pattern, p.outFile())
		handle("GET /"+p.markdownFile(), p.markdownFile())
	}
	// docs/style.css is the minified copy that mage genDocs writes.
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "style.css")
	})
	mux.Handle("GET /", http.FileServer(http.Dir(docsDir)))
	noCache := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Without it, a reload can take style.css from the cache, whose
		// freshness the browser derives from the modification time.
		w.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(w, r)
	})
	return http.ListenAndServe(addr, noCache)
}

// signalChanges writes the time to stamp whenever 1 of files or a file in the
// directory dir changes.
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

// lastChange returns the latest modification time of files and of the files in
// the directory dir.
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
