package generator_test

import (
	"go/format"
	"os"
	"path/filepath"
	"testing"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser"
	"github.com/stretchr/testify/require"
)

func TestGenerateClassifieds(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "example", "classifieds", "app"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, app, 0o644, nil)
	require.NoError(t, err)

	compareFile(t, "app_gen.go",
		filepath.Join(tmpDir, "app_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "app_gen.go"))
	compareFile(t, "action/action_gen.go",
		filepath.Join(tmpDir, "action", "action_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "action", "action_gen.go"))
	compareFile(t, "href/href_gen.go",
		filepath.Join(tmpDir, "href", "href_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "href", "href_gen.go"))
}

func compareFile(t *testing.T, name, gotPath, wantPath string) {
	t.Helper()

	got, err := os.ReadFile(gotPath)
	require.NoError(t, err, "reading generated %s", name)

	want, err := os.ReadFile(wantPath)
	require.NoError(t, err, "reading reference %s", name)

	// Format both to normalize whitespace differences.
	gotFmt, err := format.Source(got)
	if err != nil {
		// If formatting fails, compare raw.
		gotFmt = got
	}
	wantFmt, err := format.Source(want)
	if err != nil {
		wantFmt = want
	}

	if string(gotFmt) != string(wantFmt) {
		t.Errorf("%s differs from reference.\nGenerated: %s\nReference: %s",
			name, gotPath, wantPath)
	}
}
