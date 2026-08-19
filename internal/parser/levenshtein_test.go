package parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFuzzyMatchParamName(t *testing.T) {
	for name, tc := range map[string]struct {
		paramName string
		wantName  string
		wantOK    bool
	}{
		"exact sessionToken": {
			paramName: "sessionToken", wantName: "sessionToken", wantOK: true,
		},
		"sessionTok": {
			paramName: "sessionTok", wantName: "sessionToken", wantOK: true,
		},
		"sesionToken": {
			paramName: "sesionToken", wantName: "sessionToken", wantOK: true,
		},
		// Dispatchers, path, query and signals are matched by their datapages.Dispatcher,
		// datapages.Path, datapages.Query and datapages.Signals types, never by name.
		"dispatc": {paramName: "dispatc", wantOK: false},
		"signls":  {paramName: "signls", wantOK: false},
		"qury":    {paramName: "qury", wantOK: false},
		"sess":    {paramName: "sess", wantOK: false}, // 3 edits too far
		"sessio":  {paramName: "sessio", wantName: "session", wantOK: true},
		"xyz":     {paramName: "xyz", wantOK: false},
		"abc":     {paramName: "abc", wantOK: false},
		"x":       {paramName: "x", wantOK: false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := fuzzyMatchParamName(tc.paramName, nil)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantName, got)
			}
		})
	}
}
