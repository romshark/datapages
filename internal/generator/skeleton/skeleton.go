// Package skeleton provides templates for initializing a new Datapages project.
package skeleton

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

//go:embed main.go.tmpl
var mainGoTmpl string

//go:embed app.go.tmpl
var appGoTmpl string

//go:embed app.templ.tmpl
var appTemplTmpl string

//go:embed compose.yaml.tmpl
var ComposeYAML string

//go:embed Makefile.tmpl
var makefileTmpl string

//go:embed vscode-extensions.json.tmpl
var VSCodeExtensions string

//go:embed ci.yml.tmpl
var ciWorkflowTmpl string

// TemplVersion is the templ release the scaffold generates with.
// Pinning it keeps 2 runs of `datapages init` a month apart on the same generator,
// and keeps the CI of a scaffolded project on the version its committed
// *_templ.go files were written by. Move it together with toolTempl in magefiles,
// which is the version this repository generates with.
const TemplVersion = "v0.3.1070"

// TemplCmd is the templ command with its version, for `go run` and `go install`.
const TemplCmd = "github.com/a-h/templ/cmd/templ@" + TemplVersion

var ciWorkflow = template.Must(template.New("ci.yml").Parse(ciWorkflowTmpl))

// datapagesModule is the module the CLI is installed from.
const datapagesModule = "github.com/romshark/datapages/cmd/datapages"

// CIWorkflow renders the GitHub Actions workflow of a scaffolded project.
//
// modVersion is the version in the scaffolded go.mod, including the leading
// "v". An empty value selects latest. The workflow installs the same generator
// version because it checks committed datapagesgen output.
// Source builds use a pseudo-version.
func CIWorkflow(modVersion string) (string, error) {
	datapagesCmd := datapagesModule + "@latest"
	if modVersion != "" {
		datapagesCmd = datapagesModule + "@" + modVersion
	}
	data := struct{ TemplCmd, DatapagesCmd string }{TemplCmd, datapagesCmd}
	var buf bytes.Buffer
	if err := ciWorkflow.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing ci.yml template: %w", err)
	}
	return buf.String(), nil
}

var (
	tmpl       = template.Must(template.New("main.go").Parse(mainGoTmpl))
	appGoTempl = template.Must(template.New("app.go").Parse(appGoTmpl))
	// Templ writes Go expressions in single braces, [[ ]] keeps them apart.
	appTempl = template.Must(
		template.New("app.templ").Delims("[[", "]]").Parse(appTemplTmpl),
	)
	makefile = template.Must(
		template.New("Makefile").Parse(makefileTmpl),
	)
)

// parseChoice returns the choice named s. what describes the choice in the error.
func parseChoice[T ~string](what string, choices []T, s string) (T, error) {
	if c := T(s); slices.Contains(choices, c) {
		return c, nil
	}
	names := make([]string, len(choices))
	for i, c := range choices {
		names[i] = string(c)
	}
	return "", fmt.Errorf("unknown %s %q, want one of %s",
		what, s, strings.Join(names, ", "))
}

// AppGo renders the app/app.go skeleton and returns formatted Go source.
// With Web Components it declares the static files and a head that loads them.
func AppGo(w WebComponents) ([]byte, error) {
	var buf bytes.Buffer
	data := struct{ Components bool }{w != WebComponentsNone}
	if err := appGoTempl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing app.go template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting app.go: %w", err)
	}
	return src, nil
}

// AppTempl renders the app/app.templ skeleton. With Web Components it defines
// the head [AppGo] returns and shows the example component on the welcome page.
//
// assetsImport is the import path of the generated assets package,
// which the head refers to the entry of the components through. Before the first
// generation the package does not exist, and the app package would not type-check.
// An empty assetsImport writes the head without the entry.
func AppTempl(w WebComponents, assetsImport string) ([]byte, error) {
	var buf bytes.Buffer
	data := struct {
		Components   bool
		AssetsImport string
		Script, Dir  string
	}{
		Components:   w != WebComponentsNone,
		AssetsImport: assetsImport,
		Script:       componentsScript(w),
		Dir:          ComponentsDir(w),
	}
	if w == WebComponentsNone {
		data.AssetsImport = ""
	}
	if err := appTempl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing app.templ template: %w", err)
	}
	return buf.Bytes(), nil
}

// Makefile renders the Makefile skeleton. For [WebComponentsTypeScript] it
// installs the npm packages and bundles the components before dev and build.
func Makefile(w WebComponents) (string, error) {
	var buf bytes.Buffer
	data := struct{ TypeScript bool }{w == WebComponentsTypeScript}
	if err := makefile.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing Makefile template: %w", err)
	}
	return buf.String(), nil
}

// mainGoImports maps every import main.go.tmpl writes to the identifier it
// binds, which is not always the last path element: nats.go binds "nats".
//
// TestMainGoImportsAreKnown fails when the template imports something this
// table does not list.
var mainGoImports = map[string]string{
	"bufio":                         "bufio",
	"context":                       "context",
	"encoding/hex":                  "hex",
	"errors":                        "errors",
	"fmt":                           "fmt",
	"io/fs":                         "fs",
	"log/slog":                      "slog",
	"net":                           "net",
	"net/http":                      "http",
	"os":                            "os",
	"os/signal":                     "signal",
	"strings":                       "strings",
	"syscall":                       "syscall",
	"github.com/romshark/datapages": "datapages",
	"github.com/romshark/datapages/modules/messaging/natscore": "natscore",
	"github.com/romshark/datapages/modules/sessions":           "sessions",
	"github.com/romshark/datapages/modules/sessions/natskv":    "natskv",
	"github.com/nats-io/nats.go":                               "nats",
}

// MainGoImportIdents returns the imports main.go carries, mapped to the identifier
// each binds. It exists for the test that keeps the table in step with the template.
func MainGoImportIdents() map[string]string {
	return maps.Clone(mainGoImports)
}

// MainGoTaken is every identifier main.go binds before the session data type
// brings a package of its own along. A package of that name needs an alias,
// or 1 identifier would name 2 packages.
func MainGoTaken(appPkgName, genPkgName string) map[string]bool {
	taken := make(map[string]bool, len(mainGoImports)+3)
	for _, id := range mainGoImports {
		taken[id] = true
	}
	taken[genPkgName] = true
	appPkg, _ := mainGoAppPkg(appPkgName, genPkgName)
	taken[appPkg] = true
	return taken
}

// MainGoAppPkg reports the identifier main.go refers to the app package by
// and whether the import carries it as an alias.
func MainGoAppPkg(appPkgName, genPkgName string) (name string, aliased bool) {
	return mainGoAppPkg(appPkgName, genPkgName)
}

// mainGoAppPkg reports the identifier main.go refers to
// the app package by and whether the import carries it as an alias.
//
// The declared name is kept when nothing else in the file binds it, which is
// the ordinary case. An app package named after one of the imports takes an
// alias instead: unaliased, 1 identifier would bind 2 packages.
func mainGoAppPkg(appPkgName, genPkgName string) (name string, aliased bool) {
	taken := make(map[string]bool, len(mainGoImports)+1)
	for _, id := range mainGoImports {
		taken[id] = true
	}
	taken[genPkgName] = true
	if !taken[appPkgName] {
		return appPkgName, false
	}
	for n := 0; ; n++ {
		alias := "dpapp"
		if n > 0 {
			alias += strconv.Itoa(n + 1)
		}
		if !taken[alias] {
			return alias, true
		}
	}
}

// Import is a package main.go imports for the session data type,
// under the identifier that file qualifies it by.
type Import struct {
	Path    string
	Ident   string
	Aliased bool
}

type mainGoData struct {
	AppImport  string
	AppPkg     string
	AppAliased bool
	GenImport  string
	Gen        string
	Prometheus bool
	HasSession bool

	// SessionData is the rendered session Data type the session manager is
	// instantiated with, for example "struct{}" or "app.SessionData".
	SessionData string
	// ExtraImports are the packages SessionData names beyond the app package.
	ExtraImports []Import

	AssetsVar  string
	DatastarJS string
}

// MainGoConfig is what [MainGo] renders cmd/server/main.go from.
type MainGoConfig struct {
	// AppImport and GenImport are the import paths of the app package and the
	// package generated for it. AppPkgName is the name the app package
	// declares, which need not match the last element of its import path.
	AppImport, AppPkgName, GenImport, GenPkgName string

	Prometheus bool

	// SessionData is the rendered session Data type, empty for an application
	// without sessions. ExtraImports are the packages it names beyond the app
	// package, which the caller renders it against: [MainGoTaken] is the set
	// they have to keep out of the way of.
	SessionData  string
	ExtraImports []Import

	// AssetsVar is the embed.FS variable of the app package that holds its
	// static files, empty for an application without them.
	AssetsVar string

	// DatastarJS is the URL of the Datastar bundle, empty for the default.
	DatastarJS string
}

// MainGo renders the cmd/server/main.go template and returns formatted Go source.
func MainGo(c MainGoConfig) ([]byte, error) {
	var buf bytes.Buffer
	appPkg, aliased := mainGoAppPkg(c.AppPkgName, c.GenPkgName)
	if err := tmpl.Execute(&buf, mainGoData{
		AppImport:    c.AppImport,
		AppPkg:       appPkg,
		AppAliased:   aliased,
		GenImport:    c.GenImport,
		Gen:          c.GenPkgName,
		Prometheus:   c.Prometheus,
		HasSession:   c.SessionData != "",
		SessionData:  c.SessionData,
		ExtraImports: c.ExtraImports,
		AssetsVar:    c.AssetsVar,
		DatastarJS:   c.DatastarJS,
	}); err != nil {
		return nil, fmt.Errorf("executing main.go template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting main.go: %w", err)
	}
	return src, nil
}
