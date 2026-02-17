package generator_test

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/model"
)

func parseExampleClassifiedsApp(tb testing.TB) *model.App {
	tb.Helper()
	app, errs := parser.Parse(filepath.Join("..", "example", "classifieds", "app"))
	if errs.Len() > 0 {
		tb.Fatalf("parse errors: %s", errs.Error())
	}
	return app
}

var emptyApp = &model.App{
	PkgPath: "example.com/app",
	Fset:    token.NewFileSet(),
}

func BenchmarkWriteApp(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WriteApp(emptyApp)
		}
	})
	b.Run("example/classifieds", func(b *testing.B) {
		m := parseExampleClassifiedsApp(b)
		w := generator.Writer{Buf: make([]byte, 2*1024*1024)} // 2 MiB
		for b.Loop() {
			w.Reset()
			w.WriteApp(m)
		}
	})
}

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
