package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romshark/yamagiconf"
)

// Config holds the datapages.yaml configuration.
type Config struct {
	// Cmd is the path to the server cmd package (default: "cmd/server").
	Cmd string `yaml:"cmd"`

	// Watch is the Templier watch mode settings (optional).
	Watch *WatchConfig `yaml:"watch"`
}

// WatchConfig holds Templier watch mode settings.
type WatchConfig struct {
	AppHost        string               `yaml:"app-host"`
	ProxyTimeout   time.Duration        `yaml:"proxy-timeout"`
	Debounce       time.Duration        `yaml:"debounce"`
	Format         bool                 `yaml:"format"`
	Lint           bool                 `yaml:"lint"`
	Exclude        []string             `yaml:"exclude"`
	WatcherIgnore  []string             `yaml:"watcher-ignore"`
	Flags          string               `yaml:"flags"`
	DirWork        string               `yaml:"dir-work"`
	Log            WatchLog             `yaml:"log"`
	TLS            *WatchTLS            `yaml:"tls"`
	Compiler       *WatchCompiler       `yaml:"compiler"`
	CustomWatchers []WatchCustomWatcher `yaml:"custom-watchers"`
}

// WatchTLS configures TLS for the dev server.
type WatchTLS struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// WatchLog configures watch mode logging.
type WatchLog struct {
	Level            LogLevel `yaml:"level"`
	ClearOn          LogClear `yaml:"clear-on"`
	PrintJSDebugLogs bool     `yaml:"print-js-debug-logs"`
}

// WatchCompiler configures the Go compiler for watch mode.
type WatchCompiler struct {
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

// WatchCustomWatcher configures a custom file watcher.
type WatchCustomWatcher struct {
	Name        string          `yaml:"name"`
	Include     []string        `yaml:"include"`
	Exclude     []string        `yaml:"exclude"`
	Cmd         string          `yaml:"cmd"`
	FailOnError bool            `yaml:"fail-on-error"`
	Debounce    time.Duration   `yaml:"debounce"`
	Requires    WatcherRequires `yaml:"requires"`
}

// enumOption is one accepted text value of a config enum.
type enumOption[T any] struct {
	name  string
	value T
}

// unmarshalEnum resolves text against opts, empty text yields the zero value.
// what names the setting in the error.
func unmarshalEnum[T any](
	text []byte, what string, opts []enumOption[T], into *T,
) error {
	if len(text) == 0 {
		var zero T
		*into = zero
		return nil
	}
	for _, o := range opts {
		if o.name == string(text) {
			*into = o.value
			return nil
		}
	}
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.name
	}
	return fmt.Errorf("invalid %s %q, use: %s", what, text, strings.Join(names, ", "))
}

// LogLevel controls watch mode log verbosity.
type LogLevel int8

const (
	LogLevelErrOnly LogLevel = iota
	LogLevelVerbose
	LogLevelDebug
)

var logLevels = []enumOption[LogLevel]{
	{"erronly", LogLevelErrOnly},
	{"verbose", LogLevelVerbose},
	{"debug", LogLevelDebug},
}

func (l *LogLevel) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, "log level", logLevels, l)
}

// LogClear controls when the console is cleared in watch mode.
type LogClear int8

const (
	LogClearDisabled LogClear = iota
	LogClearOnRestart
	LogClearOnFileChange
)

var logClears = []enumOption[LogClear]{
	{"restart", LogClearOnRestart},
	{"file-change", LogClearOnFileChange},
}

func (l *LogClear) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, "clear-on", logClears, l)
}

// WatcherRequires defines what action a custom watcher triggers.
type WatcherRequires int8

const (
	WatcherRequiresNone WatcherRequires = iota
	WatcherRequiresReload
	WatcherRequiresRestart
	WatcherRequiresRebuild
)

var watcherRequires = []enumOption[WatcherRequires]{
	{"reload", WatcherRequiresReload},
	{"restart", WatcherRequiresRestart},
	{"rebuild", WatcherRequiresRebuild},
}

func (r *WatcherRequires) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, "requires", watcherRequires, r)
}

// Load reads datapages.yml or datapages.yaml from moduleDir.
// If neither file exists, default values are returned and found is false.
// Returns an error if both files exist simultaneously.
func Load(moduleDir string) (c Config, found bool, _ error) {
	var foundName string
	for _, name := range []string{"datapages.yml", "datapages.yaml"} {
		if _, err := os.Stat(filepath.Join(moduleDir, name)); err != nil {
			continue
		}
		if foundName != "" {
			return Config{}, false, fmt.Errorf(
				"ambiguous config: both %s and %s exist; remove one", foundName, name,
			)
		}
		foundName = name
	}
	if foundName != "" {
		if err := yamagiconf.LoadFile(
			filepath.Join(moduleDir, foundName), &c, yamagiconf.WithOptionalPresence(),
		); err != nil {
			return Config{}, false, fmt.Errorf("loading %s: %w", foundName, err)
		}
		found = true
	}
	if c.Cmd == "" {
		c.Cmd = "cmd/server"
	}
	return c, found, nil
}

// DefaultYAML is the default datapages.yaml content.
//
// What the application itself configures is not in here: the app package, the
// destination and the Prometheus option are read from the datapages.NewServer
// call, assets from the Config variable of the app package.
const DefaultYAML = `cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
`

// WriteDefault writes a default datapages.yaml to moduleDir.
func WriteDefault(moduleDir string) error {
	p := filepath.Join(moduleDir, "datapages.yaml")
	return os.WriteFile(p, []byte(DefaultYAML), 0o644)
}
