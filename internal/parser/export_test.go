package parser

import (
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/parser/model"
)

// PosFromPackagesError is [posFromPackagesError] for the tests of package parser_test.
var PosFromPackagesError = posFromPackagesError

// LoadPackage exposes an uncached [loadPackage] to parser_test.
func LoadPackage(appPackagePath string) (*packages.Package, error) {
	pkg, _, err := loadPackage(appPackagePath, false, nil)
	return pkg, err
}

// NameConstrained is [nameConstrained] for the tests of package parser_test.
var NameConstrained = nameConstrained

// ParseLoad exposes cache hits and source retries to parser_test.
func ParseLoad(
	appPackagePath string, cached bool,
) (app *model.App, errs Errors, hit, retried bool) {
	app, errs, r := parseLoad(appPackagePath, cached)
	return app, errs, r.hit, r.retried
}
