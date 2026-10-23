// Package buildconstraint reports whether a //go:build line excludes
// a file from every build.
package buildconstraint

import (
	"go/build/constraint"
	"slices"
)

// NeverBuilt reports whether x excludes a file from every build.
// No build sets "ignore". With more than 16 tags it reports false.
func NeverBuilt(x constraint.Expr) bool {
	var tags []string
	var collect func(constraint.Expr)
	collect = func(x constraint.Expr) {
		switch x := x.(type) {
		case *constraint.TagExpr:
			if x.Tag != "ignore" && !slices.Contains(tags, x.Tag) {
				tags = append(tags, x.Tag)
			}
		case *constraint.NotExpr:
			collect(x.X)
		case *constraint.AndExpr:
			collect(x.X)
			collect(x.Y)
		case *constraint.OrExpr:
			collect(x.X)
			collect(x.Y)
		}
	}
	collect(x)
	if len(tags) > 16 {
		return false
	}
	for set := range 1 << len(tags) {
		if x.Eval(func(tag string) bool {
			i := slices.Index(tags, tag)
			return i >= 0 && set&(1<<i) != 0
		}) {
			return false
		}
	}
	return true
}
