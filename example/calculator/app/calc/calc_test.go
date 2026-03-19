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
		"decimal_addition": {expr: "2.3+5.6", want: "7.9"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, Evaluate(tt.expr))
		})
	}
}

func TestFormatDisplay(t *testing.T) {
	for name, tt := range map[string]struct {
		input string
		want  string
	}{
		"empty":       {input: "", want: "0"},
		"zero":        {input: "0", want: "0"},
		"small":       {input: "999", want: "999"},
		"thousands":   {input: "1000", want: "1,000"},
		"millions":    {input: "1234567", want: "1,234,567"},
		"decimal":     {input: "1234.56", want: "1,234.56"},
		"expression":  {input: "1000+2000", want: "1,000+2,000"},
		"negative":    {input: "-1234567", want: "-1,234,567"},
		"unicode_ops": {input: "1000\u00d72000", want: "1,000\u00d72,000"},
		"no_frac_sep": {input: "1.123456", want: "1.123456"},
		"large":       {input: "999999999999", want: "999,999,999,999"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, FormatDisplay(tt.input))
		})
	}
}
