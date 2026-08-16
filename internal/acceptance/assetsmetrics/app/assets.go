package app

import "embed"

//go:embed static/*
var StaticFS embed.FS
