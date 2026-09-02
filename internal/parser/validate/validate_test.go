package validate_test

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser/validate"
)

// TestPageTypeName tests the naming convention a page type has to follow:
// the "Page" prefix plus an upper-case ASCII letter and alphanumerics after it.
// The name becomes an identifier in generated code and a URL path, which is why
// nothing outside that alphabet is accepted.
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

// TestActionMethodName tests the same convention for an action method,
// whose prefix is the HTTP method it answers.
func TestActionMethodName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.ActionMethodName(input), expect)
	}

	f(nil, "POSTA")
	f(nil, "POSTDoThing")
	f(nil, "PUTA")
	f(nil, "PUTDoThing2")
	f(nil, "PATCHA")
	f(nil, "PATCHThing")
	f(nil, "DELETEA")
	f(nil, "DELETEThing99")
	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "POST")
	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "PUT")
	// missing suffix
	f(validate.ErrActionMethodNameInvalid, "PATCH")
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

// TestEventTypeName tests the same convention for an event type.
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

// TestEventSubjectCommentSubject tests the quoted subject in an event's doc comment.
// Only double quotes delimit it, since the value is read out of the
// comment textually rather than parsed as a Go literal.
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

// TestEventSubjectComment tests the doc comment an event type has to carry:
// its first line names the type, the word "is" and the quoted subject.
// A missing comment, a first line that says something else and an
// unparseable subject are reported apart, since each names a different fix.
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

	// ok: header only
	f(nil, "EventFoo", cg(`EventFoo is "foo.bar"`))

	// ok: header + mandatory blank + description
	f(nil, "EventFoo", cg(`EventFoo is "foo.bar"`, ``, `something else`))

	// ok: extra whitespace between type name and "is"
	f(nil, "EventFoo", cg(`EventFoo  is "foo.bar"`))

	// ok: extra whitespace between "is" and subject
	f(nil, "EventFoo", cg(`EventFoo is   "foo.bar"`))

	// ok: extra whitespace everywhere
	f(nil, "EventFoo", cg(`EventFoo   is   "foo.bar"`))

	// missing: no comment group
	f(validate.ErrEventCommMissing, "EventFoo", (*ast.CommentGroup)(nil))

	// invalid comment: doc exists but header not first line / wrong
	f(validate.ErrEventCommInvalid, "EventFoo",
		cg("other", `EventFoo is "foo.bar"`))
	f(validate.ErrEventCommInvalid, "EventFoo",
		cg(`EventBar is "foo.bar"`))
	f(validate.ErrEventCommInvalid, "EventFoo",
		cg(`foo bar blabla`))
	f(validate.ErrEventCommInvalid, "EventFoo",
		cg(`EventFoo handles "foo.bar"`))

	// ok: header + description without blank line separator
	f(nil, "EventFoo", cg(`EventFoo is "foo.bar"`, `not blank`, `desc`))

	// invalid subject: header ok, quoted payload invalid
	f(validate.ErrEventSubjectInvalid, "EventFoo",
		cg(`EventFoo is ""`))
	f(validate.ErrEventSubjectInvalid, "EventFoo",
		cg(`EventFoo is foo.bar`))
	f(validate.ErrEventSubjectInvalid, "EventFoo",
		cg(`EventFoo is "foo.bar`))
	f(validate.ErrEventSubjectInvalid, "EventFoo",
		cg("EventFoo is `foo.bar`"))
}

// TestEventHandlerMethodName tests the same convention for an event handler,
// whose prefix is "On".
func TestEventHandlerMethodName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.EventHandlerMethodName(input), expect)
	}

	// ok
	f(nil, "OnA")
	f(nil, "OnFoo")
	f(nil, "OnA1")
	f(nil, "OnA1b2")
	f(nil, "OnDoThing99")

	// missing suffix
	f(validate.ErrEventHandlerNameInvalid, "On")

	// wrong prefix case / wrong prefix
	f(validate.ErrEventHandlerNameInvalid, "onFoo")
	f(validate.ErrEventHandlerNameInvalid, "XOnFoo")

	// suffix must start with A-Z
	f(validate.ErrEventHandlerNameInvalid, "Onfoo")

	// invalid chars / whitespace / non-ascii
	f(validate.ErrEventHandlerNameInvalid, "On_Foo")
	f(validate.ErrEventHandlerNameInvalid, "On-Foo")
	f(validate.ErrEventHandlerNameInvalid, "On Foo")
	f(validate.ErrEventHandlerNameInvalid, "OnÄBC")
	f(validate.ErrEventHandlerNameInvalid, "On💥")

	// invalid char after valid start
	f(validate.ErrEventHandlerNameInvalid, "OnA_B")
	f(validate.ErrEventHandlerNameInvalid, "OnA-B")
	f(validate.ErrEventHandlerNameInvalid, "OnA B")
	f(validate.ErrEventHandlerNameInvalid, "OnAÄ")
	f(validate.ErrEventHandlerNameInvalid, "OnA💥")
}

// TestSignalTagName tests the signal names a struct tag may declare. They travel
// into Datastar expressions as identifiers, dotted paths included, which rules out
// upper case, a leading digit or underscore, and anything that could end the attribute.
func TestSignalTagName(t *testing.T) {
	f := func(expect error, input string) {
		t.Helper()
		require.ErrorIs(t, validate.SignalTagName(input), expect)
	}

	// valid
	f(nil, "instance_id")
	f(nil, "x")
	f(nil, "foo")
	f(nil, "foo_bar")
	f(nil, "foo123")
	f(nil, "a.b.c")
	f(nil, "form.name")

	// empty
	f(validate.ErrSignalTagNameInvalid, "")
	// starts with uppercase
	f(validate.ErrSignalTagNameInvalid, "Foo")
	// starts with digit
	f(validate.ErrSignalTagNameInvalid, "1foo")
	// starts with underscore
	f(validate.ErrSignalTagNameInvalid, "_foo")
	// contains spaces
	f(validate.ErrSignalTagNameInvalid, "has spaces")
	// contains uppercase
	f(validate.ErrSignalTagNameInvalid, "camelCase")
	// contains hyphen
	f(validate.ErrSignalTagNameInvalid, "foo-bar")
	// contains quotes
	f(validate.ErrSignalTagNameInvalid, `foo"bar`)
}
