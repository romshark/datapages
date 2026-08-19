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

		"ErrSignatureEvHandMissingSSE": {
			err:  parser.ErrSignatureEvHandMissingSSE,
			want: "fix: Add `sse datapages.SSE` parameter",
		},
		"ErrSignatureEvHandMissingSSE/wrapped": {
			err:  fmt.Errorf("%w: PageFoo.OnEventBar", parser.ErrSignatureEvHandMissingSSE),
			want: "fix: Add `sse datapages.SSE` parameter",
		},

		"ErrSignatureEvHandMissingEvent": {
			err:  parser.ErrSignatureEvHandMissingEvent,
			want: "fix: Add `event EventName` parameter",
		},
		"ErrSignatureEvHandMissingEvent/wrapped": {
			err:  fmt.Errorf("%w: PageFoo.OnEventBar", parser.ErrSignatureEvHandMissingEvent),
			want: "fix: Add `event EventName` parameter",
		},

		"ErrPageMissingFieldApp": {
			err:  &parser.ErrorPageMissingFieldApp{TypeName: "PageProfile"},
			want: "fix: Add field `App *App` to PageProfile",
		},
		"ErrPageMissingFieldApp/wrapped": {
			err:  fmt.Errorf("outer: %w", &parser.ErrorPageMissingFieldApp{TypeName: "PageProfile"}),
			want: "fix: Add field `App *App` to PageProfile",
		},

		"ErrPageMissingPathComm/index": {
			err:  &parser.ErrorPageMissingPathComm{TypeName: "PageIndex"},
			want: "fix: Add `// PageIndex is /`",
		},
		"ErrPageMissingPathComm/profile": {
			err:  &parser.ErrorPageMissingPathComm{TypeName: "PageProfile"},
			want: "fix: Add `// PageProfile is /profile/`",
		},
		"ErrPageMissingPathComm/foobar": {
			err:  &parser.ErrorPageMissingPathComm{TypeName: "PageFooBar"},
			want: "fix: Add `// PageFooBar is /foobar/`",
		},
		"ErrPageMissingPathComm/wrapped": {
			err:  fmt.Errorf("outer: %w", &parser.ErrorPageMissingPathComm{TypeName: "PageProfile"}),
			want: "fix: Add `// PageProfile is /profile/`",
		},

		"ErrPageMissingGET/profile": {
			err:  &parser.ErrorPageMissingGET{TypeName: "PageProfile"},
			want: "fix: Add `func (p PageProfile) GET(r *http.Request) (body templ.Component, err error) {}`",
		},
		"ErrPageMissingGET/index": {
			err:  &parser.ErrorPageMissingGET{TypeName: "PageIndex"},
			want: "fix: Add `func (p PageIndex) GET(r *http.Request) (body templ.Component, err error) {}`",
		},

		"ErrPageIndexPathMustBeRoot": {
			err:  &parser.ErrorPageIndexPathMustBeRoot{Route: "/home"},
			want: "fix: Use `// PageIndex is /`",
		},

		"ErrPageInvalidPathComm/profile": {
			err:  &parser.ErrorPageInvalidPathComm{TypeName: "PageProfile"},
			want: "fix: First doc comment line must be `// PageProfile is /profile/`",
		},
		"ErrPageInvalidPathComm/index": {
			err:  &parser.ErrorPageInvalidPathComm{TypeName: "PageIndex"},
			want: "fix: First doc comment line must be `// PageIndex is /`",
		},

		"ErrActionMissingPathComm/with page path": {
			err: &parser.ErrorActionMissingPathComm{
				PagePath:   "/profile/",
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: Add `// POSTFoo is /profile/foo`",
		},
		"ErrActionMissingPathComm/root page": {
			err: &parser.ErrorActionMissingPathComm{
				PagePath:   "/",
				Recv:       "PageIndex",
				MethodName: "POSTLogin",
			},
			want: "fix: Add `// POSTLogin is /login`",
		},
		"ErrActionMissingPathComm/app level": {
			err: &parser.ErrorActionMissingPathComm{
				Recv:       "App",
				MethodName: "POSTSignup",
			},
			want: "fix: Add `// POSTSignup is /signup`",
		},

		"ErrActionInvalidPathComm": {
			err: &parser.ErrorActionInvalidPathComm{
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: First doc comment line must be `// POSTFoo is /profile/foo`",
		},
		"ErrActionInvalidPathComm/app": {
			err: &parser.ErrorActionInvalidPathComm{
				Recv:       "App",
				MethodName: "DELETEItem",
			},
			want: "fix: First doc comment line must be `// DELETEItem is /item`",
		},

		"ErrActionPathNotUnderPage": {
			err: &parser.ErrorActionPathNotUnderPage{
				PagePath:   "/profile/",
				Recv:       "PageProfile",
				MethodName: "POSTFoo",
			},
			want: "fix: Use `// POSTFoo is /profile/foo`",
		},
		"ErrActionPathNotUnderPage/delete": {
			err: &parser.ErrorActionPathNotUnderPage{
				PagePath:   "/items/",
				Recv:       "PageItems",
				MethodName: "DELETEItem",
			},
			want: "fix: Use `// DELETEItem is /items/item`",
		},

		"ErrEventCommMissing": {
			err:  &parser.ErrorEventCommMissing{TypeName: "EventUserCreated"},
			want: "fix: Add `// EventUserCreated is \"subject\"` as the first doc comment line",
		},

		"ErrEventCommInvalid": {
			err:  &parser.ErrorEventCommInvalid{TypeName: "EventUserCreated"},
			want: "fix: First doc comment line must be `// EventUserCreated is \"subject\"`",
		},

		"ErrPathFieldMissingTag": {
			err: &paramvalidation.ErrorPathFieldMissingTag{
				FieldName: "UserID",
				Recv:      "PageProfile",
				Method:    "GETProfile",
			},
			want: "fix: Add `path:\"user_id\"` struct tag to field UserID",
		},

		"ErrPathFieldEmptyTag": {
			err: &paramvalidation.ErrorPathFieldEmptyTag{
				FieldName: "UserID",
				Recv:      "PageProfile",
				Method:    "GETProfile",
			},
			want: "fix: Add a non-empty name to the path tag of field UserID, e.g. `path:\"user_id\"`",
		},

		"ErrQueryFieldMissingTag": {
			err: &paramvalidation.ErrorQueryFieldMissingTag{
				FieldName: "Page",
				Recv:      "PageItems",
				Method:    "GETItems",
			},
			want: "fix: Add `query:\"page\"` struct tag to field Page",
		},

		"ErrQueryFieldEmptyTag": {
			err: &paramvalidation.ErrorQueryFieldEmptyTag{
				FieldName: "Page",
				Recv:      "PageItems",
				Method:    "GETItems",
			},
			want: "fix: Add a non-empty name to the query tag of field Page, e.g. `query:\"page\"`",
		},

		"ErrSignalsFieldMissingTag": {
			err: &paramvalidation.ErrorSignalsFieldMissingTag{
				FieldName: "SearchQuery",
				Recv:      "PageSearch",
				Method:    "POSTSearch",
			},
			want: "fix: Add `json:\"search_query\"` struct tag to field SearchQuery",
		},

		"ErrSignalsFieldEmptyTag": {
			err: &paramvalidation.ErrorSignalsFieldEmptyTag{
				FieldName: "SearchQuery",
				Recv:      "PageSearch",
				Method:    "POSTSearch",
			},
			want: "fix: Add a non-empty name to the json tag of field SearchQuery, e.g. `json:\"search_query\"`",
		},

		"ErrEventFieldMissingTag": {
			err: &parser.ErrorEventFieldMissingTag{
				FieldName: "UserID",
				TypeName:  "EventUserCreated",
			},
			want: "fix: Add `json:\"user_id\"` struct tag to field UserID",
		},

		"ErrEventFieldEmptyTag": {
			err: &parser.ErrorEventFieldEmptyTag{
				FieldName: "UserID",
				TypeName:  "EventUserCreated",
			},
			want: "fix: Add a non-empty name to the json tag of field UserID, " +
				"e.g. `json:\"user_id\"`",
		},

		"ErrEventSubjectUserNoSession": {
			err:  &parser.ErrorEventSubjectUserNoSession{TypeName: "EventChat", PkgName: "app"},
			want: "fix: Define a Session type in package app",
		},

		"ErrEventSubjectAfterPayload": {
			err: &parser.ErrorEventSubjectAfterPayload{
				FieldName: "SubjectUser",
				TypeName:  "EventChat",
			},
			want: "fix: Move SubjectUser before payload fields in EventChat",
		},

		"ErrEventSubjectOverlap": {
			err: &parser.ErrorEventSubjectOverlap{
				Subject:       "notify.user",
				TypeName:      "EventNotifyUser",
				FirstSubject:  "notify",
				FirstTypeName: "EventNotify",
			},
			want: "fix: Give EventNotifyUser a subject outside \"notify.\", which EventNotify occupies with its subject fields",
		},

		"ErrEventSubjectDuplicateSignal": {
			err: &parser.ErrorEventSubjectDuplicateSignal{
				FieldName:      "SubjectBar",
				FirstFieldName: "SubjectFoo",
				SignalName:     "instance_id",
				TypeName:       "EventCalc",
			},
			want: "fix: Use a unique signal tag for SubjectBar in EventCalc (signal \"instance_id\" is already used by SubjectFoo)",
		},

		"ErrEventSubjectSignalInvalid": {
			err: &parser.ErrorEventSubjectSignalInvalid{
				FieldName:  "SubjectInstance",
				SignalName: "has spaces",
				TypeName:   "EventBad",
			},
			want: "fix: Use a valid signal name for SubjectInstance in EventBad (must start with a lowercase letter, then lowercase/digits/underscores/dots)",
		},

		"ErrEventSubjectUserSignal": {
			err: &parser.ErrorEventSubjectUserSignal{TypeName: "EventChat"},
			want: "fix: Remove the signal tag: a datapages.SubjectUser(s) field" +
				" is always bound to the authenticated user's ID",
		},

		"ErrTemplHrefRelative/simple": {
			err:  &parser.ErrorTemplHrefRelative{URL: "/login"},
			want: `fix: Use href={ href.PageLogin(...) } instead of "/login"`,
		},
		"ErrTemplHrefRelative/index": {
			err:  &parser.ErrorTemplHrefRelative{URL: "/"},
			want: `fix: Use href={ href.PageIndex(...) } instead of "/"`,
		},
		"ErrTemplHrefRelative/trailing slash": {
			err:  &parser.ErrorTemplHrefRelative{URL: "/profile/"},
			want: `fix: Use href={ href.PageProfile(...) } instead of "/profile/"`,
		},
		"ErrTemplHrefRelative/deep path fallback": {
			err:  &parser.ErrorTemplHrefRelative{URL: "/profile/edit"},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package instead of "/profile/edit"`,
		},
		"ErrTemplActionHardcoded/app level": {
			err:  &parser.ErrorTemplActionHardcoded{URL: "/submit"},
			want: `fix: Use action={ action.POSTAppSubmit(...) } instead of "/submit"`,
		},
		"ErrTemplActionHardcoded/page level": {
			err:  &parser.ErrorTemplActionHardcoded{URL: "/profile/save"},
			want: `fix: Use action={ action.POSTPageProfileSave(...) } instead of "/profile/save"`,
		},
		"ErrTemplActionHardcoded/deep path fallback": {
			err:  &parser.ErrorTemplActionHardcoded{URL: "/a/b/c"},
			want: `fix: Use action={ action.Xxx(...) } from the generated action package instead of "/a/b/c"`,
		},
		"ErrTemplActionUnverifiable": {
			err:  &parser.ErrorTemplActionUnverifiable{Expr: `buildAction()`},
			want: `fix: Use action={ action.Xxx(...) } from the generated action package instead of "buildAction()"`,
		},
		"ErrTemplActionUnverifiableWithPrefix": {
			err: &parser.ErrorTemplActionUnverifiableWithPrefix{
				Expr:       `"$_fresh = true; " + action.POSTPageIndexCalculate()`,
				ActionFunc: "POSTPageIndexCalculate",
				Prefix:     `"$_fresh = true; "`,
			},
			want: `fix: Use action.POSTPageIndexCalculate(action.WithBefore("$_fresh = true; ")) instead of concatenating a prefix`,
		},
		"ErrTemplActionUnverifiableWithSuffix": {
			err: &parser.ErrorTemplActionUnverifiableWithSuffix{
				Expr:       `action.POSTPageIndexCalculate() + "; $_fresh = true"`,
				ActionFunc: "POSTPageIndexCalculate",
				Suffix:     `"; $_fresh = true"`,
			},
			want: `fix: Use action.POSTPageIndexCalculate(action.WithAfter("; $_fresh = true")) instead of concatenating a suffix`,
		},
		"ErrTemplFormAction": {
			err:  &parser.ErrorTemplFormAction{},
			want: "fix: Remove the action attribute and use data-on:submit with Datastar actions instead",
		},
		"ErrTemplHrefUnverifiable": {
			err:  &parser.ErrorTemplHrefUnverifiable{Expr: `templ.SafeURL("/about")`},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package, or href={ href.External(url) } for external URLs instead of "templ.SafeURL(\"/about\")"`,
		},

		"ErrTemplHrefExternalIsRelative/known page": {
			err:  &parser.ErrorTemplHrefExternalIsRelative{URL: "/login"},
			want: `fix: Use href={ href.PageLogin(...) } instead of href.External("/login")`,
		},
		"ErrTemplHrefExternalIsRelative/deep path fallback": {
			err:  &parser.ErrorTemplHrefExternalIsRelative{URL: "/a/b/c"},
			want: `fix: Use href={ href.Xxx(...) } from the generated href package instead of href.External("/a/b/c")`,
		},

		"ErrTemplHrefContext": {
			err: &parser.ErrorTemplHrefContext{
				AttrName: "data-on:click",
				HrefFunc: "PageIndex",
			},
			want: "fix: href.PageIndex() returns a URL path, not a Datastar action — use action.Xxx(...) from the generated action package instead",
		},
		"ErrTemplActionContext": {
			err: &parser.ErrorTemplActionContext{
				AttrName:   "href",
				ActionFunc: "POSTPageLoginSubmit",
			},
			want: "fix: action.POSTPageLoginSubmit() is a Datastar action, not a URL — use href.PageXxx(...) from the generated href package instead",
		},
		"ErrTemplActionWrongPage": {
			err: &parser.ErrorTemplActionWrongPage{
				ActionFunc: "POSTPageProfileSave",
				PageType:   "PageSettings",
				OwnerPage:  "PageProfile",
			},
			want: "fix: Move this action reference to a template used by PageProfile, or use an action owned by PageSettings",
		},

		"ErrSignatureUnsupportedInput/remove": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:  "b",
				ParamType:  "*net/http.Request",
				Recv:       "PageFoo",
				MethodName: "GET",
			},
			want: "fix: Remove parameter b",
		},
		"ErrSignatureUnsupportedInput/rename": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:    "sess",
				ParamType:    "Session",
				Recv:         "PageFoo",
				MethodName:   "GET",
				ExpectedName: "session",
			},
			want: "fix: Rename parameter sess to session",
		},
		"ErrSignatureUnsupportedInput/fuzzy sessionTok": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:    "sessionTok",
				ParamType:    "string",
				Recv:         "PageFoo",
				MethodName:   "GET",
				ExpectedName: "sessionToken",
			},
			want: "fix: Rename parameter sessionTok to sessionToken",
		},
		"ErrSignatureUnsupportedInput/type string single candidate": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:    "s",
				ParamType:    "string",
				Recv:         "PageFoo",
				MethodName:   "GET",
				ExpectedName: "sessionToken",
			},
			want: "fix: Rename parameter s to sessionToken",
		},
		"ErrSignatureUnsupportedInput/name already matches expected": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:    "signals",
				ParamType:    `struct{InstanceID string "json:\"instance_id\""}`,
				Recv:         "PageIndex",
				MethodName:   "OnCalcUpdated",
				ExpectedName: "signals",
			},
			want: "fix: Remove parameter signals",
		},
		"ErrSignatureUnsupportedInput/type struct multiple candidates": {
			err: &parser.ErrorSignatureUnsupportedInput{
				ParamName:      "data",
				ParamType:      "struct{...}",
				Recv:           "PageFoo",
				MethodName:     "GET",
				CandidateNames: []string{"path", "query", "signals"},
			},
			want: "fix: Potential candidates: path, query, signals",
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

		"ErrDispatchParamLegacy": {
			err: &paramvalidation.ErrorDispatchParamLegacy{
				Recv:       "PageFoo",
				MethodName: "GET",
				ParamName:  "dispatch",
			},
			want: "fix: Type dispatch as datapages.Dispatcher[EventXXX]," +
				" one parameter per event type the handler dispatches",
		},
		"ErrDispatchDuplicate": {
			err: &parser.ErrorDispatchDuplicate{
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
