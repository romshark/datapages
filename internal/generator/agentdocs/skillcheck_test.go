package agentdocs_test

import (
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

var (
	helperSelector = regexp.MustCompile(`\b(action|href)\.[A-Za-z_][A-Za-z_0-9.]*`)
	assetsCall     = regexp.MustCompile(`datapages\.WithAssets\(([^)]*)\)`)
)

// TestSkillExamplesResolve checks the helper names in code examples against
// the generated API used by the classifieds app.
func TestSkillExamplesResolve(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	appDir := filepath.Join(root, "example", "classifieds")
	pkgs, err := packages.Load(&packages.Config{
		Dir:  appDir,
		Mode: packages.NeedName | packages.NeedTypes,
	}, "./app/datapagesgen/action", "./app/datapagesgen/href",
		"github.com/romshark/datapages")
	require.NoError(t, err)
	require.Len(t, pkgs, 3)
	byName := make(map[string]*types.Package, len(pkgs))
	for _, pkg := range pkgs {
		require.Empty(t, pkg.Errors)
		byName[pkg.Name] = pkg.Types
	}
	withAssets := byName["datapages"].Scope().Lookup("WithAssets")
	require.NotNil(t, withAssets)
	withAssetsArity := withAssets.Type().(*types.Signature).Params().Len()

	paths, err := filepath.Glob(filepath.Join("data", "skills", "*", "SKILL.md"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		inCode := false
		for line := range strings.SplitSeq(string(b), "\n") {
			if strings.HasPrefix(line, "```") {
				if inCode {
					inCode = false
				} else {
					inCode = line == "```go" || line == "```templ"
				}
				continue
			}
			if !inCode {
				continue
			}
			for _, selector := range helperSelector.FindAllString(line, -1) {
				parts := strings.Split(selector, ".")
				pkg := byName[parts[0]]
				require.NotNil(t, pkg, "%s: %s", path, selector)
				obj := pkg.Scope().Lookup(parts[1])
				require.NotNil(t, obj, "%s: %s", path, selector)
				for _, name := range parts[2:] {
					obj, _, _ = types.LookupFieldOrMethod(obj.Type(), false, pkg, name)
					require.NotNil(t, obj, "%s: %s", path, selector)
				}
			}
			for _, match := range assetsCall.FindAllStringSubmatch(line, -1) {
				require.Len(t, strings.Split(match[1], ","), withAssetsArity,
					"%s: WithAssets requires a filesystem and browsable flag", path)
			}
		}
	}
}
