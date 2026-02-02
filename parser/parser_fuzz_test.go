package parser_test

import (
	"datapages/parser"
	"os"
	"path/filepath"
	"testing"
)

// FuzzParser tests the parser with randomly generated Go code to catch panics and edge cases.
// This fuzzer creates a minimal valid Go module with fuzzed handler signatures.
func FuzzParser(f *testing.F) {
	// Seed corpus with known interesting cases
	f.Add(`GET(r *http.Request) (body templ.Component, err error)`)
	f.Add(`GET() (body templ.Component, err error)`)
	f.Add(`
		GET(
			r *http.Request,
			sse *datastar.ServerSentEventGenerator,
		) (body templ.Component, err error)`)
	f.Add(`GET(r *http.Request, unknown string) (body templ.Component, err error)`)
	f.Add(`OnEventFoo(event EventFoo, sse *datastar.ServerSentEventGenerator) error`)
	f.Add(`OnEventFoo(event EventFoo) error`)
	f.Add(`
		OnEventFoo(
			event EventFoo,
			sse *datastar.ServerSentEventGenerator,
			extra int,
		) error`)
	f.Add(`OnEventFoo(event EventFoo, notSSE int) error`)
	f.Add(`POSTAction(r *http.Request) error`)
	f.Add(`POSTAction(r *http.Request, sse *datastar.ServerSentEventGenerator) error`)
	f.Add(`POSTAction(r *http.Request, unknown int) error`)

	f.Fuzz(func(t *testing.T, handlerSignature string) {
		// Create a temporary directory for the fuzz test
		tmpDir := t.TempDir()

		// Create a minimal valid go.mod
		goMod := `module fuzztest

go 1.25

require (
	github.com/a-h/templ v0.3.977
	github.com/starfederation/datastar-go v1.1.0
)
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
			t.Skip("failed to write go.mod")
		}

		// Create a Go file with the fuzzed handler signature
		appGo := `package app

import (
	"net/http"
	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

// EventFoo is "foo"
type EventFoo struct {
	Foo string ` + "`json:\"foo\"`" + `
}

func (PageIndex) ` + handlerSignature + ` {
	return
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte(appGo), 0644); err != nil {
			t.Skip("failed to write app.go")
		}

		// The goal is to ensure the parser doesn't panic, regardless of input
		// We don't care if it returns errors - that's expected for invalid signatures
		p := parser.New()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on input %q: %v", handlerSignature, r)
			}
		}()

		// Parse the code - we expect errors for invalid signatures, but never panics
		_, _ = p.Parse(tmpDir)
	})
}

// FuzzParserEventHandlerParams specifically fuzzes event handler parameter counts
// to catch index out of bounds errors.
func FuzzParserEventHandlerParams(f *testing.F) {
	// Seed with different parameter counts
	f.Add(0) // No params
	f.Add(1) // Just event
	f.Add(2) // Event + SSE
	f.Add(3) // Event + SSE + extra
	f.Add(5) // Many params

	f.Fuzz(func(t *testing.T, paramCount int) {
		if paramCount < 0 || paramCount > 10 {
			t.Skip("param count out of reasonable range")
		}

		tmpDir := t.TempDir()

		goMod := `module fuzztest

go 1.25

require (
	github.com/a-h/templ v0.3.977
	github.com/starfederation/datastar-go v1.1.0
)
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
			t.Skip("failed to write go.mod")
		}

		// Build parameter list
		params := ""
		for i := 0; i < paramCount; i++ {
			if i > 0 {
				params += ", "
			}
			switch i {
			case 0:
				params += "event EventFoo"
			case 1:
				params += "sse *datastar.ServerSentEventGenerator"
			default:
				params += "param" + string(rune('A'+i-2)) + " int"
			}
		}

		appGo := `package app

import (
	"net/http"
	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// EventFoo is "foo"
type EventFoo struct {
	Foo string ` + "`json:\"foo\"`" + `
}

func (PageIndex) OnEventFoo(` + params + `) error {
	return nil
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte(appGo), 0644); err != nil {
			t.Skip("failed to write app.go")
		}

		p := parser.New()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked with %d params: %v", paramCount, r)
			}
		}()

		_, _ = p.Parse(tmpDir)
	})
}

// FuzzParserActionHandlerParams specifically fuzzes action handler parameter counts
// to catch index out of bounds errors.
func FuzzParserActionHandlerParams(f *testing.F) {
	// Seed with different parameter counts
	f.Add(0) // No params - should error
	f.Add(1) // Just request
	f.Add(2) // Request + SSE
	f.Add(3) // Request + SSE + extra - should error
	f.Add(5) // Many params - should error

	f.Fuzz(func(t *testing.T, paramCount int) {
		if paramCount < 0 || paramCount > 10 {
			t.Skip("param count out of reasonable range")
		}

		tmpDir := t.TempDir()

		goMod := `module fuzztest

go 1.25

require (
	github.com/a-h/templ v0.3.977
	github.com/starfederation/datastar-go v1.1.0
)
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
			t.Skip("failed to write go.mod")
		}

		// Build parameter list
		params := ""
		for i := 0; i < paramCount; i++ {
			if i > 0 {
				params += ", "
			}
			switch i {
			case 0:
				params += "r *http.Request"
			case 1:
				params += "sse *datastar.ServerSentEventGenerator"
			default:
				params += "param" + string(rune('A'+i-2)) + " int"
			}
		}

		appGo := `package app

import (
	"net/http"
	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageActions is /actions
type PageActions struct{ App *App }

func (PageActions) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// POSTAction is /actions/test
func (PageActions) POSTAction(` + params + `) error {
	return nil
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte(appGo), 0644); err != nil {
			t.Skip("failed to write app.go")
		}

		p := parser.New()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked with %d params: %v", paramCount, r)
			}
		}()

		_, _ = p.Parse(tmpDir)
	})
}
