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

var stdTmpl = template.Must(template.New("hello").Parse(
	`<div><h1>Hello, {{.Name}}!</h1><p>Welcome to the benchmark.</p></div>`,
))

var (
	jetSet     = jet.NewSet(jet.NewInMemLoader())
	jetTmpl, _ = jetSet.Parse("hello",
		`<div><h1>Hello, {{name}}!</h1><p>Welcome to the benchmark.</p></div>`,
	)
)

func helloGomponents(name string) g.Node {
	return ghtml.Div(
		ghtml.H1(g.Textf("Hello, %s!", name)),
		ghtml.P(g.Text("Welcome to the benchmark.")),
	)
}

func BenchmarkTemplatingStd(b *testing.B) {
	data := struct{ Name string }{Name: "World"}
	for b.Loop() {
		if err := stdTmpl.Execute(io.Discard, data); err != nil {
			panic(err)
		}
	}
}

func BenchmarkTemplatingTempl(b *testing.B) {
	ctx := context.Background()
	c := Hello("World")
	for b.Loop() {
		if err := c.Render(ctx, io.Discard); err != nil {
			panic(err)
		}
	}
}

func BenchmarkTemplatingQuicktemplate(b *testing.B) {
	for b.Loop() {
		WriteHelloQT(io.Discard, "World")
	}
}

func BenchmarkTemplatingGomponents(b *testing.B) {
	c := helloGomponents("World")
	for b.Loop() {
		if err := c.Render(io.Discard); err != nil {
			panic(err)
		}
	}
}

func BenchmarkTemplatingJet(b *testing.B) {
	vars := make(jet.VarMap)
	vars.Set("name", "World")
	for b.Loop() {
		if err := jetTmpl.Execute(io.Discard, vars, nil); err != nil {
			panic(err)
		}
	}
}

func TestTemplating(t *testing.T) {
	var bufStd, buf2 bytes.Buffer

	err := stdTmpl.Execute(&bufStd, struct{ Name string }{Name: "World"})
	require.NoError(t, err)
	expected := bufStd.String()

	for name, render := range map[string]func(w io.Writer) error{
		"a-h/templ": func(w io.Writer) error {
			return Hello("World").Render(context.Background(), w)
		},
		"valyala/quicktemplate": func(w io.Writer) error {
			WriteHelloQT(w, "World")
			return nil
		},
		"maragu.dev/gomponents": func(w io.Writer) error {
			return helloGomponents("World").Render(w)
		},
		"CloudyKit/jet": func(w io.Writer) error {
			vars := make(jet.VarMap)
			vars.Set("name", "World")
			return jetTmpl.Execute(w, vars, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			buf2.Reset()
			require.NoError(t, render(&buf2), name)
			require.Equal(t, expected, buf2.String(), name)
		})
	}
}
