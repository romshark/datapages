package bench

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

var stdTmpl = template.Must(template.New("hello").Parse(
	`<div><h1>Hello, {{.Name}}!</h1><p>Welcome to the benchmark.</p></div>`,
))

func BenchmarkTemplatingStd(b *testing.B) {
	data := struct{ Name string }{Name: "World"}
	b.ReportAllocs()
	for b.Loop() {
		if err := stdTmpl.Execute(io.Discard, data); err != nil {
			panic(err)
		}
	}
}

func BenchmarkTemplatingTempl(b *testing.B) {
	ctx := context.Background()
	c := Hello("World")
	b.ReportAllocs()
	for b.Loop() {
		if err := c.Render(ctx, io.Discard); err != nil {
		}
	}
}

func TestTemplating(t *testing.T) {
	var stdBuf, templBuf bytes.Buffer

	err := stdTmpl.Execute(&stdBuf, struct{ Name string }{Name: "World"})
	require.NoError(t, err)

	err = Hello("World").Render(context.Background(), &templBuf)
	require.NoError(t, err)

	require.Equal(t, stdBuf.String(), templBuf.String())
}
