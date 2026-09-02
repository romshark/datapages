package generator_test

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/model"
)

func parseExampleClassifiedsApp(tb testing.TB) *model.App {
	tb.Helper()
	app, errs := parser.Parse(filepath.Join("..", "..", "example", "classifieds", "app"))
	if errs.Len() > 0 {
		tb.Fatalf("parse errors: %s", errs.Error())
	}
	return app
}

var emptyApp = &model.App{
	PkgPath: "example.com/app",
	Fset:    token.NewFileSet(),
}

// BenchmarkWriteApp measures writing the dispatchers and handlers, against an
// empty model and against example/classifieds as the largest one in this repo.
// The buffer is preallocated, which leaves the codegen itself in the numbers
// rather than the growth of the buffer.
func BenchmarkWriteApp(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WriteApp("datapagesgen", emptyApp)
		}
	})
	b.Run("example/classifieds", func(b *testing.B) {
		m := parseExampleClassifiedsApp(b)
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WriteApp("datapagesgen", m)
		}
	})
}

// BenchmarkWritePkgAction measures writing the action package.
func BenchmarkWritePkgAction(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WritePkgAction(emptyApp)
		}
	})
	b.Run("example/classifieds", func(b *testing.B) {
		m := parseExampleClassifiedsApp(b)
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		b.ResetTimer()
		for b.Loop() {
			w.Reset()
			w.WritePkgAction(m)
		}
	})
}

// BenchmarkWritePkgHref measures writing the href package.
func BenchmarkWritePkgHref(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WritePkgHref(emptyApp)
		}
	})
	b.Run("example/classifieds", func(b *testing.B) {
		m := parseExampleClassifiedsApp(b)
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WritePkgHref(m)
		}
	})
}
