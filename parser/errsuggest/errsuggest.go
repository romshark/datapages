// Package errsuggest provides fix suggestions for parser errors.
package errsuggest

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/internal/paramvalidation"
)

// toSnakeCase converts a PascalCase or camelCase Go identifier to snake_case.
// Examples: "UserID" → "user_id", "CreatedAt" → "created_at", "HTTPStatus" → "http_status".
func toSnakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				next := rune(0)
				if i+1 < len(runes) {
					next = runes[i+1]
				}
				if unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next)) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Suggest returns an optional fix hint for a parser error, or "" if none is available.
// The hint is formatted as a short "fix: ..." line meant to be printed after the error.
func Suggest(err error) string {
	switch {
	case errors.Is(err, parser.ErrPageMissingFieldApp):
		var d *parser.ErrorPageMissingFieldApp
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add field `App *App` to %s", d.TypeName)

	case errors.Is(err, parser.ErrPageMissingPathComm):
		var d *parser.ErrorPageMissingPathComm
		if !errors.As(err, &d) {
			return ""
		}
		path := pageTypePath(d.TypeName)
		return fmt.Sprintf("fix: Add `// %s is %s`", d.TypeName, path)

	case errors.Is(err, parser.ErrActionMissingPathComm):
		var d *parser.ErrorActionMissingPathComm
		if !errors.As(err, &d) {
			return ""
		}
		suffix := methodPathSuffix(d.MethodName)
		base := "/"
		if d.PagePath != "" && d.PagePath != "/" {
			base = cleanPath(d.PagePath) + "/"
		}
		path := base + suffix
		return fmt.Sprintf("fix: Add `// %s is %s`", d.MethodName, path)

	case errors.Is(err, parser.ErrActionPathNotUnderPage):
		var d *parser.ErrorActionPathNotUnderPage
		if !errors.As(err, &d) {
			return ""
		}
		suffix := methodPathSuffix(d.MethodName)
		path := cleanPath(d.PagePath) + "/" + suffix
		return fmt.Sprintf("fix: Use `// %s is %s`", d.MethodName, path)

	case errors.Is(err, parser.ErrPageMissingGET):
		var d *parser.ErrorPageMissingGET
		if !errors.As(err, &d) {
			return ""
		}
		recv := strings.ToLower(string(d.TypeName[0]))
		return fmt.Sprintf(
			"fix: Add `func (%s *%s) GET(r *http.Request) (body templ.Component, err error) {}`",
			recv, d.TypeName,
		)

	case errors.Is(err, parser.ErrPageInvalidPathComm):
		var d *parser.ErrorPageInvalidPathComm
		if !errors.As(err, &d) {
			return ""
		}
		path := pageTypePath(d.TypeName)
		return fmt.Sprintf(
			"fix: First doc comment line must be `// %s is %s`; if there are more lines, the next must be an empty `//`",
			d.TypeName, path,
		)

	case errors.Is(err, parser.ErrActionInvalidPathComm):
		var d *parser.ErrorActionInvalidPathComm
		if !errors.As(err, &d) {
			return ""
		}
		suffix := methodPathSuffix(d.MethodName)
		return fmt.Sprintf(
			"fix: First doc comment line must be `// %s is /%s`; if there are more lines, the next must be an empty `//`",
			d.MethodName, suffix,
		)

	case errors.Is(err, parser.ErrEventCommMissing):
		var d *parser.ErrorEventCommMissing
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf(`fix: Add `+"`"+`// %s is "subject"`+"`"+` as the first doc comment line`, d.TypeName)

	case errors.Is(err, parser.ErrEventCommInvalid):
		var d *parser.ErrorEventCommInvalid
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf(
			`fix: First doc comment line must be `+
				"`"+`// %s is "subject"`+"`"+
				`; if there are more lines, the next must be an empty `+"`//`",
			d.TypeName,
		)

	case errors.Is(err, parser.ErrPathFieldMissingTag):
		var d *paramvalidation.ErrorPathFieldMissingTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add `path:\"%s\"` struct tag to field %s",
			toSnakeCase(d.FieldName), d.FieldName)

	case errors.Is(err, parser.ErrPathFieldEmptyTag):
		var d *paramvalidation.ErrorPathFieldEmptyTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add a non-empty name to the path tag of field %s, e.g. `path:\"%s\"`",
			d.FieldName, toSnakeCase(d.FieldName))

	case errors.Is(err, parser.ErrQueryFieldMissingTag):
		var d *paramvalidation.ErrorQueryFieldMissingTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add `query:\"%s\"` struct tag to field %s",
			toSnakeCase(d.FieldName), d.FieldName)

	case errors.Is(err, parser.ErrQueryFieldEmptyTag):
		var d *paramvalidation.ErrorQueryFieldEmptyTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add a non-empty name to the query tag of field %s, e.g. `query:\"%s\"`",
			d.FieldName, toSnakeCase(d.FieldName))

	case errors.Is(err, parser.ErrSignalsFieldMissingTag):
		var d *paramvalidation.ErrorSignalsFieldMissingTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add `json:\"%s\"` struct tag to field %s",
			toSnakeCase(d.FieldName), d.FieldName)

	case errors.Is(err, parser.ErrSignalsFieldEmptyTag):
		var d *paramvalidation.ErrorSignalsFieldEmptyTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add a non-empty name to the json tag of field %s, e.g. `json:\"%s\"`",
			d.FieldName, toSnakeCase(d.FieldName))

	case errors.Is(err, parser.ErrEventFieldMissingTag):
		var d *parser.ErrorEventFieldMissingTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Add `json:\"%s\"` struct tag to field %s",
			toSnakeCase(d.FieldName), d.FieldName)

	case errors.Is(err, parser.ErrEventFieldEmptyTag):
		var d *parser.ErrorEventFieldEmptyTag
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf(
			"fix: Add a non-empty name to the json tag of field %s, e.g. `json:\"%s\"`",
			d.FieldName, toSnakeCase(d.FieldName))

	case errors.Is(err, parser.ErrEventTargetUserIDsNoSession):
		var d *parser.ErrorEventTargetUserIDsNoSession
		if !errors.As(err, &d) {
			return ""
		}
		return fmt.Sprintf("fix: Define a Session type in package %s", d.PkgName)
	}
	return ""
}

// Errors intentionally excluded from Suggest — the error message already states
// the fix, or there is no specific context available to produce a useful hint:
//
//   - ErrAppMissingTypeApp            — message names the required type
//   - ErrAppMissingPageIndex          — message names the required page type
//   - ErrSignatureMissingReq          — message names *http.Request
//   - ErrSignatureMultiErrRet         — message states to remove the duplicate
//   - ErrSignatureUnknownInput        — no information about which parameter is wrong
//   - ErrSignatureSecondArgNotSSE     — message names the exact required type
//   - ErrSignatureEvHandReturnMustBeError — message names the required return type
//   - ErrSignatureEvHandFirstArgNotEvent — message states the required parameter name
//   - ErrSignatureEvHandFirstArgTypeNotEvent — no specific valid type to suggest
//   - ErrSignatureGETMissingBody      — message states "return body templ.Component"
//   - ErrSignatureGETBodyWrongName    — message states to name it "body"
//   - ErrSignatureGETHeadWrongName    — message states to name it "head"
//   - ErrPageHasExtraFields           — message states to remove the fields
//   - ErrPageConflictingGETEmbed      — message names the conflicting embedded types
//   - ErrPageNameInvalid              — naming rule is clear from valid examples
//   - ErrActionNameMissing            — message states a name is required
//   - ErrActionNameInvalid            — naming rule is clear from valid examples
//   - ErrEventSubjectInvalid          — message states subject must be a quoted string
//   - ErrEvHandDuplicate              — message identifies the duplicate handler
//   - ErrEvHandDuplicateEmbed         — message identifies the conflicting embeds
//   - ErrEventFieldUnexported         — fix is obvious: capitalize the field name
//   - ErrEventFieldDuplicateTag       — message names the duplicate value
//   - ErrPathParamNotStruct           — type constraint is clear from message
//   - ErrPathFieldUnexported          — fix is obvious: capitalize the field name
//   - ErrPathFieldDuplicateTag        — message names the duplicate value
//   - ErrPathFieldNotString           — type constraint is clear from message
//   - ErrPathFieldNotInRoute          — message names the tag value missing from route
//   - ErrPathMissingRouteVar          — message names the route variable without a field
//   - ErrQueryParamNotStruct          — type constraint is clear from message
//   - ErrQueryFieldUnexported         — fix is obvious: capitalize the field name
//   - ErrQueryFieldDuplicateTag       — message names the duplicate value
//   - ErrQueryReflectSignalNotInSignals — message names the missing signal
//   - ErrSignalsParamNotStruct        — type constraint is clear from message
//   - ErrSignalsFieldUnexported       — fix is obvious: capitalize the field name
//   - ErrSignalsFieldDuplicateTag     — message names the duplicate value
//   - ErrDispatchParamNotFunc         — type constraint is clear from message
//   - ErrDispatchReturnCount          — return constraint is clear from message
//   - ErrDispatchMustReturnError      — type constraint is clear from message
//   - ErrDispatchNoParams             — constraint is clear from message
//   - ErrDispatchParamNotEvent        — constraint is clear from message
//   - ErrSessionNotStruct             — type constraint is clear from message
//   - ErrSessionMissingUserID         — message names the required field and type
//   - ErrSessionMissingIssuedAt       — message names the required field and type
//   - ErrSessionParamNotSessionType   — constraint is clear from message
//   - ErrSessionTokenParamNotString   — constraint is clear from message
//   - ErrRedirectNotString            — constraint is clear from message
//   - ErrRedirectStatusNotInt         — constraint is clear from message
//   - ErrRedirectStatusWithoutRedirect — message states "requires redirect"
//   - ErrNewSessionNotSessionType     — constraint is clear from message
//   - ErrCloseSessionNotBool          — constraint is clear from message
//   - ErrNewSessionWithSSE            — message states the mutual exclusion
//   - ErrCloseSessionWithSSE          — message states the mutual exclusion
//   - ErrEnableBgStreamNotBool        — constraint is clear from message
//   - ErrEnableBgStreamNotGET         — message states it must be in a GET handler
//   - ErrDisableRefreshNotBool        — constraint is clear from message
//   - ErrDisableRefreshNotGET         — message states it must be in a GET handler

// pageTypePath derives a suggested route path from a page type name.
// "PageIndex" -> "/", "PageProfile" -> "/profile/", "PageFooBar" -> "/foobar/".
func pageTypePath(typeName string) string {
	suffix, ok := strings.CutPrefix(typeName, "Page")
	if !ok || suffix == "" {
		return "/"
	}
	if suffix == "Index" {
		return "/"
	}
	return "/" + strings.ToLower(suffix) + "/"
}

// methodPathSuffix derives a suggested URL path segment from an action method name
// by stripping the HTTP method prefix and lowercasing the remainder.
// E.g., "POSTFoo" -> "foo", "DELETEFooBar" -> "foobar".
func methodPathSuffix(method string) string {
	for _, prefix := range []string{"DELETE", "PATCH", "POST", "PUT", "GET"} {
		if after, ok := strings.CutPrefix(method, prefix); ok && after != "" {
			return strings.ToLower(after)
		}
	}
	return strings.ToLower(method)
}

func cleanPath(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}
