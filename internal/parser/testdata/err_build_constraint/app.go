// Package app has files that build constraints restrict, which are reported
// on every platform: probe_windows.go by its name and unix.go by its
// //go:build line. gen.go is in no build, and probe_windows_test.go is a test
// file, which generated code does not read.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
