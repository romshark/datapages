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
	"path"
	"path/filepath"
	"slices"
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

1 dev server runs 1 application. A module that builds more than 1
needs --app to say which, the directory of the app package relative to
the module root, as in "datapages watch --app app/frontend".`,
	}
	host := cmd.Flags().String("host", "localhost:7331",
		"Host address for the dev server proxy")
	app := cmd.Flags().String("app", "",
		"Directory of the app package to run, relative to the module root, "+
			"required when the module builds more than 1")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runWatch(c.Context(), *host, *app, stderr, version)
	}
	return cmd
}

// selectApp picks the app the dev server runs. A module building 1 needs no
// flag, one building more the directory of the app package to run.
func selectApp(r serverscan.Result, dir string) (serverscan.App, error) {
	if dir != "" {
		a, ok := r.Find(dir)
		if !ok {
			return serverscan.App{}, fmt.Errorf(
				"no app package in %q, the module builds: %s",
				dir, strings.Join(r.Dirs(), ", "),
			)
		}
		return a, nil
	}
	if len(r.Apps) == 1 {
		return r.Apps[0], nil
	}
	return serverscan.App{}, fmt.Errorf(
		"this module builds %d applications (%s), "+
			"pass the directory of the one to run with --app",
		len(r.Apps), strings.Join(r.Dirs(), ", "),
	)
}

func runWatch(
	ctx context.Context, host, appDir string, stderr io.Writer, version string,
) error {
	// Start the update check immediately so it has maximum time to complete.
	startUpdateCheck(ctx, version, stderr, http.DefaultClient)

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	releaseLock, err := acquireWatchLock(moduleDir, host)
	if err != nil {
		// The development server can run without the lock. Warn because a
		// concurrent "templ generate" can delete its temporary template files.
		_, _ = fmt.Fprintln(stderr, "warning:", err)
	}
	defer releaseLock()

	cfg, found, err := config.Load(moduleDir)
	if err != nil {
		return err
	}
	if !found {
		if err := config.WriteDefault(moduleDir); err != nil {
			return err
		}
	}
	if err := runGen(
		moduleDir, cfg, scaffold{prometheus: scaffoldPrometheus}, stderr, version,
	); err != nil {
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
	app, err := selectApp(scan, appDir)
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

	appHost, err := parseAppHost(w.AppHost)
	if err != nil {
		return err
	}

	dirWork := moduleDir
	if w.DirWork != "" {
		dirWork = filepath.Join(moduleDir, w.DirWork)
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
		engineConf.CustomWatchers = append(
			[]engine.CustomWatcherConfig{genWatcher(genExe, scan)},
			engineConf.CustomWatchers...,
		)
	}

	// templier enables templ development mode for the whole process.
	// Without a watch root, templ loads dependency components from development-mode
	// files that templier doesn't write, which makes rendering fail.
	// TEMPL_DEV_MODE_WATCH_ROOT limits development-mode loading to this module.
	//
	// [templ watch mode]: https://github.com/a-h/templ/blob/main/runtime/watchmode.go
	if err := os.Setenv("TEMPL_DEV_MODE_WATCH_ROOT", moduleDir); err != nil {
		return fmt.Errorf("setting templ watch root: %w", err)
	}

	e, err := engine.New(engineConf, engine.Options{
		Logger: newWatchLogger(w.Log.Level, stderr),
	})
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

// newWatchLogger returns the logger of the watch engine at the level of
// watch.log.level, mapped as the templier CLI maps it. The engine reads no
// level of its own and falls back to [slog.Default], which logs info.
func newWatchLogger(level config.LogLevel, w io.Writer) *slog.Logger {
	l := slog.LevelError
	switch level {
	case config.LogLevelVerbose:
		l = slog.LevelInfo
	case config.LogLevelDebug:
		l = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: l}))
}

// parseAppHost parses watch.app-host, the URL the dev server proxies to.
func parseAppHost(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parsing watch.app-host: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("watch.app-host %q must be an http or https URL "+
			"with a host, such as http://localhost:8080", s)
	}
	return u, nil
}

// genWatcher is the custom watcher that re-runs gen on a file change.
func genWatcher(
	genExe string, scan serverscan.Result,
) engine.CustomWatcherConfig {
	return engine.CustomWatcherConfig{
		Name:    "datapages gen",
		Include: watchInclude(scan),
		// The generated packages sit under the app packages they belong to.
		// Without this a run of gen would trigger gen.
		Exclude:   watchExclude(scan),
		Cmd:       genWatcherCmd(genExe),
		FailOnErr: true,
		Requires:  engine.ActionRebuild,
	}
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

// shQuote wraps s as 1 word of a shell command. A single quote ends the quoting,
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
// the Go sources of every app package of the module, and those of the packages
// calling datapages.NewServer. gen reads the Metrics type argument of a call
// and generates no code for a call naming the wrong session type. A fix to the
// call has to regenerate.
func watchInclude(r serverscan.Result) []string {
	include := make([]string, 0, len(r.Apps)+2)
	for _, a := range r.Apps {
		include = append(include, filepath.ToSlash(a.Dir)+"/**/*.go")
		for _, c := range a.Calls {
			p := path.Join(filepath.ToSlash(c.Dir), "*.go")
			if !slices.Contains(include, p) {
				include = append(include, p)
			}
		}
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
