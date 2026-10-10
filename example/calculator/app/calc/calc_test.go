package calc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEvaluate tests the expression grammar the calculator accepts: the four operators
// including their Unicode spellings, precedence, parentheses and unary minus.
// Anything it cannot parse, and division by zero, yield "Error" rather than a Go error,
// since the result goes straight into the display.
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

// TestFormatDisplay tests the thousands separators the display adds.
// They go into the integer part only, on both sides of an operator,
// and never into the fractional part.
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

// TestValidInput tests the input a server accepts back from the client.
// The input signal round-trips through the browser and lands in a
// data-signals attribute, where an apostrophe would end the JavaScript string.
// The length limit counts runes and bounds the work of Evaluate.
func TestValidInput(t *testing.T) {
	for name, tt := range map[string]struct {
		input string
		want  bool
	}{
		"empty":          {input: "", want: true},
		"digits":         {input: "1234567890", want: true},
		"expression":     {input: "(1.5+2)×3÷4-5", want: true},
		"error":          {input: "Error", want: true},
		"error_appended": {input: "Error7", want: false},
		"limit":          {input: strings.Repeat("÷", MaxInputLen), want: true},
		"over_limit":     {input: strings.Repeat("(", MaxInputLen+1), want: false},
		"apostrophe":     {input: "1');alert(1);('", want: false},
		"letter":         {input: "1e5", want: false},
		"space":          {input: "1 + 2", want: false},
		"asterisk":       {input: "1*2", want: false},
		"less_than":      {input: "<script>", want: false},
		"nul":            {input: "1\x002", want: false},
		"combining":      {input: "1́", want: false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, ValidInput(tt.input))
		})
	}
}

// TestPress tests the state a button press returns. The browser sends it back
// with the next press, and the server refuses every press after a state that
// fails ValidInput, C included.
func TestPress(t *testing.T) {
	full := strings.Repeat("1", MaxInputLen)
	for name, tt := range map[string]struct {
		input     string
		fresh     bool
		btn       CalcButton
		wantInput string
		wantFresh bool
	}{
		"digit":                 {input: "12", btn: CalcButton3, wantInput: "123"},
		"digit_after_result":    {input: "5", fresh: true, btn: CalcButton9, wantInput: "9"},
		"operator_after_result": {input: "5", fresh: true, btn: CalcButtonAdd, wantInput: "5+"},
		"equals":                {input: "2+3", btn: CalcButtonEq, wantInput: "5", wantFresh: true},
		"division_by_zero":      {input: "5÷0", btn: CalcButtonEq, wantInput: "Error", wantFresh: true},
		"clear_after_error":     {input: "Error", fresh: true, btn: CalcButtonClear, wantInput: ""},
		"backspace_after_error": {input: "Error", fresh: true, btn: CalcButtonBackspace, wantInput: ""},
		"digit_after_error":     {input: "Error", fresh: true, btn: CalcButton7, wantInput: "7"},
		"paren_after_error":     {input: "Error", fresh: true, btn: CalcButtonParen, wantInput: "("},
		"operator_after_error":  {input: "Error", fresh: true, btn: CalcButtonSub, wantInput: "-"},
		"equals_after_error": {
			input: "Error", fresh: true, btn: CalcButtonEq,
			wantInput: "0", wantFresh: true,
		},
		"digit_at_limit":    {input: full, btn: CalcButton7, wantInput: full},
		"paren_at_limit":    {input: full, btn: CalcButtonParen, wantInput: full},
		"operator_at_limit": {input: full, btn: CalcButtonAdd, wantInput: full},
		"operator_swap_at_limit": {
			input: full[1:] + "+", btn: CalcButtonMul,
			wantInput: full[1:] + "×",
		},
		"operator_after_result_at_limit": {
			input: full, fresh: true, btn: CalcButtonAdd,
			wantInput: full, wantFresh: true,
		},
		"digit_after_result_at_limit": {
			input: full, fresh: true, btn: CalcButton7,
			wantInput: "7",
		},
		// 1÷3 has 16 decimal places, the product of seven of them 112.
		"result_over_limit": {
			input: strings.Repeat("(1÷3)", 7), btn: CalcButtonEq,
			wantInput: "Error", wantFresh: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, ValidInput(tt.input), "the server refuses %q", tt.input)
			input, fresh := Press(tt.input, tt.fresh, tt.btn)
			require.Equal(t, tt.wantInput, input)
			require.Equal(t, tt.wantFresh, fresh)
			require.True(t, ValidInput(input), "the server refuses %q", input)
		})
	}
}

// TestPaste tests the state a pasted number returns, which the browser
// also sends back with the next press.
func TestPaste(t *testing.T) {
	long := strings.Repeat("9", MaxInputLen)
	for name, tt := range map[string]struct {
		input     string
		fresh     bool
		num       string
		wantInput string
		wantFresh bool
	}{
		"append":       {input: "1+", num: "23", wantInput: "1+23"},
		"after_result": {input: "5", fresh: true, num: "23", wantInput: "23"},
		"after_error":  {input: "Error", fresh: true, num: "23", wantInput: "23"},
		"to_limit":     {input: "1+", num: long[2:], wantInput: "1+" + long[2:]},
		"over_limit":   {input: "1+", num: long, wantInput: "1+"},
		"over_limit_after_result": {
			input: "5", fresh: true, num: long + "9",
			wantInput: "5", wantFresh: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, ValidInput(tt.input), "the server refuses %q", tt.input)
			input, fresh := Paste(tt.input, tt.fresh, tt.num)
			require.Equal(t, tt.wantInput, input)
			require.Equal(t, tt.wantFresh, fresh)
			require.True(t, ValidInput(input), "the server refuses %q", input)
		})
	}
}
