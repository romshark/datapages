// Package skeleton provides templates for initializing a new Datapages project.
package skeleton

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"text/template"
)

//go:embed main.go.tmpl
var mainGoTmpl string

//go:embed app.go.tmpl
var AppGo string

//go:embed app.templ.tmpl
var AppTempl string

//go:embed compose.yaml.tmpl
var ComposeYAML string

//go:embed Makefile.tmpl
var Makefile string

//go:embed vscode-extensions.json.tmpl
var VSCodeExtensions string

//go:embed ci.yml.tmpl
var CIWorkflow string

var tmpl = template.Must(template.New("main.go").Parse(mainGoTmpl))

type mainGoData struct {
	AppImport  string
	GenImport  string
	Gen        string
	Prometheus bool
	HasSession bool

	// SessionData is the rendered session Data type the session manager is
	// instantiated with, for example "struct{}" or "app.SessionData".
	SessionData string
}

// MainGo renders the cmd/server/main.go template with the given import paths
// and returns formatted Go source.
//
// sessionData is the rendered session Data type,
// empty for an application without sessions.
func MainGo(
	appImportPath, genImportPath, genPkgName string,
	prometheus bool, sessionData string,
) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, mainGoData{
		AppImport:   appImportPath,
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
