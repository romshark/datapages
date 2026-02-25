package errsuggest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/errsuggest"
	"github.com/romshark/datapages/parser/internal/paramvalidation"
	"github.com/stretchr/testify/require"
)

func TestSuggest(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"nil":           {err: nil, want: ""},
		"unrelated":     {err: errors.New("some error"), want: ""},
		"sentinel only": {err: parser.ErrActionPathNotUnderPage, want: ""},

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
			want: "fix: Add `func (p *PageProfile) GET(r *http.Request) (body templ.Component, err error) {}`",
		},
		"ErrPageMissingGET/index": {
			err:  &parser.ErrorPageMissingGET{TypeName: "PageIndex"},
			want: "fix: Add `func (p *PageIndex) GET(r *http.Request) (body templ.Component, err error) {}`",
		},

		"ErrPageInvalidPathComm/profile": {
			err:  &parser.ErrorPageInvalidPathComm{TypeName: "PageProfile"},
			want: "fix: First doc comment line must be `// PageProfile is /profile/`; if there are more lines, the next must be an empty `//`",
		},
		"ErrPageInvalidPathComm/index": {
			err:  &parser.ErrorPageInvalidPathComm{TypeName: "PageIndex"},
			want: "fix: First doc comment line must be `// PageIndex is /`; if there are more lines, the next must be an empty `//`",
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
			want: "fix: First doc comment line must be `// POSTFoo is /foo`; if there are more lines, the next must be an empty `//`",
		},
		"ErrActionInvalidPathComm/app": {
			err: &parser.ErrorActionInvalidPathComm{
				Recv:       "App",
				MethodName: "DELETEItem",
			},
			want: "fix: First doc comment line must be `// DELETEItem is /item`; if there are more lines, the next must be an empty `//`",
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
			want: "fix: First doc comment line must be `// EventUserCreated is \"subject\"`; if there are more lines, the next must be an empty `//`",
		},

		"ErrPathFieldMissingTag": {
			err: &paramvalidation.ErrorPathFieldMissingTag{
				FieldName: "UserID",
				Recv:      "PageProfile",
				Method:    "GETProfile",
			},
			want: "fix: Add `path:\"userid\"` struct tag to field UserID",
		},

		"ErrQueryFieldMissingTag": {
			err: &paramvalidation.ErrorQueryFieldMissingTag{
				FieldName: "Page",
				Recv:      "PageItems",
				Method:    "GETItems",
			},
			want: "fix: Add `query:\"page\"` struct tag to field Page",
		},

		"ErrSignalsFieldMissingTag": {
			err: &paramvalidation.ErrorSignalsFieldMissingTag{
				FieldName: "SearchQuery",
				Recv:      "PageSearch",
				Method:    "POSTSearch",
			},
			want: "fix: Add `json:\"searchquery\"` struct tag to field SearchQuery",
		},

		"ErrEventFieldMissingTag": {
			err: &parser.ErrorEventFieldMissingTag{
				FieldName: "UserID",
				TypeName:  "EventUserCreated",
			},
			want: "fix: Add `json:\"userid\"` struct tag to field UserID",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, errsuggest.Suggest(tc.err))
		})
	}
}
