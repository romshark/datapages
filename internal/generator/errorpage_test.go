package generator_test

import (
	"go/ast"
	goparser "go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
)

// TestError500PageReportsWithoutRenderingItself tests the helper the generated
// PageError500 handler reports its own failure through.
//
// httpErrIntern answers a failed page load by rendering PageError500.
// An error page reporting through it answers its own failure by rendering itself,
// and that pair runs until the stack is gone, which ends the process rather than
// the request. The handler reports through httpErrFinal for a returned error
// and through recoverPanicFinal for a panic, both of which write the status
// and nothing else.
//
// internal/acceptance/error500failing asserts what a visitor gets.
// It runs in the test binary, where the recursion this guards against
// overflows the stack and takes every other test in the package with it.
// This reads the generated source instead and names what changed.
func TestError500PageReportsWithoutRenderingItself(t *testing.T) {
	t.Parallel()

	m, errs := parser.Parse(filepath.Join("..", "parser", "testdata", "basic"))
	require.Zero(t, errs.Len(), errs.Error())
	require.NotNil(t, m.PageError500, "the fixture declares no error page")

	dst := t.TempDir()
	require.NoError(t, generator.Generate(
		dst, "datapagesgen", m, 0o644,
		generator.Options{GenImport: "datapagestest/x/datapagesgen"},
	))
	src, err := os.ReadFile(filepath.Join(dst, "app_gen.go"))
	require.NoError(t, err)

	handler := pageGETHandler(t, string(src), m.PageError500.TypeName)
	require.Contains(t, handler, "s.httpErrFinal(",
		"the error page reports through something other than httpErrFinal")
	require.Contains(t, handler, "s.recoverPanicFinal(",
		"a panic in the error page reports through something "+
			"other than recoverPanicFinal")
	require.NotContains(t, handler, "s.httpErrIntern(",
		"the error page reports through the helper that renders it, "+
			"which renders it again for as long as the stack lasts")
	require.NotContains(t, handler, "s.recoverPanic(",
		"a panic in the error page reports through the helper that renders it, "+
			"which renders it again for as long as the stack lasts")
}

// pageGETHandler returns the source of the generated GET handler of pageType,
// found by the page value it constructs. The receiver type the generator
// derives from the page name is not part of the model, which is why this
// searches for the construction instead of naming the method.
func pageGETHandler(t *testing.T, src, pageType string) string {
	t.Helper()

	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_gen.go", src, 0)
	require.NoError(t, err)

	constructs := "." + pageType + "{"
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || fd.Name.Name != "GET" {
			continue
		}
		var b strings.Builder
		require.NoError(t, printer.Fprint(&b, fset, fd))
		if strings.Contains(b.String(), constructs) {
			return b.String()
		}
	}
	t.Fatalf("no generated GET handler constructs %s", pageType)
	return ""
}
