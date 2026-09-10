// Package skeleton provides templates for initializing a new Datapages project.
package skeleton

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"maps"
	"strconv"
	"text/template"
)

//go:embed main.go.tmpl
var mainGoTmpl string

//go:embed app.go.tmpl
var appGoTmpl string

//go:embed app.templ.tmpl
var AppTempl string

//go:embed compose.yaml.tmpl
var ComposeYAML string

//go:embed Makefile.tmpl
var Makefile string

//go:embed vscode-extensions.json.tmpl
var VSCodeExtensions string

//go:embed ci.yml.tmpl
var ciWorkflowTmpl string

// TemplVersion is the templ release the scaffold generates with.
// Pinning it keeps two runs of `datapages init` a month apart on the same generator,
// and keeps the CI of a scaffolded project on the version its committed
// *_templ.go files were written by. Move it together with toolTempl in magefiles,
// which is the version this repository generates with.
const TemplVersion = "v0.3.1020"

// TemplCmd is the templ command with its version, for `go run` and `go install`.
const TemplCmd = "github.com/a-h/templ/cmd/templ@" + TemplVersion

var ciWorkflow = template.Must(template.New("ci.yml").Parse(ciWorkflowTmpl))

// datapagesModule is the module the CLI is installed from.
const datapagesModule = "github.com/romshark/datapages/cmd/datapages"

// CIWorkflow renders the GitHub Actions workflow of a scaffolded project.
//
// version is the release of the CLI doing the scaffolding, without the leading "v".
// The workflow installs that release, since the generator version decides
// what the committed datapagesgen holds and the workflow fails the build on a difference.
// A build from source carries no version and falls back to latest.
func CIWorkflow(version string) (string, error) {
	datapagesCmd := datapagesModule + "@latest"
	if version != "" {
		datapagesCmd = datapagesModule + "@v" + version
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
)

// AppGo renders the app/app.go skeleton and returns formatted Go source.
func AppGo() ([]byte, error) {
	var buf bytes.Buffer
	if err := appGoTempl.Execute(&buf, nil); err != nil {
		return nil, fmt.Errorf("executing app.go template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting app.go: %w", err)
	}
	return src, nil
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

// mainGoAppPkg reports the identifier main.go refers to
// the app package by and whether the import carries it as an alias.
//
// The declared name is kept when nothing else in the file binds it, which is
// the ordinary case. An app package named after one of the imports takes an
// alias instead: unaliased, one identifier would bind two packages.
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
}

// MainGo renders the cmd/server/main.go template with the given import paths
// and returns formatted Go source. appPkgName is the name the app package
// declares, which need not match the last element of its import path.
//
// sessionData is the rendered session Data type,
// empty for an application without sessions.
func MainGo(
	appImportPath, appPkgName, genImportPath, genPkgName string,
	prometheus bool, sessionData string,
) ([]byte, error) {
	var buf bytes.Buffer
	appPkg, aliased := mainGoAppPkg(appPkgName, genPkgName)
	if err := tmpl.Execute(&buf, mainGoData{
		AppImport:   appImportPath,
		AppPkg:      appPkg,
		AppAliased:  aliased,
		GenImport:   genImportPath,
		Gen:         genPkgName,
		Prometheus:  prometheus,
		HasSession:  sessionData != "",
		SessionData: sessionData,
	}); err != nil {
		return nil, fmt.Errorf("executing main.go template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting main.go: %w", err)
	}
	return src, nil
}
