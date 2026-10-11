package app_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"slices"
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
			resp, logs := query(t, tt.btn, tt.paste, tt.signals)
			require.Empty(t, logs)
			input, display := patchedCalculator(t, resp)
			require.Equal(t, tt.input, input, "the input signal")
			require.Equal(t, tt.display, display, "the display")
		})
	}
}

// TestQUERYInputOverLimit tests that the server refuses an input signal longer
// than calc.MaxInputLen before evaluating it. calc.Evaluate recurses once per
// parenthesis, and the default body limit of 1 MiB holds a million of them.
func TestQUERYInputOverLimit(t *testing.T) {
	resp, logs := query(t, calc.CalcButtonEq, false,
		signals{Input: strings.Repeat("(", calc.MaxInputLen+1)})
	require.Empty(t, resp.Body)
	require.Contains(t, logs, "invalid input signal")
}

// query loads the page and sends the request it sends for a press of btn or
// for a paste. It returns the response and the server log.
func query(
	t *testing.T, btn calc.CalcButton, paste bool, s signals,
) (resp *datapagestest.Response, logs string) {
	t.Helper()
	var logBuf bytes.Buffer
	srv, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.NewApp(), inmem.New(0),
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithLogger(slog.New(slog.NewTextHandler(&logBuf, nil))))
	require.NoError(t, err)

	tab := datapagestest.New(t, srv).Visitor(t).Load(t, href.PageIndex())
	url := urlOf(t, action.PageIndex.Input.QUERY(
		action.PageIndex.Input.QUERYQuery(int(btn), paste)))
	resp = tab.Do(t, "QUERY", url, s)
	return resp, logBuf.String()
}

// urlOf takes the URL out of a Datastar action expression such as
// "@query('/input/?btn=18')", the form the template carries.
func urlOf(t *testing.T, expr string) string {
	t.Helper()
	i := strings.Index(expr, "('")
	j := strings.LastIndex(expr, "')")
	require.False(t, i < 0 || j < i, "not an action expression: %s", expr)
	return expr[i+2 : j]
}

// patchedCalculator returns the input signal and the display text of the calculator
// that resp patches into the page.
func patchedCalculator(
	t *testing.T, resp *datapagestest.Response,
) (input, display string) {
	t.Helper()
	evs := resp.Events(t)
	i := slices.IndexFunc(evs, datapagestest.Element("page-calculator").Matches)
	require.NotEqual(t, -1, i, "no patch of #page-calculator in:\n%s", resp.Body)
	calculator := evs[i].ElementByID(t, "page-calculator")

	expr, _ := datapagestest.Attr(calculator, "data-signals:input")
	require.NoError(t, json.Unmarshal([]byte(expr), &input),
		"data-signals:input=%s is not a JSON string", expr)

	// The display has no id.
	for n := range calculator.Descendants() {
		if class, _ := datapagestest.Attr(n, "class"); class == "display" {
			return input, datapagestest.Text(n)
		}
	}
	require.FailNow(t, "the calculator has no element of class display")
	return "", ""
}
