package app

import "embed"

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS
