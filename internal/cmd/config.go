package cmd

import (
	"fmt"
	"time"
)

type config struct {
	// App is the path to the app source package (default: "app").
	App string `yaml:"app"`

	// Gen configures the generated package.
	Gen genConfig `yaml:"gen"`

	// Cmd is the path to the server cmd package (default: "cmd/server").
	Cmd string `yaml:"cmd"`

	// StaticPrefix is the URL path prefix for static files (default: "/static/").
	// Must match the prefix used with WithStaticFS in the server configuration.
	StaticPrefix string `yaml:"static-prefix"`

	// Watch is the Templier watch mode settings (optional).
	Watch *watchConfig `yaml:"watch"`
}

type genConfig struct {
	// Package is the path to the generated package (default: "datapagesgen").
	Package string `yaml:"package"`

	// Prometheus enables Prometheus metrics generation (default: true).
	Prometheus *bool `yaml:"prometheus"`
}

type watchConfig struct {
	AppHost        string               `yaml:"app-host"`
	ProxyTimeout   time.Duration        `yaml:"proxy-timeout"`
	Debounce       time.Duration        `yaml:"debounce"`
	Format         bool                 `yaml:"format"`
	Lint           bool                 `yaml:"lint"`
	Exclude        []string             `yaml:"exclude"`
	Flags          string               `yaml:"flags"`
	DirWork        string               `yaml:"dir-work"`
	Log            watchLog             `yaml:"log"`
	TLS            *watchTLS            `yaml:"tls"`
	Compiler       *watchCompiler       `yaml:"compiler"`
	CustomWatchers []watchCustomWatcher `yaml:"custom-watchers"`
}

type watchTLS struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type watchLog struct {
	Level            logLevel `yaml:"level"`
	ClearOn          logClear `yaml:"clear-on"`
	PrintJSDebugLogs bool     `yaml:"print-js-debug-logs"`
}

type watchCompiler struct {
	Gcflags  string            `yaml:"gcflags"`
	Ldflags  string            `yaml:"ldflags"`
	Asmflags string            `yaml:"asmflags"`
	Tags     []string          `yaml:"tags"`
	Race     bool              `yaml:"race"`
	Trimpath bool              `yaml:"trimpath"`
	Msan     bool              `yaml:"msan"`
	P        uint32            `yaml:"p"`
	Env      map[string]string `yaml:"env"`
}

type watchCustomWatcher struct {
	Name        string          `yaml:"name"`
	Include     []string        `yaml:"include"`
	Exclude     []string        `yaml:"exclude"`
	Cmd         string          `yaml:"cmd"`
	FailOnError bool            `yaml:"fail-on-error"`
	Debounce    time.Duration   `yaml:"debounce"`
	Requires    watcherRequires `yaml:"requires"`
}

// logLevel controls watch mode log verbosity.
type logLevel int8

const (
	logLevelErrOnly logLevel = iota
	logLevelVerbose
	logLevelDebug
)

func (l *logLevel) UnmarshalText(text []byte) error {
	switch string(text) {
	case "", "erronly":
		*l = logLevelErrOnly
	case "verbose":
		*l = logLevelVerbose
	case "debug":
		*l = logLevelDebug
	default:
		return fmt.Errorf(
			"invalid log level %q, use: erronly, verbose, debug", string(text),
		)
	}
	return nil
}

// logClear controls when the console is cleared in watch mode.
type logClear int8

const (
	logClearDisabled logClear = iota
	logClearOnRestart
	logClearOnFileChange
)

func (l *logClear) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*l = logClearDisabled
	case "restart":
		*l = logClearOnRestart
	case "file-change":
		*l = logClearOnFileChange
	default:
		return fmt.Errorf(
			"invalid clear-on %q, use: restart, file-change", string(text),
		)
	}
	return nil
}

// watcherRequires defines what action a custom watcher triggers.
type watcherRequires int8

const (
	watcherRequiresNone watcherRequires = iota
	watcherRequiresReload
	watcherRequiresRestart
	watcherRequiresRebuild
)

func (r *watcherRequires) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*r = watcherRequiresNone
	case "reload":
		*r = watcherRequiresReload
	case "restart":
		*r = watcherRequiresRestart
	case "rebuild":
		*r = watcherRequiresRebuild
	default:
		return fmt.Errorf(
			"invalid requires %q, use: reload, restart, rebuild", string(text),
		)
	}
	return nil
}
