package skeleton

import _ "embed"

// GeneratedCode is how a scaffolded project keeps the code that templ generate
// and datapages gen write: *_templ.go and the datapagesgen packages.
type GeneratedCode string

const (
	// GeneratedCodeCommitted checks the generated code into git.
	// CI fails when it differs from what the generators write.
	GeneratedCodeCommitted GeneratedCode = "committed"

	// GeneratedCodeHidden lists the generated code in .gitignore and hides it
	// in VS Code. CI generates it before building.
	GeneratedCodeHidden GeneratedCode = "hidden"
)

// GeneratedCodeChoices lists every choice in the order init offers them.
var GeneratedCodeChoices = []GeneratedCode{GeneratedCodeCommitted, GeneratedCodeHidden}

// ParseGeneratedCode returns the choice named s.
func ParseGeneratedCode(s string) (GeneratedCode, error) {
	return parseChoice("generated code setting", GeneratedCodeChoices, s)
}

// GeneratedCodeIgnore lists the .gitignore entries of [GeneratedCodeHidden].
// Without a leading slash they match at any depth, which covers every app of the module.
var GeneratedCodeIgnore = []string{"datapagesgen/", "*_templ.go"}

// VSCodeSettingsHidden hides the files of [GeneratedCodeIgnore] in VS Code.
//
//go:embed vscode-settings.json.tmpl
var VSCodeSettingsHidden string
