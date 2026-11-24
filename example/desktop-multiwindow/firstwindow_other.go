//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// openFirstWindow calls open. Only macOS needs to wait for the app to be active.
func openFirstWindow(_ *application.App, open func()) { open() }
