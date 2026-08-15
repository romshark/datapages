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
		"exact sessionToken": {paramName: "sessionToken", wantName: "sessionToken", wantOK: true},
		"sessionTok":         {paramName: "sessionTok", wantName: "sessionToken", wantOK: true},
		"sesionToken":        {paramName: "sesionToken", wantName: "sessionToken", wantOK: true},
		"signal":             {paramName: "signal", wantName: "signals", wantOK: true},
		"signls":             {paramName: "signls", wantName: "signals", wantOK: true},
		"dispatc":            {paramName: "dispatc", wantName: "dispatch", wantOK: true},
		"sess":               {paramName: "sess", wantOK: false}, // 3 edits too far
		"sessio":             {paramName: "sessio", wantName: "session", wantOK: true},
		"qurey":              {paramName: "qurey", wantOK: false}, // 2 edits in 5-char word
		"qury":               {paramName: "qury", wantName: "query", wantOK: true},
		"querys":             {paramName: "querys", wantName: "query", wantOK: true},
		"xyz":                {paramName: "xyz", wantOK: false},
		"abc":                {paramName: "abc", wantOK: false},
		"x":                  {paramName: "x", wantOK: false},
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
