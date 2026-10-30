package app_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/app/calc"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen/action"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestPress tests what the display shows after a sequence of presses and
// pastes. The page sends its state with each press.
func TestPress(t *testing.T) {
	for name, tt := range map[string]struct {
		keys        []string // the labels of the buttons pressed, or "paste:" and a number
		wantInput   string
		wantDisplay string
	}{
		"division_by_zero": {
			keys:      []string{"5", "÷", "0", "="},
			wantInput: "Error", wantDisplay: "Error",
		},
		"clear_after_error": {
			keys:      []string{"5", "÷", "0", "=", "C"},
			wantInput: "", wantDisplay: "0",
		},
		"digit_after_error": {
			keys:      []string{"5", "÷", "0", "=", "7"},
			wantInput: "7", wantDisplay: "7",
		},
		"paste_after_error": {
			keys:      []string{"5", "÷", "0", "=", "paste:42"},
			wantInput: "42", wantDisplay: "42",
		},
		"paste_over_limit": {
			keys:      []string{"1", "paste:" + strings.Repeat("9", calc.MaxInputLen)},
			wantInput: "1", wantDisplay: "1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tab, logs := load(t)
			for _, key := range tt.keys {
				num, paste := strings.CutPrefix(key, "paste:")
				if !paste {
					tab.Click(t, datapagestest.Role("button", key))
					continue
				}
				// The paste handler reads the clipboard into num before the request.
				tab.SetSignal(t, "num", num)
				tab.Call(t, action.PageIndex.Input.QUERY(
					action.PageIndex.Input.QUERYQuery(0, true)))
			}
			require.Empty(t, logs.String())
			require.Equal(t, tt.wantInput, tab.Signal(t, "input"))
			require.Equal(t, tt.wantDisplay,
				tab.Get(t, datapagestest.Role("status", "")).Text())
		})
	}
}

// TestPage tests what the calculator shows on load.
func TestPage(t *testing.T) {
	tab, _ := load(t)
	want := datapagestest.Tree{
		{Role: "heading", Name: "Calculator", Level: 1},
		{Role: "status", Text: "0"},
	}
	for _, label := range []string{
		"C", "⇐", "()", "÷", "7", "8", "9", "×", "4", "5", "6", "−",
		"1", "2", "3", "+", "0", ".", "=",
	} {
		want = append(want, datapagestest.Node{Role: "button", Name: label})
	}
	want = append(want,
		datapagestest.Node{Role: "paragraph", Text: "Expression is evaluated server-side"},
		datapagestest.Node{Role: "text", Text: "Built with"},
		datapagestest.Node{
			Role: "link", Name: "Datapages", URL: "https://github.com/romshark/datapages",
		},
	)
	tab.RequireTree(t, datapagestest.ID("page-calculator"), want)
}

// TestInputOverLimit tests that the server refuses an input signal longer
// than calc.MaxInputLen before evaluating it. calc.Evaluate recurses once per
// parenthesis, and the default body limit of 1 MiB holds 1 million of them.
// The handler has answered with an event stream by then, so the refusal is an
// empty stream.
func TestInputOverLimit(t *testing.T) {
	tab, logs := load(t)
	tab.SetSignal(t, "input", strings.Repeat("(", calc.MaxInputLen+1))
	resp := tab.Click(t, datapagestest.Role("button", "="))
	require.Empty(t, resp.Body)
	require.Contains(t, logs.String(), "invalid input signal")
}

// load loads the calculator in a tab and returns it with the server log.
func load(t *testing.T) (*datapagestest.Tab, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	srv, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.NewApp(), inmem.New(0),
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	require.NoError(t, err)
	return datapagestest.New(t, srv).Visitor(t).Load(t, href.PageIndex()), &logs
}
