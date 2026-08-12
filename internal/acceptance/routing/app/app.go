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

	"github.com/a-h/templ"
)

type App struct{}

// echo renders the parsed values so a test can read them back.
func echo(format string, args ...any) templ.Component {
	return templ.Raw("<pre id=\"echo\">" + fmt.Sprintf(format, args...) + "</pre>")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("index"), nil
}

// PagePath is /p/{str}/{i}/{u}/{f}/{flag}
type PagePath struct{ App *App }

func (PagePath) GET(
	_ *http.Request,
	path struct {
		S string  `path:"str"`
		I int     `path:"i"`
		U uint64  `path:"u"`
		F float64 `path:"f"`
		B bool    `path:"flag"`
	},
) (body templ.Component, err error) {
	return echo("s=%q i=%d u=%d f=%v b=%v",
		path.S, path.I, path.U, path.F, path.B), nil
}

// PageInts is /ints/{i8}/{i16}/{i32}/{i64}/{u8}/{u16}/{u32}
type PageInts struct{ App *App }

func (PageInts) GET(
	_ *http.Request,
	path struct {
		I8  int8   `path:"i8"`
		I16 int16  `path:"i16"`
		I32 int32  `path:"i32"`
		I64 int64  `path:"i64"`
		U8  uint8  `path:"u8"`
		U16 uint16 `path:"u16"`
		U32 uint32 `path:"u32"`
	},
) (body templ.Component, err error) {
	return echo("i8=%d i16=%d i32=%d i64=%d u8=%d u16=%d u32=%d",
		path.I8, path.I16, path.I32, path.I64,
		path.U8, path.U16, path.U32), nil
}

// PageQuery is /q
type PageQuery struct{ App *App }

func (PageQuery) GET(
	_ *http.Request,
	query struct {
		Term  string  `query:"term"`
		Limit int     `query:"limit"`
		Ratio float32 `query:"ratio"`
		Score float64 `query:"score"`
		Big   uint32  `query:"big"`
		Deep  int64   `query:"deep"`
		Flag  bool    `query:"flag"`
	},
) (body templ.Component, err error) {
	return echo("term=%q limit=%d ratio=%v score=%v big=%d deep=%d flag=%v",
		query.Term, query.Limit, query.Ratio, query.Score,
		query.Big, query.Deep, query.Flag), nil
}

// PageTitled is /titled/{name}
//
// A page that returns its own head alongside its body.
// This is how a page carries a title and its own meta tags.
type PageTitled struct{ App *App }

func (PageTitled) GET(
	_ *http.Request,
	path struct {
		Name string `path:"name"`
	},
) (body, head templ.Component, err error) {
	return echo("titled %s", path.Name),
		templ.Raw("<title>" + path.Name + "</title>"), nil
}

// PageReflect is /reflect
//
// A query parameter bound to a Datastar signal. The value in the URL becomes
// the signal's value on load, and the page carries the code that writes the
// signal back into the URL when it changes.
type PageReflect struct{ App *App }

func (PageReflect) GET(
	_ *http.Request,
	query struct {
		Term string `query:"t" reflectsignal:"term"`
		Page int    `query:"p" reflectsignal:"page"`
	},
) (body templ.Component, err error) {
	return echo("term=%q page=%d", query.Term, query.Page), nil
}

// PageMixed is /org/{org}/item/{id}
//
// Path and query on one handler, which is where an off-by-one in either
// parser shows up as the other one's value.
type PageMixed struct{ App *App }

func (PageMixed) GET(
	_ *http.Request,
	path struct {
		Org string `path:"org"`
		ID  int    `path:"id"`
	},
	query struct {
		Tab  string `query:"tab"`
		Page int    `query:"page"`
	},
) (body templ.Component, err error) {
	return echo("org=%q id=%d tab=%q page=%d",
		path.Org, path.ID, query.Tab, query.Page), nil
}

// PageConflict is /c/{value}/{s_value}/{s_s_value}
//
// The URL writer names its locals after the path variables. These three names
// are chosen so that a naive naming scheme produces the same local twice and
// one value lands in two segments.
type PageConflict struct{ App *App }

func (PageConflict) GET(
	_ *http.Request,
	path struct {
		Value   int32  `path:"value"`
		SValue  int32  `path:"s_value"`
		SSValue string `path:"s_s_value"`
	},
) (body templ.Component, err error) {
	return echo("value=%d s_value=%d s_s_value=%q",
		path.Value, path.SValue, path.SSValue), nil
}
