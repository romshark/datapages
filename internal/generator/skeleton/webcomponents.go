package skeleton

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"

	"github.com/romshark/datapages/runtime/httpserve"
)

// WebComponents is the setup init scaffolds for Web Components.
type WebComponents string

const (
	WebComponentsNone WebComponents = "none"

	// WebComponentsRocket defines components with Datastar Rocket, whose bundle
	// replaces the Datastar bundle of the page.
	WebComponentsRocket WebComponents = "rocket"

	// WebComponentsTypeScript writes components in TypeScript under web/src,
	// which esbuild bundles into app/static/components.js.
	WebComponentsTypeScript WebComponents = "typescript"

	// WebComponentsJavaScript writes components as JavaScript modules under
	// app/static/components, which the server serves as they are.
	WebComponentsJavaScript WebComponents = "javascript"
)

// WebComponentsChoices lists every setup in the order init offers them.
var WebComponentsChoices = []WebComponents{
	WebComponentsRocket, WebComponentsTypeScript, WebComponentsJavaScript, WebComponentsNone,
}

// ParseWebComponents returns the setup named s.
func ParseWebComponents(s string) (WebComponents, error) {
	for _, w := range WebComponentsChoices {
		if string(w) == s {
			return w, nil
		}
	}
	names := make([]string, len(WebComponentsChoices))
	for i, w := range WebComponentsChoices {
		names[i] = string(w)
	}
	return "", fmt.Errorf("unknown Web Components setup %q, want one of %s",
		s, strings.Join(names, ", "))
}

// RocketJS is the Datastar bundle with Rocket of the release Datapages loads by
// default. Components import rocket from the same URL. A different URL would
// load a second copy of Datastar into the page.
var RocketJS = strings.TrimSuffix(httpserve.DefaultDatastarJSSrc, "datastar.js") +
	"datastar-rocket.js"

// TypeScriptWatchYAML continues the watch section of the default
// datapages.yaml for [WebComponentsTypeScript]: datapages watch bundles the
// components on every change of their source.
const TypeScriptWatchYAML = `  watcher-ignore:
    # The tree is large and would use up watch descriptors.
    - "web/node_modules/**"
  custom-watchers:
    - name: "Bundle the Web Components (esbuild)"
      include:
        - "web/src/**/*.ts"
      cmd: "cd web && node build.mjs"
      fail-on-error: true
      requires: reload
`

// File is a file of the scaffold. Path is relative to the module root and
// uses forward slashes.
type File struct {
	Path    string
	Content []byte
}

//go:embed webcomponents
var webComponentsFS embed.FS

// WebComponentFiles returns the files of setup w other than app/app.go and
// app/app.templ, which [AppGo] and [AppTempl] render. A file ending in .tmpl
// is a template with [[ ]] delimiters, written without the suffix.
func WebComponentFiles(w WebComponents) ([]File, error) {
	if w == WebComponentsNone {
		return nil, nil
	}
	root := path.Join("webcomponents", string(w))
	var files []File
	err := fs.WalkDir(webComponentsFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := webComponentsFS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		if name, ok := strings.CutSuffix(rel, ".tmpl"); ok {
			t, err := template.New(name).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				return fmt.Errorf("parsing %s: %w", p, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, struct{ RocketJS string }{RocketJS}); err != nil {
				return fmt.Errorf("executing %s: %w", p, err)
			}
			rel, content = name, buf.Bytes()
		}
		files = append(files, File{Path: rel, Content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the %s scaffold: %w", w, err)
	}
	return files, nil
}

// componentsScript is the element of the page head that loads the components
// of w, with its entry referenced through the generated assets package.
func componentsScript(w WebComponents) string {
	if w == WebComponentsTypeScript {
		return `<script src={ assets.Path("components.js") } defer></script>`
	}
	return `<script type="module" src={ assets.Path("components/index.js") }></script>`
}

// ComponentsDir returns the directory of the component sources of w,
// relative to the module root, with a trailing slash.
func ComponentsDir(w WebComponents) string {
	switch w {
	case WebComponentsTypeScript:
		return "web/src/"
	case WebComponentsRocket, WebComponentsJavaScript:
		return "app/static/components/"
	}
	return ""
}

// ComponentsAbout describes the components of w for AGENTS.md.
func ComponentsAbout(w WebComponents) string {
	switch w {
	case WebComponentsTypeScript:
		return "Web Components in TypeScript. `make bundle` and `datapages watch` " +
			"bundle them into `app/static/`."
	case WebComponentsJavaScript:
		return "Web Components as JavaScript modules, served as they are. " +
			"`index.js` imports each."
	case WebComponentsRocket:
		return "Web Components made with Datastar Rocket, which is in beta: " +
			"https://data-star.dev/reference/rocket. `index.js` imports each. " +
			"Import `rocket` from the URL that `WithDatastarJS` loads in the entry point."
	}
	return ""
}
