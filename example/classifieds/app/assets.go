package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var embedFS embed.FS

// FSStaticDev returns an http.FileSystem that reads from the filesystem.
// Use this in development mode for live reloading without server recompilation.
func FSStaticDev() http.FileSystem {
	return http.Dir("./app/static")
}

// FSStatic returns an http.FileSystem from the embedded filesystem.
// Use this in production mode.
func FSStatic() (http.FileSystem, error) {
	subFS, err := fs.Sub(embedFS, "static")
	if err != nil {
		return nil, err
	}
	return http.FS(subFS), nil
}
