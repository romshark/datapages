package calc

import "unicode/utf8"

// CalcButton identifies a calculator button.
type CalcButton int

const (
	CalcButtonClear CalcButton = iota
	CalcButtonBackspace
	CalcButtonParen
	CalcButtonDiv
	CalcButton7
	CalcButton8
	CalcButton9
	CalcButtonMul
	CalcButton4
	CalcButton5
	CalcButton6
	CalcButtonSub
	CalcButton1
	CalcButton2
	CalcButton3
	CalcButtonAdd
	CalcButton0
	CalcButtonDot
	CalcButtonEq
	calcButtonCount // must remain last
)

// ValidButton reports whether btn is a known calculator button.
func ValidButton(btn CalcButton) bool {
	return btn >= 0 && btn < calcButtonCount
}

// MaxInputLen is the length in runes of the longest input [ValidInput] accepts.
// [Evaluate] recurses once per parenthesis and unary minus, and its arithmetic
// takes time quadratic in the input length: each division by 0.1 in a chain
// adds a digit to the quotient.
const MaxInputLen = 100

// ValidInput reports whether input is "Error" or at most [MaxInputLen] runes
// of digits, decimal points, parentheses and the four operator runes.
// [Press] and [Paste] return an input that passes when given one that passes.
//
// The input signal round-trips through the client, which can send anything.
func ValidInput(input string) bool {
	if input == errorText {
		return true
	}
	if utf8.RuneCountInString(input) > MaxInputLen {
		return false
	}
	for _, r := range input {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'.', '(', ')', '+', '-', '\u00d7', '\u00f7':
		default:
			return false
		}
	}
	return true
}

// Press applies a button press to the current calculator state
// and returns the new input string and fresh flag. An "Error" input counts as empty.
// A press that would make the input longer than [MaxInputLen] leaves the state unchanged,
// and = turns a longer result into "Error".
func Press(input string, fresh bool, btn CalcButton) (string, bool) {
	if input == errorText {
		input = ""
	}
	next, nextFresh := press(input, fresh, btn)
	if ValidInput(next) {
		return next, nextFresh
	}
	// Only the length can fail: press appends calculator runes,
	// and Evaluate returns a decimal number or "Error".
	if btn == CalcButtonEq {
		return errorText, true
	}
	return input, fresh
}

// Paste appends the pasted number num to input and returns the new input
// and fresh flag. num replaces input when fresh is set or input is "Error".
// A paste that would make the input fail [ValidInput] leaves the state unchanged.
func Paste(input string, fresh bool, num string) (string, bool) {
	next := num
	if !fresh && input != errorText {
		next = input + num
	}
	if !ValidInput(next) {
		return input, fresh
	}
	return next, false
}

// press is [Press] for an input other than "Error", without the length limit.
func press(input string, fresh bool, btn CalcButton) (string, bool) {
	switch btn {
	case CalcButtonClear:
		return "", false

	case CalcButtonBackspace:
		if fresh {
			return "", false
		}
		runes := []rune(input)
		if len(runes) > 0 {
			return string(runes[:len(runes)-1]), false
		}
		return "", false

	case CalcButtonEq:
		return Evaluate(input), true

	case CalcButtonParen:
		if fresh {
			input = ""
		}
		return input + smartParen(input), false

	case CalcButtonDiv, CalcButtonMul, CalcButtonSub, CalcButtonAdd:
		// Operators don't clear on fresh, allowing result chaining.
		ch := operatorRune(btn)
		runes := []rune(input)
		if len(runes) > 0 && isOperator(runes[len(runes)-1]) {
			return string(runes[:len(runes)-1]) + string(ch), false
		}
		return input + string(ch), false

	default:
		if fresh {
			input = ""
		}
		return input + string(digitRune(btn)), false
	}
}

// smartParen returns "(" or ")" based on the current expression,
// matching Android calculator behavior: close when there are
// unmatched open parens and the last character is a digit, dot, or ")".
func smartParen(input string) string {
	open := 0
	for _, r := range input {
		switch r {
		case '(':
			open++
		case ')':
			open--
		}
	}
	runes := []rune(input)
	if open > 0 && len(runes) > 0 {
		last := runes[len(runes)-1]
		if last >= '0' && last <= '9' || last == '.' || last == ')' {
			return ")"
		}
	}
	return "("
}

func operatorRune(btn CalcButton) rune {
	switch btn {
	case CalcButtonDiv:
		return '\u00f7' // ÷
	case CalcButtonMul:
		return '\u00d7' // ×
	case CalcButtonSub:
		return '-'
	case CalcButtonAdd:
		return '+'
	default:
		return 0
	}
}

func digitRune(btn CalcButton) rune {
	switch btn {
	case CalcButton0:
		return '0'
	case CalcButton1:
		return '1'
	case CalcButton2:
		return '2'
	case CalcButton3:
		return '3'
	case CalcButton4:
		return '4'
	case CalcButton5:
		return '5'
	case CalcButton6:
		return '6'
	case CalcButton7:
		return '7'
	case CalcButton8:
		return '8'
	case CalcButton9:
		return '9'
	case CalcButtonDot:
		return '.'
	default:
		return 0
	}
}

func isOperator(r rune) bool {
	return r == '\u00f7' || r == '\u00d7' || r == '-' || r == '+'
}
