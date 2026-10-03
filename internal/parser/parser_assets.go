package parser

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/parser/validate"
)

// embedDirective declares the directory the files are read from.
const embedDirective = "//go:embed"

// embedFSType is the type an assets variable must have.
const embedFSType = "embed.FS"

var (
	ErrAssetsMultiple = errors.New(
		"multiple embed.FS variables declare an assets URL prefix",
	)
	ErrAssetsMissingEmbed = errors.New(
		"assets variable has no " + embedDirective + " directive",
	)
	ErrAssetsEmbedPatterns = errors.New(
		embedDirective + " must name exactly one directory",
	)
	ErrAssetsEmbedOutside = errors.New(
		embedDirective + " must name a directory inside the app package",
	)
)

// collectAssets reads the static file serving of the app package.
//
// An embed.FS variable whose doc comment names a URL path turns serving on,
// the same way a page type's doc comment names its route:
//
//	// AssetsFS is /static/
//	//go:embed static/*
//	var AssetsFS embed.FS
//
// The comment gives the URL prefix, the directive gives the directory.
// A package without such a variable serves no files.
func collectAssets(ctx *parseCtx, errs *Errors) {
	var found *ast.ValueSpec
	var foundName *ast.Ident

	for _, f := range ctx.pkg.Syntax {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				name := assetsVarName(ctx, vs, g)
				if name == nil {
					continue
				}
				if found != nil {
					errs.ErrAt(ctx.pkg.Fset.Position(name.Pos()), ErrAssetsMultiple)
					return
				}
				found, foundName = vs, name
				// A var block shares its doc with every spec in it,
				// hence the doc is read from the spec when it has one.
				if vs.Doc == nil && g.Doc != nil {
					found = &ast.ValueSpec{
						Doc: g.Doc, Names: vs.Names, Type: vs.Type, Values: vs.Values,
					}
				}
			}
		}
	}
	if found == nil {
		return
	}
	ctx.app.Assets = readAssets(ctx, errs, found, foundName)
}

// assetsVarName returns the name of an embed.FS variable whose doc comment
// declares a URL prefix, or nil when the spec is not one.
func assetsVarName(ctx *parseCtx, vs *ast.ValueSpec, g *ast.GenDecl) *ast.Ident {
	if len(vs.Names) != 1 {
		return nil
	}
	name := vs.Names[0]
	obj := ctx.pkg.TypesInfo.Defs[name]
	v, ok := obj.(*types.Var)
	if !ok || v.Type().String() != embedFSType {
		return nil
	}
	doc := vs.Doc
	if doc == nil {
		doc = g.Doc
	}
	if _, ok := urlPrefixOf(doc, name.Name); !ok {
		return nil
	}
	return name
}

// readAssets reads the URL prefix and the directory out of the declaration.
func readAssets(
	ctx *parseCtx, errs *Errors, vs *ast.ValueSpec, name *ast.Ident,
) model.Assets {
	pos := ctx.pkg.Fset.Position(name.Pos())
	prefix, _ := urlPrefixOf(vs.Doc, name.Name)
	if err := validate.AssetsURLPrefix(prefix); err != nil {
		errs.ErrAt(pos, err)
		return model.Assets{}
	}
	// A go:embed pattern is relative to the directory of its source file.
	srcDir := filepath.Dir(pos.Filename)
	dir, err := embedDir(vs.Doc, srcDir)
	if err != nil {
		errs.ErrAt(pos, err)
		return model.Assets{}
	}
	return model.Assets{URLPrefix: prefix, Dir: dir}
}

// urlPrefixOf reads the URL path out of the doc comment of an assets variable.
func urlPrefixOf(doc *ast.CommentGroup, name string) (string, bool) {
	if doc == nil || len(doc.List) == 0 {
		return "", false
	}
	rest, ok := validate.CutEventIsPrefix(cleanComment(doc.List[0].Text), name)
	if !ok || !strings.HasPrefix(rest, "/") {
		return "", false
	}
	// Only the path is the declaration, anything after it is prose.
	path, _, _ := strings.Cut(rest, " ")
	return path, true
}

// embedDir reads the directory out of the go:embed directive of the variable.
// srcDir is the directory the pattern is relative to.
//
// Exactly one directory is named. The generator serves it from the embed.FS in
// production and from disk in dev mode, both of which need one root.
// A glob such as static/*.css or a single file names none: used as the root,
// it is a directory that doesn't exist, and every asset answers 404.
func embedDir(doc *ast.CommentGroup, srcDir string) (string, error) {
	for _, c := range doc.List {
		rest, ok := strings.CutPrefix(c.Text, embedDirective)
		if !ok {
			continue
		}
		patterns := strings.Fields(rest)
		if len(patterns) != 1 {
			return "", ErrAssetsEmbedPatterns
		}
		p := patterns[0]
		if unquoted, err := strconv.Unquote(p); err == nil {
			p = unquoted
		}
		// A pattern addresses the files, the generator needs their root.
		p = strings.TrimPrefix(p, "all:")
		p = strings.TrimSuffix(p, "/*")
		switch {
		case p == "" || p == "." || strings.HasPrefix(p, "/"):
			return "", ErrAssetsEmbedOutside
		case strings.HasPrefix(p, "../") || strings.Contains(p, "/../"):
			return "", ErrAssetsEmbedOutside
		case strings.ContainsAny(p, "*?["):
			return "", ErrAssetsEmbedPatterns
		}
		if info, err := os.Stat(filepath.Join(srcDir, p)); err == nil && !info.IsDir() {
			return "", ErrAssetsEmbedPatterns
		}
		return p, nil
	}
	return "", ErrAssetsMissingEmbed
}

// cleanComment strips the comment marker and the space around it.
func cleanComment(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "//"))
}
