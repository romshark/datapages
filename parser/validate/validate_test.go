package validate_test

import (
	"datapages/parser/validate"
	"go/ast"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPageTypeName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.PageTypeName(input), expect)
	}

	f(nil, "PageIndex")
	f(nil, "PageA")
	f(nil, "PageA1")
	f(nil, "PageA1b2")

	// missing suffix
	f(validate.ErrPageTypeNameInvalid, "Page")
	// wrong prefix case
	f(validate.ErrPageTypeNameInvalid, "pageIndex")
	// suffix must start with A-Z
	f(validate.ErrPageTypeNameInvalid, "Pageindex")
	// invalid char
	f(validate.ErrPageTypeNameInvalid, "Page_ABC")
	// invalid char
	f(validate.ErrPageTypeNameInvalid, "Page-ABC")
	// whitespace
	f(validate.ErrPageTypeNameInvalid, "Page ABC")
	// non-ascii
	f(validate.ErrPageTypeNameInvalid, "PageÄBC")
	// non-ascii
	f(validate.ErrPageTypeNameInvalid, "Page💥")
	// wrong prefix
	f(validate.ErrPageTypeNameInvalid, "XPageIndex")
	// invalid char after valid start
	f(validate.ErrPageTypeNameInvalid, "PageA_B")
	// invalid char after valid start
	f(validate.ErrPageTypeNameInvalid, "PageA-B")
	// whitespace after valid start
	f(validate.ErrPageTypeNameInvalid, "PageA B")
	// non-ascii after valid start
	f(validate.ErrPageTypeNameInvalid, "PageAÄ")
	// non-ascii after valid start
	f(validate.ErrPageTypeNameInvalid, "PageA💥")
}

func TestActionMethodName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.ActionMethodName(input), expect)
	}

	f(nil, "POSTA")
	f(nil, "POSTDoThing")
	f(nil, "PUTA")
	f(nil, "PUTDoThing2")
	f(nil, "DELETEA")
	f(nil, "DELETEThing99")

	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "POST")
	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "PUT")
	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "DELETE")
	// suffix must start with A-Z
	f(validate.ErrActionMethodNameInvalid, "POSTdoThing")
	// invalid char
	f(validate.ErrActionMethodNameInvalid, "PUT_do")
	// whitespace
	f(validate.ErrActionMethodNameInvalid, "DELETE do")
	// wrong verb
	f(validate.ErrActionMethodNameInvalid, "GETThing")
	// wrong verb
	f(validate.ErrActionMethodNameInvalid, "PATCHThing")
	// wrong case
	f(validate.ErrActionMethodNameInvalid, "postThing")
	// invalid char after valid start
	f(validate.ErrActionMethodNameInvalid, "POSTA_B")
	// invalid char after valid start
	f(validate.ErrActionMethodNameInvalid, "PUTA-B")
	// whitespace after valid start
	f(validate.ErrActionMethodNameInvalid, "DELETEA B")
	// non-ascii after valid start
	f(validate.ErrActionMethodNameInvalid, "POSTAÄ")
	// non-ascii after valid start
	f(validate.ErrActionMethodNameInvalid, "PUTA💥")
}

func TestEventTypeName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.EventTypeName(input), expect)
	}

	f(nil, "EventFoo")
	f(nil, "EventA")
	f(nil, "EventA1")
	f(nil, "EventA1b2")

	// missing suffix
	f(validate.ErrEventTypeNameInvalid, "Event")
	// wrong prefix case
	f(validate.ErrEventTypeNameInvalid, "eventFoo")
	// suffix must start with A-Z
	f(validate.ErrEventTypeNameInvalid, "Eventfoo")
	// invalid char
	f(validate.ErrEventTypeNameInvalid, "Event_Foo")
	// invalid char
	f(validate.ErrEventTypeNameInvalid, "Event-Foo")
	// whitespace
	f(validate.ErrEventTypeNameInvalid, "Event Foo")
	// non-ascii
	f(validate.ErrEventTypeNameInvalid, "EventÄBC")
	// non-ascii
	f(validate.ErrEventTypeNameInvalid, "Event💥")
	// wrong prefix
	f(validate.ErrEventTypeNameInvalid, "XEventFoo")
	// invalid char after valid start
	f(validate.ErrEventTypeNameInvalid, "EventA_B")
	// invalid char after valid start
	f(validate.ErrEventTypeNameInvalid, "EventA-B")
	// whitespace after valid start
	f(validate.ErrEventTypeNameInvalid, "EventA B")
	// non-ascii after valid start
	f(validate.ErrEventTypeNameInvalid, "EventAÄ")
	// non-ascii after valid start
	f(validate.ErrEventTypeNameInvalid, "EventA💥")
}

func TestEventSubjectCommentSubject(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.EventSubjectCommentSubject(input), expect)
	}

	f(nil, `"foo"`)
	f(nil, `"foo.bar"`)
	f(nil, `" foo "`) // non-empty payload allowed

	// empty
	f(validate.ErrEventSubjectInvalid, ``)
	// whitespace
	f(validate.ErrEventSubjectInvalid, ` `)
	// no quotes
	f(validate.ErrEventSubjectInvalid, `foo`)
	// unterminated/empty
	f(validate.ErrEventSubjectInvalid, `"`)
	// missing closing quote
	f(validate.ErrEventSubjectInvalid, `"foo`)
	// mismatched closer
	f(validate.ErrEventSubjectInvalid, `"foo'`)
	// empty payload
	f(validate.ErrEventSubjectInvalid, `""`)
	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "`foo`")
	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "``")
	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "`")
	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "`foo")
	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "`foo\"")
}

func TestEventSubjectComment(t *testing.T) {
	f := func(expect error, typeName string, doc *ast.CommentGroup) {
		t.Helper()
		require.ErrorIs(t, validate.EventSubjectComment(typeName, doc), expect)
	}

	cg := func(lines ...string) *ast.CommentGroup {
		out := &ast.CommentGroup{}
		for _, l := range lines {
			out.List = append(out.List, &ast.Comment{Text: "// " + l})
		}
		return out
	}

	f(nil, "EventFoo", cg(`EventFoo is "foo.bar"`))
	f(nil, "EventFoo", cg("other", `EventFoo is "foo.bar"`, "more"))

	// backticks not supported
	f(validate.ErrEventSubjectInvalid, "EventFoo", cg("EventFoo is `foo.bar`"))

	// missing quotes
	f(validate.ErrEventSubjectInvalid, "EventFoo", cg("EventFoo is foo.bar"))
	// unterminated
	f(validate.ErrEventSubjectInvalid, "EventFoo", cg(`EventFoo is "foo.bar`))
	// wrong form
	f(validate.ErrEventSubjectInvalidSyntax, "EventFoo", cg(`EventFoo foo`))
	// wrong symbol
	f(validate.ErrEventSubjectMissing, "EventFoo", cg(`EventBar is "foo.bar"`))
	// no matching line
	f(validate.ErrEventSubjectMissing, "EventFoo", cg("something else"))
	// nil doc
	f(validate.ErrEventSubjectMissing, "EventFoo", (*ast.CommentGroup)(nil))
}

func TestEventHandlerMethodName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.EventHandlerMethodName(input), expect)
	}

	f(nil, "OnFoo")
	f(nil, "OnA")
	f(nil, "OnA1")
	f(nil, "OnA1b2")

	// missing suffix
	f(validate.ErrEventHandlerMethodNameInvalid, "On")
	// wrong prefix case
	f(validate.ErrEventHandlerMethodNameInvalid, "onFoo")
	// suffix must start with A-Z
	f(validate.ErrEventHandlerMethodNameInvalid, "Onfoo")
	// invalid char
	f(validate.ErrEventHandlerMethodNameInvalid, "On_Foo")
	// invalid char
	f(validate.ErrEventHandlerMethodNameInvalid, "On-Foo")
	// whitespace
	f(validate.ErrEventHandlerMethodNameInvalid, "On Foo")
	// non-ascii
	f(validate.ErrEventHandlerMethodNameInvalid, "OnÄBC")
	// non-ascii
	f(validate.ErrEventHandlerMethodNameInvalid, "On💥")
	// wrong prefix
	f(validate.ErrEventHandlerMethodNameInvalid, "XOnFoo")
	// invalid char after valid start
	f(validate.ErrEventHandlerMethodNameInvalid, "OnA_B")
	// invalid char after valid start
	f(validate.ErrEventHandlerMethodNameInvalid, "OnA-B")
	// whitespace after valid start
	f(validate.ErrEventHandlerMethodNameInvalid, "OnA B")
	// non-ascii after valid start
	f(validate.ErrEventHandlerMethodNameInvalid, "OnAÄ")
	// non-ascii after valid start
	f(validate.ErrEventHandlerMethodNameInvalid, "OnA💥")
}
