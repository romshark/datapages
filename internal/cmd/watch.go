package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/romshark/templier/engine"
	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/serverscan"
)

func newWatchCmd(stderr io.Writer, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Args:  cobra.NoArgs,
		Short: "Start the live-reloading dev server",
		Long: `Start a development server that watches for file changes, rebuilds
the application, and live-reloads the browser tabs. Configuration is read
from the "watch" section of datapages.yaml; sane defaults are used
if the section is missing.

One dev server runs one application. A module that builds more than one
needs --app to say which, naming the app package: "datapages watch --app frontend".`,
	}
	host := cmd.Flags().String("host", "localhost:7331",
		"Host address for the dev server proxy")
	app := cmd.Flags().String("app", "",
		"App package to run, required when the module builds more than one")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runWatch(c.Context(), *host, *app, stderr, version)
	}
	return cmd
}

// selectApp picks the app the dev server runs.
// A module building one needs no flag; one building more must name it.
func selectApp(r serverscan.Result, name string) (serverscan.App, error) {
	if name != "" {
		a, ok := r.Find(name)
		if !ok {
			return serverscan.App{}, fmt.Errorf(
				"no app package %q in this module, it builds: %s",
				name, strings.Join(r.Names(), ", "),
			)
		}
		return a, nil
	}
	if len(r.Apps) == 1 {
		return r.Apps[0], nil
	}
	return serverscan.App{}, fmt.Errorf(
		"this module builds %d applications (%s), name the one to run with --app",
		len(r.Apps), strings.Join(r.Names(), ", "),
	)
}

func runWatch(
	ctx context.Context, host, appName string, stderr io.Writer, version string,
) error {
	// Start the update check immediately so it has maximum time to complete.
	startUpdateCheck(ctx, version, stderr, http.DefaultClient)

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	cfg, found, err := config.Load(moduleDir)
	if err != nil {
		return err
	}
	if !found {
		if err := config.WriteDefault(moduleDir); err != nil {
			return err
		}
	}
	if err := runGen(moduleDir, cfg, false, stderr, version); err != nil {
		// Non-fatal: individual parse errors are already on stderr.
		// The gen watcher will retry and surface errors in the browser on next save.
		_, _ = fmt.Fprintln(stderr, err)
	}
	// The app packages are whatever the NewServer calls name, which the run
	// above has just made resolvable.
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}
	scan, err := serverscan.Scan(moduleDir, modulePath)
	if err != nil {
		return err
	}
	app, err := selectApp(scan, appName)
	if err != nil {
		return err
	}
	// The entry point is the command the call is written in. A module that
	// keeps none falls back to the configured cmd package.
	cmdDir, ok := app.Cmd()
	if !ok {
		cmdDir = cfg.Cmd
	}
	w := cfg.Watch
	if w == nil {
		w = &config.WatchConfig{}
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

	// Templier v0.11.7 stores conf.Log.Level but never applies it to its
	// slog logger, so setting `watch.log.level: debug` in datapages.yaml
	// has no effect on its own. Configure slog.Default() here so templier's
	// logger.Debug calls (which tell us what the file-watcher/reload
	// broadcaster are doing) actually render.
	if w.Log.Level == config.LogLevelDebug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	engineConf := engine.Config{
		App: engine.AppConfig{
			DirSrcRoot: moduleDir,
			Exclude:    w.Exclude,
			DirCmd:     "./" + filepath.ToSlash(cmdDir) + "/",
			DirWork:    dirWork,
			Flags:      splitFlags(w.Flags),
			Host:       appHost,
		},
		Compiler:       buildCompilerConfig(w.Compiler),
		WatcherIgnore:  w.WatcherIgnore,
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
		engineConf.TLS = &engine.TLSConfig{
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
		engineConf.CustomWatchers = append([]engine.CustomWatcherConfig{
			{
				Name:    "datapages gen",
				Include: watchInclude(scan),
				// The generated packages sit under the app packages they
				// belong to. Without this a run of gen would trigger gen.
				Exclude:   watchExclude(scan),
				Cmd:       genExe + " gen",
				FailOnErr: true,
				Requires:  engine.ActionRebuild,
			},
		}, engineConf.CustomWatchers...)
	}

	e, err := engine.New(engineConf, engine.Options{})
	if err != nil {
		return fmt.Errorf("initializing watch engine: %w", err)
	}

	err = e.Run(ctx)
	// Without this a second `datapages watch` would half-start
	// (Go process alive, HTTP server not bound)
	if err != nil && errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("%s is already in use (another `datapages watch` running?)",
			host)
	}
	return err
}

// genWatcherCmd is the shell line that re-runs gen on a file change.
//
// Templier validates the first word of the line with [os/exec.LookPath] and
// runs the line through `sh -c`. "env" is a program of its own, which passes that
// check however the path of this binary reads, and it execs the quoted path whole.
// The path as the first word splits at its first space.
func genWatcherCmd(exe string) string {
	return "env " + shQuote(exe) + " gen"
}

// shQuote wraps s as one word of a shell command. A single quote ends the quoting,
// escapes itself and reopens it, which is the only form `sh` takes.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func splitFlags(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func buildCompilerConfig(c *config.WatchCompiler) *engine.CompilerConfig {
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

func mapCustomWatchers(watchers []config.WatchCustomWatcher) []engine.CustomWatcherConfig {
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

// An unmapped value yields the engine zero value, which is the default of each enum:
// LogLevelError, LogClearNever and ActionNone.
var (
	engineLogLevel = map[config.LogLevel]engine.LogLevel{
		config.LogLevelErrOnly: engine.LogLevelError,
		config.LogLevelVerbose: engine.LogLevelVerbose,
		config.LogLevelDebug:   engine.LogLevelDebug,
	}
	engineLogClear = map[config.LogClear]engine.LogClearOn{
		config.LogClearDisabled:     engine.LogClearNever,
		config.LogClearOnRestart:    engine.LogClearOnRestart,
		config.LogClearOnFileChange: engine.LogClearOnFileChange,
	}
	engineAction = map[config.WatcherRequires]engine.ActionType{
		config.WatcherRequiresNone:    engine.ActionNone,
		config.WatcherRequiresReload:  engine.ActionReload,
		config.WatcherRequiresRestart: engine.ActionRestart,
		config.WatcherRequiresRebuild: engine.ActionRebuild,
	}
)

func mapLogLevel(l config.LogLevel) engine.LogLevel { return engineLogLevel[l] }

func mapLogClear(l config.LogClear) engine.LogClearOn { return engineLogClear[l] }

func mapWatcherRequires(r config.WatcherRequires) engine.ActionType {
	return engineAction[r]
}

// watchInclude lists what a change to re-runs "datapages gen" for:
// the Go sources of every app package of the module.
func watchInclude(r serverscan.Result) []string {
	include := make([]string, 0, len(r.Apps)+2)
	for _, a := range r.Apps {
		include = append(include, filepath.ToSlash(a.Dir)+"/**/*.go")
	}
	return append(include, "datapages.yaml", "datapages.yml")
}

// watchExclude holds the generated packages out of what triggers a run.
// They sit under the app packages, so a run of gen writes into what the
// gen watcher watches, which would trigger it again without this.
func watchExclude(r serverscan.Result) []string {
	exclude := make([]string, 0, len(r.Apps))
	for _, a := range r.Apps {
		exclude = append(exclude, filepath.ToSlash(a.GenDir)+"/**")
	}
	return exclude
}
