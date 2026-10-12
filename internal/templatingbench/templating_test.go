package templatingbench

//go:generate templ generate
//go:generate qtc -dir=.

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"testing"

	"github.com/CloudyKit/jet/v6"
	"github.com/stretchr/testify/require"
	g "maragu.dev/gomponents"
	ghtml "maragu.dev/gomponents/html"
)

// benchName is the request data every benchmark renders. It's a variable,
// as request data would be. Given a constant, the compiler converts the data
// html/template receives to an interface without allocating.
var benchName = "World"

type helloData struct{ Name string }

var stdTmpl = template.Must(template.New("hello").Parse(
	`<div><h1>Hello, {{.Name}}!</h1><p>Welcome to the benchmark.</p></div>`,
))

var (
	jetSet     = jet.NewSet(jet.NewInMemLoader())
	jetTmpl, _ = jetSet.Parse(
		"hello",
		`<div><h1>Hello, {{.Name}}!</h1><p>Welcome to the benchmark.</p></div>`,
	)
)

// helloGomponents writes name as its own text node. g.Textf would run
// fmt.Sprintf per request.
func helloGomponents(name string) g.Node {
	return ghtml.Div(
		ghtml.H1(g.Text("Hello, "), g.Text(name), g.Text("!")),
		ghtml.P(g.Text("Welcome to the benchmark.")),
	)
}

// renderFunc does all the work of 1 request. It passes name to the engine,
// builds what the engine renders and writes the markup to w. It leaves out template
// parsing, which a server does once at startup. Where an engine documents several
// ways to pass the data, its renderFunc uses the fastest.
type renderFunc func(w io.Writer, name string) error

func renderStd(w io.Writer, name string) error {
	return stdTmpl.Execute(w, helloData{Name: name})
}

func renderTempl(w io.Writer, name string) error {
	return Hello(name).Render(context.Background(), w)
}

func renderQuicktemplate(w io.Writer, name string) error {
	WriteHelloQT(w, name)
	return nil
}

func renderGomponents(w io.Writer, name string) error {
	return helloGomponents(name).Render(w)
}

// renderJet passes the data as the data argument of Execute.
// A [jet.VarMap] would cost a new map per request.
func renderJet(w io.Writer, name string) error {
	return jetTmpl.Execute(w, nil, helloData{Name: name})
}

// BenchmarkTemplatingStd measures html/template, the baseline the others in this
// file are compared against in FAQ.md.
func BenchmarkTemplatingStd(b *testing.B) {
	for b.Loop() {
		if err := renderStd(io.Discard, benchName); err != nil {
			panic(err)
		}
	}
}

// BenchmarkTemplatingTempl measures a-h/templ, which is what Datapages renders with.
func BenchmarkTemplatingTempl(b *testing.B) {
	for b.Loop() {
		if err := renderTempl(io.Discard, benchName); err != nil {
			panic(err)
		}
	}
}

// BenchmarkTemplatingQuicktemplate measures valyala/quicktemplate.
func BenchmarkTemplatingQuicktemplate(b *testing.B) {
	for b.Loop() {
		if err := renderQuicktemplate(io.Discard, benchName); err != nil {
			panic(err)
		}
	}
}

// BenchmarkTemplatingGomponents measures maragu.dev/gomponents.
func BenchmarkTemplatingGomponents(b *testing.B) {
	for b.Loop() {
		if err := renderGomponents(io.Discard, benchName); err != nil {
			panic(err)
		}
	}
}

// BenchmarkTemplatingJet measures CloudyKit/jet.
func BenchmarkTemplatingJet(b *testing.B) {
	for b.Loop() {
		if err := renderJet(io.Discard, benchName); err != nil {
			panic(err)
		}
	}
}

// TestTemplating tests that every engine benchmarked here renders the same markup
// and escapes name the same way.
// A benchmark of a template that produces something else would compare nothing.
func TestTemplating(t *testing.T) {
	for input, tc := range map[string]struct{ name, want string }{
		"plain": {
			name: benchName,
			want: `<div><h1>Hello, World!</h1><p>Welcome to the benchmark.</p></div>`,
		},
		"escaped": {
			name: "<b>Tom & Jerry</b>",
			want: `<div><h1>Hello, &lt;b&gt;Tom &amp; Jerry&lt;/b&gt;!</h1>` +
				`<p>Welcome to the benchmark.</p></div>`,
		},
	} {
		t.Run(input, func(t *testing.T) {
			for engine, render := range map[string]renderFunc{
				"html/template":         renderStd,
				"a-h/templ":             renderTempl,
				"valyala/quicktemplate": renderQuicktemplate,
				"maragu.dev/gomponents": renderGomponents,
				"CloudyKit/jet":         renderJet,
			} {
				t.Run(engine, func(t *testing.T) {
					var buf bytes.Buffer
					require.NoError(t, render(&buf, tc.name))
					require.Equal(t, tc.want, buf.String())
				})
			}
		})
	}
}
