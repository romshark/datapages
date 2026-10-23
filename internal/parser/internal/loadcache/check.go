package loadcache

import (
	"errors"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"go/types"
	"log"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// errIncomplete triggers a fallback when cached types cannot satisfy an import.
var errIncomplete = errors.New("import without complete types")

// checkSource reproduces go/packages parsing and type checking under [Mode].
// It records parse errors, type errors and warn in that order.
func checkSource(
	p *packages.Package, parse ParseFunc, sizes types.Sizes, warn string,
) error {
	appendError := func(err error) {
		var errs []packages.Error
		switch err := err.(type) {
		case packages.Error:
			errs = append(errs, err)
		case *os.PathError:
			errs = append(errs, packages.Error{
				Pos:  err.Path + ":1",
				Msg:  err.Err.Error(),
				Kind: packages.ParseError,
			})
		case scanner.ErrorList:
			for _, err := range err {
				errs = append(errs, packages.Error{
					Pos:  err.Pos.String(),
					Msg:  err.Msg,
					Kind: packages.ParseError,
				})
			}
		case types.Error:
			p.TypeErrors = append(p.TypeErrors, err)
			errs = append(errs, packages.Error{
				Pos:  err.Fset.Position(err.Pos).String(),
				Msg:  err.Msg,
				Kind: packages.TypeError,
			})
		default:
			errs = append(errs, packages.Error{
				Pos:  "-",
				Msg:  err.Error(),
				Kind: packages.UnknownError,
			})
			log.Printf("internal error: error %q (%T) without position", err, err)
		}
		p.Errors = append(p.Errors, errs...)
	}
	if warn != "" {
		defer func() {
			if len(p.Errors) > 0 {
				appendError(packages.Error{
					Pos: "-", Msg: warn,
					Kind: packages.UnknownError,
				})
			}
		}()
	}

	files, errs := parseFiles(p.Fset, p.CompiledGoFiles, parse)
	for _, err := range errs {
		appendError(err)
	}
	p.Syntax = files
	p.TypesInfo = newTypesInfo()

	var importErr error
	importer := importerFunc(func(path string) (*types.Package, error) {
		if path == "unsafe" {
			return types.Unsafe, nil
		}
		ipkg := p.Imports[path]
		if ipkg == nil {
			return nil, fmt.Errorf("no metadata for %s", path)
		}
		if ipkg.Types != nil && ipkg.Types.Complete() {
			return ipkg.Types, nil
		}
		importErr = fmt.Errorf("%w: %s imports %s", errIncomplete, p.PkgPath, path)
		return nil, importErr
	})
	tc := &types.Config{
		Importer: importer,
		Error:    appendError,
		Sizes:    sizes,
	}
	if p.Module != nil && p.Module.GoVersion != "" {
		tc.GoVersion = "go" + p.Module.GoVersion
	}
	typErr := types.NewChecker(tc, p.Fset, p.Types, p.TypesInfo).Files(p.Syntax)
	if importErr != nil {
		return importErr
	}
	if typErr != nil && len(p.Errors) == 0 && len(p.Syntax) > 0 {
		msg := typErr.Error()
		if strings.HasPrefix(msg, "package requires newer Go version") {
			appendError(types.Error{
				Fset: p.Fset,
				Pos:  p.Syntax[0].Package,
				Msg:  msg,
			})
		}
	}
	if typErr != nil && len(p.Errors) == 0 {
		appendError(typErr)
	}

	illTyped := len(p.Errors) > 0
	if !illTyped {
		for _, imp := range p.Imports {
			if imp.IllTyped {
				illTyped = true
				break
			}
		}
	}
	p.IllTyped = illTyped
	return nil
}

// parseFiles preserves the file and error order returned by go/packages.
func parseFiles(
	fset *token.FileSet, filenames []string, parse ParseFunc,
) ([]*ast.File, []error) {
	var files []*ast.File
	var errs []error
	for _, filename := range filenames {
		src, err := os.ReadFile(filename)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f, err := parse(fset, filename, src)
		if f != nil {
			files = append(files, f)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return files, errs
}

func newTypesInfo() *types.Info {
	return &types.Info{
		Types:        make(map[ast.Expr]types.TypeAndValue),
		Defs:         make(map[*ast.Ident]types.Object),
		Uses:         make(map[*ast.Ident]types.Object),
		Implicits:    make(map[ast.Node]types.Object),
		Instances:    make(map[*ast.Ident]types.Instance),
		Scopes:       make(map[ast.Node]*types.Scope),
		Selections:   make(map[*ast.SelectorExpr]*types.Selection),
		FileVersions: make(map[*ast.File]string),
	}
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }
