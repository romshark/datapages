// Package app exercises route patterns, path variables and query parameters.
//
// Every handler echoes what it parsed. The tests build the URL with the
// generated href package and read the values back out of the response,
// which puts the URL writer and the request parser in the same assertion:
// if either one changes its mind about an order, a name or a format,
// the round trip stops agreeing with itself.
package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// echo renders the parsed values so a test can read them back.
func echo(format string, args ...any) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + fmt.Sprintf(format, args...) + "</pre>")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("index"), nil
}

// PagePath is /p/{str}/{i}/{u}/{f}/{flag}
type PagePath struct{ App *App }

func (PagePath) GET(
	_ *http.Request,
	path datapages.Path[struct {
		S string  `path:"str"`
		I int     `path:"i"`
		U uint64  `path:"u"`
		F float64 `path:"f"`
		B bool    `path:"flag"`
	}],
) (body datapages.Component, err error) {
	return echo("s=%q i=%d u=%d f=%v b=%v",
		path.Values.S, path.Values.I, path.Values.U, path.Values.F, path.Values.B), nil
}

// PageInts is /ints/{i8}/{i16}/{i32}/{i64}/{u8}/{u16}/{u32}
type PageInts struct{ App *App }

func (PageInts) GET(
	_ *http.Request,
	path datapages.Path[struct {
		I8  int8   `path:"i8"`
		I16 int16  `path:"i16"`
		I32 int32  `path:"i32"`
		I64 int64  `path:"i64"`
		U8  uint8  `path:"u8"`
		U16 uint16 `path:"u16"`
		U32 uint32 `path:"u32"`
	}],
) (body datapages.Component, err error) {
	return echo("i8=%d i16=%d i32=%d i64=%d u8=%d u16=%d u32=%d",
		path.Values.I8, path.Values.I16, path.Values.I32, path.Values.I64,
		path.Values.U8, path.Values.U16, path.Values.U32), nil
}

// PageQuery is /q
type PageQuery struct{ App *App }

func (PageQuery) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Term  string  `query:"term"`
		Limit int     `query:"limit"`
		Ratio float32 `query:"ratio"`
		Score float64 `query:"score"`
		Big   uint32  `query:"big"`
		Deep  int64   `query:"deep"`
		Flag  bool    `query:"flag"`
	}],
) (body datapages.Component, err error) {
	return echo("term=%q limit=%d ratio=%v score=%v big=%d deep=%d flag=%v",
		query.Values.Term, query.Values.Limit, query.Values.Ratio, query.Values.Score,
		query.Values.Big, query.Values.Deep, query.Values.Flag), nil
}

// TitledPath is the path variables of PageTitled, a named type where the
// other pages use an anonymous one.
type TitledPath struct {
	Name string `path:"name"`
}

// PageTitled is /titled/{name}
//
// A page that returns its own head alongside its body.
// This is how a page carries a title and its own meta tags.
type PageTitled struct{ App *App }

func (PageTitled) GET(
	_ *http.Request,
	path datapages.Path[TitledPath],
) (view datapages.Component, meta datapages.Head, err error) {
	return echo("titled %s", path.Values.Name),
		templ.Raw("<title>" + path.Values.Name + "</title>"), nil
}

// PageReflect is /reflect
//
// A query parameter bound to a Datastar signal. The value in the URL becomes
// the signal's value on load, and the page carries the code that writes the
// signal back into the URL when it changes.
type PageReflect struct{ App *App }

func (PageReflect) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Term string `query:"t" reflectsignal:"term"`
		Page int    `query:"p" reflectsignal:"page"`
	}],
) (body datapages.Component, err error) {
	return echo("term=%q page=%d", query.Values.Term, query.Values.Page), nil
}

// PageMixed is /org/{org}/item/{id}
//
// Path and query on one handler, which is where an off-by-one in either
// parser shows up as the other one's value.
type PageMixed struct{ App *App }

func (PageMixed) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Org string `path:"org"`
		ID  int    `path:"id"`
	}],
	query datapages.Query[struct {
		Tab  string `query:"tab"`
		Page int    `query:"page"`
	}],
) (body datapages.Component, err error) {
	return echo("org=%q id=%d tab=%q page=%d",
		path.Values.Org, path.Values.ID, query.Values.Tab, query.Values.Page), nil
}

// PageConflict is /c/{value}/{s_value}/{s_s_value}
//
// The URL writer names its locals after the path variables. These three names
// are chosen so that a naive naming scheme produces the same local twice and
// one value lands in two segments.
type PageConflict struct{ App *App }

func (PageConflict) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Value   int32  `path:"value"`
		SValue  int32  `path:"s_value"`
		SSValue string `path:"s_s_value"`
	}],
) (body datapages.Component, err error) {
	return echo("value=%d s_value=%d s_s_value=%q",
		path.Values.Value, path.Values.SValue, path.Values.SSValue), nil
}

// PageFiles is /files/{rest...}
//
// A wildcard that matches the rest of the path. The router takes no end-of-path
// marker after it: one appended there puts the wildcard in the middle,
// which is a pattern it will not parse.
type PageFiles struct{ App *App }

func (PageFiles) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Rest string `path:"rest"`
	}],
) (body datapages.Component, err error) {
	return echo("rest=%q", path.Values.Rest), nil
}

// Slug is a string that parses through its own UnmarshalText. Only lowercase
// values are valid, which is what tells the conversion apart from the parse.
type Slug string

func (s *Slug) UnmarshalText(b []byte) error {
	v := string(b)
	if v != strings.ToLower(v) {
		return fmt.Errorf("slug must be lowercase, received %q", v)
	}
	*s = Slug(v)
	return nil
}

// PageSlug is /slug/{slug}
//
// A named string in a path and in a query, both carrying an UnmarshalText. A generator
// that converts instead of parsing hands the handler a value the type refuses.
type PageSlug struct{ App *App }

func (PageSlug) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Slug Slug `path:"slug"`
	}],
	query datapages.Query[struct {
		Tag Slug `query:"tag"`
	}],
) (body datapages.Component, err error) {
	return echo("slug=%q tag=%q", path.Values.Slug, query.Values.Tag), nil
}
