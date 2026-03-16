package calc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Evaluate parses and evaluates a mathematical expression string.
// It supports +, -, *, / (and Unicode × ÷), parentheses, and unary minus.
// Returns the result as a string, or "Error" for invalid expressions.
func Evaluate(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "0"
	}
	expr = strings.ReplaceAll(expr, "\u00d7", "*")
	expr = strings.ReplaceAll(expr, "\u00f7", "/")
	p := &parser{input: expr}
	result, err := p.parseExpr()
	if err != nil || p.pos < len(p.input) {
		return "Error"
	}
	if math.IsInf(result, 0) || math.IsNaN(result) {
		return "Error"
	}
	return strconv.FormatFloat(result, 'f', -1, 64)
}

type parser struct {
	input string
	pos   int
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.input) && p.input[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		op := p.peek()
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			left *= right
		} else {
			left /= right
		}
	}
	return left, nil
}

func (p *parser) parseFactor() (float64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (float64, error) {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.input) &&
		(unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected number at position %d", p.pos)
	}
	return strconv.ParseFloat(p.input[start:p.pos], 64)
}
