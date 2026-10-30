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

type signals struct {
	Input string `json:"input"`
	Fresh bool   `json:"fresh"`
	Num   string `json:"num"`
}

// TestQUERYInput tests the state QUERY /input/ patches into the page.
// The browser sends that state back with the next press. After a state that
// fails calc.ValidInput, the server refuses every press until a reload.
func TestQUERYInput(t *testing.T) {
	for name, tt := range map[string]struct {
		btn     calc.CalcButton
		paste   bool
		signals signals
		input   string // input signal in the patch
		display string // display text in the patch
	}{
		"division_by_zero": {
			btn:     calc.CalcButtonEq,
			signals: signals{Input: "5÷0"},
			input:   "Error", display: "Error",
		},
		"clear_after_error": {
			btn:     calc.CalcButtonClear,
			signals: signals{Input: "Error", Fresh: true},
			input:   "", display: "0",
		},
		"digit_after_error": {
			btn:     calc.CalcButton7,
			signals: signals{Input: "Error", Fresh: true},
			input:   "7", display: "7",
		},
		"paste_after_error": {
			paste:   true,
			signals: signals{Input: "Error", Fresh: true, Num: "42"},
			input:   "42", display: "42",
		},
		"paste_over_limit": {
			paste:   true,
			signals: signals{Input: "1", Num: strings.Repeat("9", calc.MaxInputLen)},
			input:   "1", display: "1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tab, logs := load(t)
			resp := query(t, tab, tt.btn, tt.paste, tt.signals)
			require.Empty(t, logs.String())
			calculator := resp.Event(t, datapagestest.Patched("page-calculator")).
				Get(t, datapagestest.ID("page-calculator"))

			require.Equal(t, tt.input, calculator.Signals(t)["input"], "the input signal")
			require.Equal(t, tt.display,
				calculator.Get(t, datapagestest.Role("status", "")).Text(), "the display")
		})
	}
}

// TestQUERYInputOverLimit tests that the server refuses an input signal longer
// than calc.MaxInputLen before evaluating it. calc.Evaluate recurses once per
// parenthesis, and the default body limit of 1 MiB holds 1 million of them.
func TestQUERYInputOverLimit(t *testing.T) {
	tab, logs := load(t)
	resp := query(t, tab, calc.CalcButtonEq, false,
		signals{Input: strings.Repeat("(", calc.MaxInputLen+1)})
	require.Empty(t, resp.Body)
	require.Contains(t, logs.String(), "invalid input signal")
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
	want = append(
		want,
		datapagestest.Node{
			Role: "paragraph", Text: "Expression is evaluated server-side",
		},
		datapagestest.Node{
			Role: "text", Text: "Built with",
		},
		datapagestest.Node{
			Role: "link", Name: "Datapages", URL: "https://github.com/romshark/datapages",
		},
	)
	tab.Page().Get(t, datapagestest.ID("page-calculator")).RequireTree(t, want)
}

// query sends the request the page sends for a press of btn or for a paste.
func query(
	t *testing.T, tab *datapagestest.Tab, btn calc.CalcButton, paste bool, s signals,
) *datapagestest.Response {
	t.Helper()
	return tab.Do(t, action.PageIndex.Input.QUERY(
		action.PageIndex.Input.QUERYQuery(int(btn), paste),
	), s)
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
