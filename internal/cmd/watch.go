package cmd

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/romshark/templier/engine"
)

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	_ = fs.Parse(args)

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	config, _, err := loadConfig(moduleDir)
	if err != nil {
		return err
	}
	w := config.Watch
	if w == nil {
		w = &watchConfig{}
	}
	if w.Host == "" {
		w.Host = "localhost:7331"
	}
	if w.AppHost == "" {
		w.AppHost = "http://localhost:7332"
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
		TemplierHost:   w.Host,
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

	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer stop()

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
