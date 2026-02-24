package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/romshark/templier/engine"
	"github.com/spf13/cobra"
)

func newWatchCmd(stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Start the live-reloading dev server",
		Long: `Start a development server that watches for file changes, rebuilds
the application, and live-reloads the browser tabs. Configuration is read
from the "watch" section of datapages.yaml; sane defaults are used
if the section is missing.`,
	}
	host := cmd.Flags().String("host", "localhost:7331",
		"Host address for the dev server proxy")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runWatch(c.Context(), *host, stderr)
	}
	return cmd
}

func runWatch(ctx context.Context, host string, stderr io.Writer) error {
	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	config, found, err := loadConfig(moduleDir)
	if err != nil {
		return err
	}
	if !found {
		if err := writeDefaultConfig(moduleDir, true); err != nil {
			return err
		}
	}
	if err := runGen(moduleDir, config, stderr); err != nil {
		return err
	}
	w := config.Watch
	if w == nil {
		w = &watchConfig{}
	}
	if w.AppHost == "" {
		w.AppHost = "http://localhost:8080"
	}

	appHost, err := url.Parse(w.AppHost)
	if err != nil {
		return fmt.Errorf("parsing app-host URL: %w", err)
	}

	dirWork := moduleDir
	if w.DirWork != "" {
		dirWork = filepath.Join(moduleDir, w.DirWork)
	}

	conf := engine.Config{
		App: engine.AppConfig{
			DirSrcRoot: moduleDir,
			Exclude:    w.Exclude,
			DirCmd:     "./" + config.Cmd + "/",
			DirWork:    dirWork,
			Flags:      splitFlags(w.Flags),
			Host:       appHost,
		},
		Compiler:       buildCompilerConfig(w.Compiler),
		Debounce:       w.Debounce,
		ProxyTimeout:   w.ProxyTimeout,
		Lint:           w.Lint,
		Format:         w.Format,
		TemplierHost:   host,
		CustomWatchers: mapCustomWatchers(w.CustomWatchers),
		Log: engine.LogConfig{
			Level:            mapLogLevel(w.Log.Level),
			ClearOn:          mapLogClear(w.Log.ClearOn),
			PrintJSDebugLogs: w.Log.PrintJSDebugLogs,
		},
	}
	if w.TLS != nil {
		conf.TLS = &engine.TLSConfig{
			Cert: w.TLS.Cert,
			Key:  w.TLS.Key,
		}
	}

	e, err := engine.New(conf, engine.Options{})
	if err != nil {
		return fmt.Errorf("initializing watch engine: %w", err)
	}

	return e.Run(ctx)
}

func splitFlags(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func buildCompilerConfig(c *watchCompiler) *engine.CompilerConfig {
	if c == nil {
		return nil
	}
	var flags []string
	if c.Gcflags != "" {
		flags = append(flags, "-gcflags", c.Gcflags)
	}
	if c.Ldflags != "" {
		flags = append(flags, "-ldflags", c.Ldflags)
	}
	if c.Asmflags != "" {
		flags = append(flags, "-asmflags", c.Asmflags)
	}
	if len(c.Tags) > 0 {
		flags = append(flags, "-tags", strings.Join(c.Tags, ","))
	}
	if c.Race {
		flags = append(flags, "-race")
	}
	if c.Trimpath {
		flags = append(flags, "-trimpath")
	}
	if c.Msan {
		flags = append(flags, "-msan")
	}
	if c.P > 0 {
		flags = append(flags, "-p", strconv.FormatUint(uint64(c.P), 10))
	}
	var env []string
	for k, v := range c.Env {
		env = append(env, k+"="+v)
	}
	return &engine.CompilerConfig{
		Flags: flags,
		Env:   env,
	}
}

func mapCustomWatchers(watchers []watchCustomWatcher) []engine.CustomWatcherConfig {
	if len(watchers) == 0 {
		return nil
	}
	out := make([]engine.CustomWatcherConfig, len(watchers))
	for i, w := range watchers {
		out[i] = engine.CustomWatcherConfig{
			Name:      w.Name,
			Cmd:       w.Cmd,
			Include:   w.Include,
			Exclude:   w.Exclude,
			Debounce:  w.Debounce,
			FailOnErr: w.FailOnError,
			Requires:  mapWatcherRequires(w.Requires),
		}
	}
	return out
}

func mapLogLevel(l logLevel) engine.LogLevel {
	switch l {
	case logLevelVerbose:
		return engine.LogLevelVerbose
	case logLevelDebug:
		return engine.LogLevelDebug
	default:
		return engine.LogLevelError
	}
}

func mapLogClear(l logClear) engine.LogClearOn {
	switch l {
	case logClearOnRestart:
		return engine.LogClearOnRestart
	case logClearOnFileChange:
		return engine.LogClearOnFileChange
	default:
		return engine.LogClearNever
	}
}

func mapWatcherRequires(r watcherRequires) engine.ActionType {
	switch r {
	case watcherRequiresReload:
		return engine.ActionReload
	case watcherRequiresRestart:
		return engine.ActionRestart
	case watcherRequiresRebuild:
		return engine.ActionRebuild
	default:
		return engine.ActionNone
	}
}
