package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/romshark/templier/engine"
	"github.com/spf13/cobra"
)

func newWatchCmd(stderr io.Writer, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Args:  cobra.NoArgs,
		Short: "Start the live-reloading dev server",
		Long: `Start a development server that watches for file changes, rebuilds
the application, and live-reloads the browser tabs. Configuration is read
from the "watch" section of datapages.yaml; sane defaults are used
if the section is missing.`,
	}
	host := cmd.Flags().String("host", "localhost:7331",
		"Host address for the dev server proxy")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runWatch(c.Context(), *host, stderr, version)
	}
	return cmd
}

func runWatch(ctx context.Context, host string, stderr io.Writer, version string) error {
	// Start the update check immediately so it has maximum time to complete.
	startUpdateCheck(ctx, version, stderr, http.DefaultClient)

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
		// Non-fatal: individual parse errors are already on stderr.
		// The gen watcher will retry and surface errors in the browser on next save.
		_, _ = fmt.Fprintln(stderr, err)
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
		ReconnectMessage: "reconnecting 📡<br>restart datapages watch",
	}
	if w.TLS != nil {
		conf.TLS = &engine.TLSConfig{
			Cert: w.TLS.Cert,
			Key:  w.TLS.Key,
		}
	}

	// Inject a built-in gen watcher so datapages gen is re-run whenever Go files
	// in the app package change, keeping app_gen.go in sync before each rebuild.
	// Parser validation errors (e.g. missing route comment) are shown in the browser.
	// Skip the gen watcher when running as a test binary: test binaries don't
	// implement the gen sub-command and datapages may not be in PATH.
	isTestBinary := false
	if exe, exeErr := os.Executable(); exeErr == nil {
		base := filepath.Base(exe)
		isTestBinary = strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
	}
	if !isTestBinary {
		genExe := "datapages"
		if exe, exeErr := os.Executable(); exeErr == nil {
			genExe = exe
		}
		conf.CustomWatchers = append([]engine.CustomWatcherConfig{
			{
				Name: "datapages gen",
				Include: []string{
					filepath.ToSlash(filepath.Clean(config.App)) + "/**/*.go",
					"datapages.yaml",
					"datapages.yml",
				},
				Cmd:       genExe + " gen",
				FailOnErr: true,
				Requires:  engine.ActionRebuild,
			},
		}, conf.CustomWatchers...)
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
