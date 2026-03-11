package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romshark/yamagiconf"
)

// Config holds the datapages.yaml configuration.
type Config struct {
	// App is the path to the app source package (default: "app").
	App string `yaml:"app"`

	// Gen configures the generated package.
	Gen GenConfig `yaml:"gen"`

	// Cmd is the path to the server cmd package (default: "cmd/server").
	Cmd string `yaml:"cmd"`

	// Assets configures embedded static asset file serving (optional).
	// When nil, asset serving is disabled. When set, both URLPrefix and Dir
	// are required.
	Assets *AssetsConfig `yaml:"assets"`

	// Watch is the Templier watch mode settings (optional).
	Watch *WatchConfig `yaml:"watch"`
}

// GenConfig configures the generated package.
type GenConfig struct {
	// Package is the path to the generated package (default: "datapagesgen").
	Package string `yaml:"package"`

	// Prometheus enables Prometheus metrics generation (default: true).
	Prometheus *bool `yaml:"prometheus"`
}

// AssetsConfig configures embedded static asset file serving.
type AssetsConfig struct {
	// URLPrefix is the URL path prefix for serving embedded static files.
	// Must start and end with '/'. Example: "/static/".
	URLPrefix string `yaml:"url-prefix"`

	// Dir is the on-disk path to the embedded static files directory, relative to
	// the module root. Example: "./app/static/".
	Dir string `yaml:"dir"`
}

// WatchConfig holds Templier watch mode settings.
type WatchConfig struct {
	AppHost        string               `yaml:"app-host"`
	ProxyTimeout   time.Duration        `yaml:"proxy-timeout"`
	Debounce       time.Duration        `yaml:"debounce"`
	Format         bool                 `yaml:"format"`
	Lint           bool                 `yaml:"lint"`
	Exclude        []string             `yaml:"exclude"`
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

// LogLevel controls watch mode log verbosity.
type LogLevel int8

const (
	LogLevelErrOnly LogLevel = iota
	LogLevelVerbose
	LogLevelDebug
)

func (l *LogLevel) UnmarshalText(text []byte) error {
	switch string(text) {
	case "", "erronly":
		*l = LogLevelErrOnly
	case "verbose":
		*l = LogLevelVerbose
	case "debug":
		*l = LogLevelDebug
	default:
		return fmt.Errorf(
			"invalid log level %q, use: erronly, verbose, debug", string(text),
		)
	}
	return nil
}

// LogClear controls when the console is cleared in watch mode.
type LogClear int8

const (
	LogClearDisabled LogClear = iota
	LogClearOnRestart
	LogClearOnFileChange
)

func (l *LogClear) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*l = LogClearDisabled
	case "restart":
		*l = LogClearOnRestart
	case "file-change":
		*l = LogClearOnFileChange
	default:
		return fmt.Errorf(
			"invalid clear-on %q, use: restart, file-change", string(text),
		)
	}
	return nil
}

// WatcherRequires defines what action a custom watcher triggers.
type WatcherRequires int8

const (
	WatcherRequiresNone WatcherRequires = iota
	WatcherRequiresReload
	WatcherRequiresRestart
	WatcherRequiresRebuild
)

func (r *WatcherRequires) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*r = WatcherRequiresNone
	case "reload":
		*r = WatcherRequiresReload
	case "restart":
		*r = WatcherRequiresRestart
	case "rebuild":
		*r = WatcherRequiresRebuild
	default:
		return fmt.Errorf(
			"invalid requires %q, use: reload, restart, rebuild", string(text),
		)
	}
	return nil
}

// Sentinel errors for assets validation.
var (
	ErrAssetsDirRequired = errors.New(
		"assets.dir is required when embedded asset serving is enabled")
	ErrAssetsURLPrefixRequired = errors.New(
		"assets.url-prefix is required when embedded asset serving is enabled")
	ErrAssetsURLPrefixNoLeadingSlash = errors.New(
		"assets.url-prefix must start with '/'")
	ErrAssetsURLPrefixNoTrailingSlash = errors.New(
		"assets.url-prefix must end with '/'")
	ErrAssetsURLPrefixDoubleSlash = errors.New(
		"assets.url-prefix must not contain double slashes")
	ErrAssetsURLPrefixQueryString = errors.New(
		"assets.url-prefix must not contain a query string")
	ErrAssetsURLPrefixFragment = errors.New(
		"assets.url-prefix must not contain a fragment")
	ErrAssetsURLPrefixDotSegment = errors.New(
		"assets.url-prefix must not contain dot segments")
	ErrAssetsURLPrefixBackslash = errors.New(
		"assets.url-prefix must not contain backslashes")
	ErrAssetsURLPrefixEncodedTraversal = errors.New(
		"assets.url-prefix must not contain percent-encoded dots, " +
			"slashes, or backslashes")
	ErrAssetsURLPrefixRoot = errors.New(
		"assets.url-prefix must not be \"/\"; it would conflict with page routes")
	ErrAssetsURLPrefixInvalidChar = errors.New(
		"assets.url-prefix contains invalid characters; " +
			"use only ASCII letters, digits, hyphens, underscores, and slashes")
)

// ValidateAssetsURLPrefix checks that s is a valid URL path prefix for embedded files.
func ValidateAssetsURLPrefix(s string) error {
	if !strings.HasPrefix(s, "/") {
		return ErrAssetsURLPrefixNoLeadingSlash
	}
	if !strings.HasSuffix(s, "/") {
		return ErrAssetsURLPrefixNoTrailingSlash
	}
	if s == "/" {
		return ErrAssetsURLPrefixRoot
	}
	if strings.Contains(s, "//") {
		return ErrAssetsURLPrefixDoubleSlash
	}
	if strings.Contains(s, "?") {
		return ErrAssetsURLPrefixQueryString
	}
	if strings.Contains(s, "#") {
		return ErrAssetsURLPrefixFragment
	}
	if strings.Contains(s, "/.") {
		return ErrAssetsURLPrefixDotSegment
	}
	if strings.Contains(s, `\`) {
		return ErrAssetsURLPrefixBackslash
	}
	if i := strings.Index(s, "%"); i >= 0 {
		if err := checkPercentEncoding(s[i:]); err != nil {
			return err
		}
	}
	for i := range len(s) {
		c := s[i]
		if c <= ' ' || c >= 0x7f || c == '{' || c == '}' ||
			c == '<' || c == '>' || c == '|' || c == '^' || c == '`' {
			return ErrAssetsURLPrefixInvalidChar
		}
	}
	return nil
}

// checkPercentEncoding scans s (starting from the first '%') for percent-encoded
// sequences and rejects encoded dots (%2e/%2E), slashes (%2f/%2F), and
// backslashes (%5c/%5C) that could bypass path traversal checks.
func checkPercentEncoding(s string) error {
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+2 >= len(s) {
			return ErrAssetsURLPrefixInvalidChar
		}
		hi, lo := s[i+1], s[i+2]
		if !isHexDigit(hi) || !isHexDigit(lo) {
			return ErrAssetsURLPrefixInvalidChar
		}
		upper := strings.ToUpper(string([]byte{hi, lo}))
		if upper == "2E" || upper == "2F" || upper == "5C" {
			return ErrAssetsURLPrefixEncodedTraversal
		}
		i += 2
	}
	return nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// ErrNoConfig is returned when no datapages.yaml is found.
var ErrNoConfig = fmt.Errorf(
	"no datapages.yaml found; run `datapages init` to create a project",
)

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
	if c.App == "" {
		c.App = "app"
	}
	if c.Gen.Package == "" {
		c.Gen.Package = "datapagesgen"
	}
	if c.Cmd == "" {
		c.Cmd = "cmd/server"
	}
	if c.Gen.Prometheus == nil {
		v := true
		c.Gen.Prometheus = &v
	}
	if c.Assets != nil {
		if c.Assets.URLPrefix == "" {
			return Config{}, false, ErrAssetsURLPrefixRequired
		}
		if c.Assets.Dir == "" {
			return Config{}, false, ErrAssetsDirRequired
		}
		if err := ValidateAssetsURLPrefix(c.Assets.URLPrefix); err != nil {
			return Config{}, false, err
		}
	}
	return c, found, nil
}

// DefaultYAML returns the default datapages.yaml content.
func DefaultYAML(prometheus bool) string {
	return fmt.Sprintf(`app: app
gen:
  package: datapagesgen
  prometheus: %t
cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
`, prometheus)
}

// WriteDefault writes a default datapages.yaml to moduleDir.
func WriteDefault(moduleDir string, prometheus bool) error {
	p := filepath.Join(moduleDir, "datapages.yaml")
	return os.WriteFile(p, []byte(DefaultYAML(prometheus)), 0o644)
}
