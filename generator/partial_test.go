package generator_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/model"
)

// TestGeneratePartialModels runs the generator over the models the parser
// returns for applications it rejects.
//
// A partial model has holes a valid one never has. A page whose state type was
// rejected. An action whose handler failed to parse.
// Any caller holding one can pass it to Generate.
//
// Generate must refuse such a model and write nothing.
// The user is already reading their own error.
// A panic or a half-written package on top of it is noise they cannot act on.
//
// The parser's own tests assert the errors it reports for these fixtures.
// This test asserts only what the generator does with what comes back.
func TestGeneratePartialModels(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "parser", "testdata"))
	require.NoError(t, err)

	var found int
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "err_") {
			continue
		}
		found++
		fixture := e.Name()
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			app, errs := parser.Parse(
				filepath.Join("..", "parser", "testdata", fixture),
			)
			require.NotZero(t, errs.Len(),
				"the fixture parses cleanly and does not belong here")

			// The parser built no model at all. The generator writes stubs,
			// which keep the import resolving.
			if app == nil {
				dst := t.TempDir()
				require.NoError(t, generator.Generate(
					dst, "datapagesgen", nil, 0o644,
					generator.Options{GenImport: "datapagestest/x/datapagesgen"},
				))
				for _, f := range []string{
					"app_gen.go",
					filepath.Join("action", "action_gen.go"),
					filepath.Join("href", "href_gen.go"),
					filepath.Join("httperr", "httperr_gen.go"),
				} {
					_, err := os.Stat(filepath.Join(dst, f))
					require.NoError(t, err, "no stub written for %s", f)
				}
				return
			}

			dst := t.TempDir()
			err, panicked := generateRecovering(t, dst, app)
			require.False(t, panicked, "the generator panicked: %v", err)

			// An error is a fine answer. Writing files is not.
			// The destination holds what the last successful run produced.
			if err != nil {
				requireEmptyDir(t, dst)
				return
			}
		})
	}
	require.NotZero(t, found, "no err_ fixtures")
}

// generateRecovering runs the generator and
// reports whether it panicked instead of returning.
func generateRecovering(
	t *testing.T, dst string, app *model.App,
) (err error, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = &panicError{value: r}
		}
	}()
	err = generator.Generate(
		dst, "datapagesgen", app, 0o644,
		generator.Options{GenImport: "datapagestest/x/datapagesgen"},
	)
	return err, false
}

// requireEmptyDir asserts that the generator wrote nothing into dst.
func requireEmptyDir(t *testing.T, dst string) {
	t.Helper()
	var found []string
	require.NoError(t, filepath.WalkDir(dst,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				rel, err := filepath.Rel(dst, path)
				if err != nil {
					return err
				}
				found = append(found, rel)
			}
			return nil
		}))
	require.Empty(t, found, "the generator wrote files and then failed")
}

type panicError struct{ value any }

func (e *panicError) Error() string { return fmt.Sprintf("panic: %v", e.value) }
