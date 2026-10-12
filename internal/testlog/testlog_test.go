package testlog_test

import (
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/testlog"
)

// TestLines tests that each record comes back as 1 line without its time,
// a line feed in a value and the attributes of [slog.Logger.With] included,
// and that a record below info is dropped.
func TestLines(t *testing.T) {
	t.Parallel()
	var b testlog.Buffer
	require.Nil(t, b.Lines())

	log := b.Logger()
	log.Debug("dropped")
	log.Info("started", slog.String("addr", "127.0.0.1:80"))
	log.With(slog.String("handler", "page")).
		Error("recovered panic", slog.String("stack", "a\nb"))

	require.Equal(t, []string{
		`level=INFO msg=started addr=127.0.0.1:80`,
		`level=ERROR msg="recovered panic" handler=page stack="a\nb"`,
	}, b.Lines())
}

// failT records a failure instead of ending the test.
type failT struct {
	testing.TB
	failed bool
	msg    string
}

func (t *failT) Fatalf(format string, args ...any) {
	t.failed = true
	t.msg = fmt.Sprintf(format, args...)
}

// TestRequire tests that each helper fails the test exactly when the log lacks
// what it asks for.
func TestRequire(t *testing.T) {
	t.Parallel()
	var b testlog.Buffer
	b.Logger().Info("started", slog.String("addr", "127.0.0.1:80"))
	const line = "level=INFO msg=started addr=127.0.0.1:80"

	for name, tt := range map[string]struct {
		check    func(t testing.TB)
		wantFail bool
	}{
		"lines": {
			check: func(t testing.TB) { b.RequireLines(t, line) },
		},
		"lines, 1 more": {
			check:    func(t testing.TB) { b.RequireLines(t, line, line) },
			wantFail: true,
		},
		"lines, none": {
			check:    func(t testing.TB) { b.RequireLines(t) },
			wantFail: true,
		},
		"logged": {
			check: func(t testing.TB) { b.RequireLogged(t, line) },
		},
		"logged, part": {
			check: func(t testing.TB) {
				b.RequireLogged(t, "level=INFO msg=started")
			},
			wantFail: true,
		},
		"match": {
			check: func(t testing.TB) {
				b.RequireMatch(t, `^level=INFO msg=started addr=[0-9.:]+$`)
			},
		},
		"match, none": {
			check: func(t testing.TB) {
				b.RequireMatch(t, `^level=ERROR`)
			},
			wantFail: true,
		},
		"await": {
			check: func(t testing.TB) {
				b.AwaitMatch(t, `msg=started`, time.Second)
			},
		},
		"await, timeout": {
			check: func(t testing.TB) {
				b.AwaitMatch(t, `msg=stopped`, 10*time.Millisecond)
			},
			wantFail: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &failT{TB: t}
			tt.check(f)
			require.Equal(t, tt.wantFail, f.failed, f.msg)
		})
	}
}

// TestRequireBelow tests the level every record has to stay below, a level
// between the named ones included.
func TestRequireBelow(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		logged   slog.Level
		below    slog.Level
		wantFail bool
	}{
		"warn below warn": {
			logged:   slog.LevelWarn,
			below:    slog.LevelWarn,
			wantFail: true,
		},
		"error below warn": {
			logged:   slog.LevelError,
			below:    slog.LevelWarn,
			wantFail: true,
		},
		"error+4 below error": {
			logged:   slog.LevelError + 4,
			below:    slog.LevelError,
			wantFail: true,
		},
		"info below warn": {
			logged: slog.LevelInfo,
			below:  slog.LevelWarn,
		},
		"warn+2 below error": {
			logged: slog.LevelWarn + 2,
			below:  slog.LevelError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var b testlog.Buffer
			b.Logger().Log(t.Context(), tt.logged, "done")
			f := &failT{TB: t}
			b.RequireBelow(f, tt.below)
			require.Equal(t, tt.wantFail, f.failed, f.msg)
		})
	}
}

// TestAwaitMatchWaits tests a record logged after AwaitMatch began to wait.
func TestAwaitMatchWaits(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var b testlog.Buffer
		go func() {
			time.Sleep(time.Second)
			b.Logger().Info("late")
		}()
		b.AwaitMatch(t, `^level=INFO msg=late$`, 2*time.Second)
	})
}
