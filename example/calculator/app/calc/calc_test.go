package calc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluate(t *testing.T) {
	for name, tt := range map[string]struct {
		expr string
		want string
	}{
		"empty":            {expr: "", want: "0"},
		"integer":          {expr: "42", want: "42"},
		"decimal":          {expr: "3.14", want: "3.14"},
		"addition":         {expr: "2+3", want: "5"},
		"subtraction":      {expr: "10-4", want: "6"},
		"multiplication":   {expr: "6*7", want: "42"},
		"division":         {expr: "15/4", want: "3.75"},
		"unicode_multiply": {expr: "6\u00d77", want: "42"},
		"unicode_divide":   {expr: "15\u00f74", want: "3.75"},
		"precedence":       {expr: "2+3*4", want: "14"},
		"parentheses":      {expr: "(2+3)*4", want: "20"},
		"nested_parens":    {expr: "((2+3))*4", want: "20"},
		"unary_minus":      {expr: "-5+3", want: "-2"},
		"spaces":           {expr: " 2 + 3 ", want: "5"},
		"complex":          {expr: "2*(3+4)-1", want: "13"},
		"division_by_zero": {expr: "1/0", want: "Error"},
		"invalid":          {expr: "1++2", want: "Error"},
		"trailing_op":      {expr: "1+", want: "Error"},
		"letters":          {expr: "abc", want: "Error"},
		"missing_paren":    {expr: "(1+2", want: "Error"},
		"trailing_mul":     {expr: "2*", want: "Error"},
		"unary_minus_only": {expr: "-", want: "Error"},
		"empty_parens":     {expr: "()", want: "Error"},
		"large_multiply":   {expr: "999999999999999999999999999999999*2", want: "1999999999999999999999999999999998"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, Evaluate(tt.expr))
		})
	}
}
