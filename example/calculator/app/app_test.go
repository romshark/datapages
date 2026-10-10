package app_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/app/calc"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen/action"
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
			body, logs := query(t, tt.btn, tt.paste, tt.signals)
			require.Empty(t, logs)
			require.Contains(t, body, `data-signals:input="&#34;`+tt.input+`&#34;"`)
			require.Contains(t, body, `<div class="display">`+tt.display+`</div>`)
		})
	}
}

// TestQUERYInputOverLimit tests that the server refuses an input signal longer
// than calc.MaxInputLen before evaluating it. calc.Evaluate recurses once per
// parenthesis, and the default body limit of 1 MiB holds a million of them.
func TestQUERYInputOverLimit(t *testing.T) {
	body, logs := query(t, calc.CalcButtonEq, false,
		signals{Input: strings.Repeat("(", calc.MaxInputLen+1)})
	require.Empty(t, body)
	require.Contains(t, logs, "invalid input signal")
}

// query sends the request the page sends for a press of btn or for a paste
// and returns the response body and the server log.
func query(
	t *testing.T, btn calc.CalcButton, paste bool, s signals,
) (body, logs string) {
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

	b, err := json.Marshal(s)
	require.NoError(t, err)
	url := urlOf(t, action.PageIndex.Input.QUERY(
		action.PageIndex.Input.QUERYQuery(int(btn), paste)))
	req := httptest.NewRequest("QUERY", url, bytes.NewReader(b))
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Body.String(), logBuf.String()
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
