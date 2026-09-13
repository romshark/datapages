package errsuggest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/errsuggest"
	"github.com/romshark/datapages/internal/parser/internal/paramvalidation"
)

// TestSuggest tests the "fix:" line appended to a parser error. Each sentinel maps to
// the code the developer has to write, a wrapped sentinel is matched the same,
// and an error with no suggestion produces an empty string rather than a guess.
func TestSuggest(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"nil":           {err: nil, want: ""},
		"unrelated":     {err: errors.New("some error"), want: ""},
		"sentinel only": {err: parser.ErrActionPathNotUnderPage, want: ""},

		"ErrAppMissingTypeApp": {
			err:  parser.ErrAppMissingTypeApp,
			want: "fix: Add `type App struct{}`",
		},
		"ErrAppMissingTypeApp/wrapped": {
			err:  fmt.Errorf("outer: %w", parser.ErrAppMissingTypeApp),
			want: "fix: Add `type App struct{}`",
		},

		"ErrAppMissingPageIndex": {
			err: parser.ErrAppMissingPageIndex,
			want: "fix: Add: \n" +
				"// PageIndex is /\n" +
				"type PageIndex struct{ App *App }\n" +
				"func (p PageIndex) GET(r *http.Request) " +
				"(body templ.Component, err error) { return nil, nil }",
		},

		"ErrSignatureMissingReq": {
			err:  parser.ErrSignatureMissingReq,
			want: "fix: Add `r *http.Request` parameter",
		},
		"ErrSignatureMissingReq/wrapped": {
			err:  fmt.Errorf("%w in PageFoo.GET", parser.ErrSignatureMissingReq),
			want: "fix: Add `r *http.Request` parameter",
		},

		"ErrSignatureGETMissingBody": {
			err:  parser.ErrSignatureGETMissingBody,
			want: "fix: Add `body templ.Component` to return values",
		},

		"ErrSignatureActionHeadWithoutBody": {
			err:  parser.ErrSignatureActionHeadWithoutBody,
			want: "fix: Add `body templ.Component` to return values, or drop the head",
		},

		"ErrSignatureEvHandMissingSSE": {
			err:  parser.ErrSignatureEvHandMissingSSE,
			want: "fix: Add `sse datapages.SSE` parameter",
		},
		"ErrSignatureEvHandMissingSSE/wrapped": {
			err: fmt.Errorf("%w: PageFoo.OnEventBar",
				parser.ErrSignatureEvHandMissingSSE),
			want: "fix: Add `sse datapages.SSE` parameter",
		},

		"ErrSignatureEvHandMultipleEvents": {
			err:  parser.ErrSignatureEvHandMultipleEvents,
			want: "fix: Keep one parameter of an EventXXX type and remove the rest",
		},
		"ErrSignatureEvHandMissingEvent": {
			err:  parser.ErrSignatureEvHandMissingEvent,
			want: "fix: Add a parameter of an EventXXX type",
		},
		"ErrSignatureEvHandMissingEvent/wrapped": {
			err: fmt.Errorf("%w: PageFoo.OnEventBar",
				parser.ErrSignatureEvHandMissingEvent),
			want: "fix: Add a parameter of an EventXXX type",
		},

		"ErrPageMissingFieldApp": {
			err:  &parser.PageMissingFieldAppError{TypeName: "PageProfile"},
			want: "fix: Add field `App *App` to PageProfile",
		},
		"ErrPageMissingFieldApp/wrapped": {
			err: fmt.Errorf("outer: %w", &parser.PageMissingFieldAppError{
				TypeName: "PageProfile",
			}),
			want: "fix: Add field `App *App` to PageProfile",
		},

		"ErrPageMissingPathComm/index": {
			err:  &parser.PageMissingPathCommError{TypeName: "PageIndex"},
			want: "fix: Add `// PageIndex is /`",
		},
		"ErrPageMissingPathComm/profile": {
			err:  &parser.PageMissingPathCommError{TypeName: "PageProfile"},
			want: "fix: Add `// PageProfile is /profile/`",
		},
		"ErrPageMissingPathComm/foobar": {
			err:  &parser.PageMissingPathCommError{TypeName: "PageFooBar"},
			want: "fix: Add `// PageFooBar is /foobar/`",
		},
		"ErrPageMissingPathComm/wrapped": {
			err: fmt.Errorf("outer: %w", &parser.PageMissingPathCommError{
				TypeName: "PageProfile",
			}),
			want: "fix: Add `// PageProfile is /profile/`",
		},

		"ErrPageMissingGET/profile": {
			err:  &parser.PageMissingGETError{TypeName: "PageProfile"},
			want: "fix: Add `func (p PageProfile) GET(r *http.Request) (body templ.Component, err error) {}`",
		},
		"ErrPageMissingGET/index": {
			err:  &parser.PageMissingGETError{TypeName: "PageIndex"},
			want: "fix: Add `func (p PageIndex) GET(r *http.Request) (body templ.Component, err error) {}`",
		},

		"ErrPageIndexPathMustBeRoot": {
			err:  &parser.PageIndexPathMustBeRootError{Route: "/home"},
			want: "fix: Use `// PageIndex is /`",
		},

		"ErrPageInvalidPathComm/profile": {
			err:  &parser.PageInvalidPathCommError{TypeName: "PageProfile"},
			want: "fix: First doc comment line must be `// PageProfile is /profile/`",
		},
		"ErrPageInvalidPathComm/index": {
			err:  &parser.PageInvalidPathCommError{TypeName: "PageIndex"},
			want: "fix: First doc comment line must be `// PageIndex is /`",
		},

		"ErrActionMissingPathComm/with page path": {
			err: &parser.ActionMissingPathCommError{
				PagePath:   "/profile/",
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: Add `// POSTFoo is /profile/foo`",
		},
		"ErrActionMissingPathComm/root page": {
			err: &parser.ActionMissingPathCommError{
				PagePath:   "/",
				Recv:       "PageIndex",
				MethodName: "POSTLogin",
			},
			want: "fix: Add `// POSTLogin is /login`",
		},
		"ErrActionMissingPathComm/app level": {
			err: &parser.ActionMissingPathCommError{
				Recv:       "App",
				MethodName: "POSTSignup",
			},
			want: "fix: Add `// POSTSignup is /signup`",
		},

		"ErrActionInvalidPathComm": {
			err: &parser.ActionInvalidPathCommError{
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: First doc comment line must be `// POSTFoo is /profile/foo`",
		},
		"ErrActionInvalidPathComm/app": {
			err: &parser.ActionInvalidPathCommError{
				Recv:       "App",
				MethodName: "DELETEItem",
			},
			want: "fix: First doc comment line must be `// DELETEItem is /item`",
		},

		"ErrActionPathNotUnderPage": {
			err: &parser.ActionPathNotUnderPageError{
				PagePath:   "/profile/",
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: Use `// POSTFoo is /profile/foo`",
		},
		"ErrActionPathNotUnderPage/delete": {
			err: &parser.ActionPathNotUnderPageError{
				PagePath:   "/items/",
				Recv:       "PageItems",
				MethodName: "DELETEItem",
			},
			want: "fix: Use `// DELETEItem is /items/item`",
		},

		"ErrEventCommMissing": {
			err:  &parser.EventCommMissingError{TypeName: "EventUserCreated"},
			want: "fix: Add `// EventUserCreated is \"subject\"` as the first doc comment line",
		},

		"ErrEventCommInvalid": {
			err:  &parser.EventCommInvalidError{TypeName: "EventUserCreated"},
			want: "fix: First doc comment line must be `// EventUserCreated is \"subject\"`",
		},

		"ErrPathFieldMissingTag": {
			err: &paramvalidation.PathFieldMissingTagError{
				FieldName: "UserID",
				Recv:      "PageProfile",
				Method:    "GETProfile",
			},
			want: "fix: Add `path:\"user_id\"` struct tag to field UserID",
		},

		"ErrPathFieldEmptyTag": {
			err: &paramvalidation.PathFieldEmptyTagError{
				FieldName: "UserID",
				Recv:      "PageProfile",
				Method:    "GETProfile",
			},
			want: "fix: Add a non-empty name to the path tag of field UserID, e.g. `path:\"user_id\"`",
		},

		"ErrQueryFieldMissingTag": {
			err: &paramvalidation.QueryFieldMissingTagError{
				FieldName: "Page",
				Recv:      "PageItems",
				Method:    "GETItems",
			},
			want: "fix: Add `query:\"page\"` struct tag to field Page",
		},

		"ErrQueryFieldEmptyTag": {
			err: &paramvalidation.QueryFieldEmptyTagError{
				FieldName: "Page",
				Recv:      "PageItems",
				Method:    "GETItems",
			},
			want: "fix: Add a non-empty name to the query tag of field Page, e.g. `query:\"page\"`",
		},

		"ErrSignalsFieldMissingTag": {
			err: &paramvalidation.SignalsFieldMissingTagError{
				FieldName: "SearchQuery",
				Recv:      "PageSearch",
				Method:    "POSTSearch",
			},
			want: "fix: Add `json:\"search_query\"` struct tag to field SearchQuery",
		},

		"ErrSignalsFieldEmptyTag": {
			err: &paramvalidation.SignalsFieldEmptyTagError{
				FieldName: "SearchQuery",
				Recv:      "PageSearch",
				Method:    "POSTSearch",
			},
			want: "fix: Add a non-empty name to the json tag of field SearchQuery, e.g. `json:\"search_query\"`",
		},

		"ErrEventFieldMissingTag": {
			err: &parser.EventFieldMissingTagError{
				FieldName: "UserID",
				TypeName:  "EventUserCreated",
			},
			want: "fix: Add `json:\"user_id\"` struct tag to field UserID",
		},

		"ErrEventFieldEmptyTag": {
			err: &parser.EventFieldEmptyTagError{
				FieldName: "UserID",
				TypeName:  "EventUserCreated",
			},
			want: "fix: Add a non-empty name to the json tag of field UserID, " +
				"e.g. `json:\"user_id\"`",
		},

		"ErrEventSubjectUserNoSession": {
			err: &parser.EventSubjectUserNoSessionError{
				TypeName: "EventChat", PkgName: "app",
			},
			want: "fix: Define a Session type in package app",
		},

		"ErrEventSubjectAfterPayload": {
			err: &parser.EventSubjectAfterPayloadError{
				FieldName: "SubjectUser",
				TypeName:  "EventChat",
			},
			want: "fix: Move SubjectUser before payload fields in EventChat",
		},

		"ErrEventSubjectOverlap": {
			err: &parser.EventSubjectOverlapError{
				Subject:       "notify.user",
				TypeName:      "EventNotifyUser",
				FirstSubject:  "notify",
				FirstTypeName: "EventNotify",
			},
			want: "fix: Give EventNotifyUser a subject outside \"notify.\", which EventNotify occupies with its subject fields",
		},

		"ErrEventSubjectDuplicateSignal": {
			err: &parser.EventSubjectDuplicateSignalError{
				FieldName:      "SubjectBar",
				FirstFieldName: "SubjectFoo",
				SignalName:     "instance_id",
				TypeName:       "EventCalc",
			},
			want: "fix: Use a unique signal tag for SubjectBar in EventCalc (signal \"instance_id\" is already used by SubjectFoo)",
		},

		"ErrEventSubjectDerivedType": {
			err: &parser.EventSubjectDerivedTypeError{
				FieldName:       "To",
				TypeName:        "EventDirect",
				DeclTypeName:    "UserID",
				SubjectTypeName: "datapages.SubjectUser",
			},
			want: "fix: Type To in EventDirect as datapages.SubjectUser instead of UserID",
		},

		"ErrEventSubjectSignalInvalid": {
			err: &parser.EventSubjectSignalInvalidError{
				FieldName:  "SubjectInstance",
				SignalName: "has spaces",
				TypeName:   "EventBad",
			},
			want: "fix: Use a valid signal name for SubjectInstance in EventBad (must start with a lowercase letter, then lowercase/digits/underscores/dots)",
		},

		"ErrEventSubjectUserSignal": {
			err: &parser.EventSubjectUserSignalError{TypeName: "EventChat"},
			want: "fix: Remove the signal tag: a datapages.SubjectUser(s) field" +
				" is always bound to the authenticated user's ID",
		},

		"ErrTemplHrefRelative/simple": {
			err:  &parser.TemplHrefRelativeError{URL: "/login"},
			want: `fix: Use href={ href.PageLogin(...) } instead of "/login"`,
		},
		"ErrTemplHrefRelative/index": {
			err:  &parser.TemplHrefRelativeError{URL: "/"},
			want: `fix: Use href={ href.PageIndex(...) } instead of "/"`,
		},
		"ErrTemplHrefRelative/trailing slash": {
			err:  &parser.TemplHrefRelativeError{URL: "/profile/"},
			want: `fix: Use href={ href.PageProfile(...) } instead of "/profile/"`,
		},
		"ErrTemplHrefRelative/deep path fallback": {
			err:  &parser.TemplHrefRelativeError{URL: "/profile/edit"},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package instead of "/profile/edit"`,
		},
		"ErrTemplActionHardcoded/app level": {
			err:  &parser.TemplActionHardcodedError{URL: "/submit"},
			want: `fix: Use action={ action.POSTAppSubmit(...) } instead of "/submit"`,
		},
		"ErrTemplActionHardcoded/page level": {
			err:  &parser.TemplActionHardcodedError{URL: "/profile/save"},
			want: `fix: Use action={ action.POSTPageProfileSave(...) } instead of "/profile/save"`,
		},
		"ErrTemplActionHardcoded/deep path fallback": {
			err:  &parser.TemplActionHardcodedError{URL: "/a/b/c"},
			want: `fix: Use action={ action.Xxx(...) } from the generated action package instead of "/a/b/c"`,
		},
		"ErrTemplActionUnverifiable": {
			err:  &parser.TemplActionUnverifiableError{Expr: `buildAction()`},
			want: `fix: Use action={ action.Xxx(...) } from the generated action package instead of "buildAction()"`,
		},
		"ErrTemplActionUnverifiableWithPrefix": {
			err: &parser.TemplActionUnverifiableWithPrefixError{
				Expr:       `"$_fresh = true; " + action.POSTPageIndexCalculate()`,
				ActionFunc: "POSTPageIndexCalculate",
				Prefix:     `"$_fresh = true; "`,
			},
			want: `fix: Use action.POSTPageIndexCalculate(action.WithBefore("$_fresh = true; ")) instead of concatenating a prefix`,
		},
		"ErrTemplActionUnverifiableWithSuffix": {
			err: &parser.TemplActionUnverifiableWithSuffixError{
				Expr:       `action.POSTPageIndexCalculate() + "; $_fresh = true"`,
				ActionFunc: "POSTPageIndexCalculate",
				Suffix:     `"; $_fresh = true"`,
			},
			want: `fix: Use action.POSTPageIndexCalculate(action.WithAfter("; $_fresh = true")) instead of concatenating a suffix`,
		},
		"ErrTemplFormAction": {
			err:  &parser.TemplFormActionError{},
			want: "fix: Remove the action attribute and use data-on:submit with Datastar actions instead",
		},
		"ErrTemplHrefUnverifiable": {
			err:  &parser.TemplHrefUnverifiableError{Expr: `templ.SafeURL("/about")`},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package, or href={ href.External(url) } for external URLs instead of "templ.SafeURL(\"/about\")"`,
		},

		"ErrTemplHrefExternalIsRelative/known page": {
			err:  &parser.TemplHrefExternalIsRelativeError{URL: "/login"},
			want: `fix: Use href={ href.PageLogin(...) } instead of href.External("/login")`,
		},
		"ErrTemplHrefExternalIsRelative/deep path fallback": {
			err:  &parser.TemplHrefExternalIsRelativeError{URL: "/a/b/c"},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package instead of href.External("/a/b/c")`,
		},

		"ErrTemplHrefContext": {
			err: &parser.TemplHrefContextError{
				AttrName: "data-on:click",
				HrefFunc: "PageIndex",
			},
			want: "fix: href.PageIndex() returns a URL path, not a Datastar action — use action.Xxx(...) from the generated action package instead",
		},
		"ErrTemplActionContext": {
			err: &parser.TemplActionContextError{
				AttrName:   "href",
				ActionFunc: "POSTPageLoginSubmit",
			},
			want: "fix: action.POSTPageLoginSubmit() is a Datastar action, not a URL — use href.PageXxx(...) from the generated href package instead",
		},
		"ErrTemplActionWrongPage": {
			err: &parser.TemplActionWrongPageError{
				ActionFunc: "POSTPageProfileSave",
				PageType:   "PageSettings",
				OwnerPage:  "PageProfile",
			},
			want: "fix: Move this action reference to a template used by PageProfile, or use an action owned by PageSettings",
		},

		"ErrSignatureUnsupportedInput/remove": {
			err: &parser.SignatureUnsupportedInputError{
				ParamName:  "b",
				ParamType:  "*net/http.Request",
				Recv:       "PageFoo",
				MethodName: "GET",
			},
			want: "fix: Remove parameter b",
		},
		"ErrSignatureUnsupportedInput/single candidate": {
			err: &parser.SignatureUnsupportedInputError{
				ParamName:      "s",
				ParamType:      "uint64",
				Recv:           "PageFoo",
				MethodName:     "StreamOpen",
				CandidateNames: []string{"datapages.StreamID"},
			},
			want: "fix: Potential candidates: datapages.StreamID",
		},
		"ErrSignatureUnsupportedInput/type struct multiple candidates": {
			err: &parser.SignatureUnsupportedInputError{
				ParamName:  "data",
				ParamType:  "struct{...}",
				Recv:       "PageFoo",
				MethodName: "GET",
				CandidateNames: []string{
					"datapages.Path[...]",
					"datapages.Query[...]",
					"datapages.Signals[...]",
				},
			},
			want: "fix: Potential candidates: datapages.Path[...], " +
				"datapages.Query[...], datapages.Signals[...]",
		},

		"ErrPathFieldUnsupportedType": {
			err: fmt.Errorf(
				"%w: field ID in PageFoo.GET",
				parser.ErrPathFieldUnsupportedType,
			),
			want: "fix: Use either of: string, bool, " +
				"int, int8, int16, int32, int64, " +
				"uint, uint8, uint16, uint32, uint64, " +
				"float32, float64, or encoding.TextUnmarshaler",
		},

		"ErrQueryFieldUnsupportedType": {
			err: fmt.Errorf(
				"%w: field Data in PageFoo.GET",
				parser.ErrQueryFieldUnsupportedType,
			),
			want: "fix: Use either of: string, bool, " +
				"int, int8, int16, int32, int64, " +
				"uint, uint8, uint16, uint32, uint64, " +
				"float32, float64, or encoding.TextUnmarshaler",
		},

		"ErrDispatchDuplicate": {
			err: &parser.DispatchDuplicateError{
				Recv:          "PageFoo",
				MethodName:    "GET",
				EventTypeName: "EventFoo",
			},
			want: "fix: Remove the second datapages.Dispatcher[EventFoo]" +
				" parameter in PageFoo.GET",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, errsuggest.Suggest(tc.err))
		})
	}
}
