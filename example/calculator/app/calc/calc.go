package calc

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
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
	return result.String()
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

func (p *parser) parseExpr() (decimal.Decimal, error) {
	left, err := p.parseTerm()
	if err != nil {
		return decimal.Decimal{}, err
	}
	for {
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return decimal.Decimal{}, err
		}
		if op == '+' {
			left = left.Add(right)
		} else {
			left = left.Sub(right)
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (decimal.Decimal, error) {
	left, err := p.parseFactor()
	if err != nil {
		return decimal.Decimal{}, err
	}
	for {
		op := p.peek()
		if op == '*' || op == '/' {
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return decimal.Decimal{}, err
			}
			if op == '*' {
				left = left.Mul(right)
			} else {
				if right.IsZero() {
					return decimal.Decimal{}, fmt.Errorf("division by zero")
				}
				left = left.Div(right)
			}
		} else if op == '(' || unicode.IsDigit(rune(op)) {
			// Implicit multiplication: 6(2) or (2)(3).
			right, err := p.parseFactor()
			if err != nil {
				return decimal.Decimal{}, err
			}
			left = left.Mul(right)
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseFactor() (decimal.Decimal, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return decimal.Decimal{}, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		if err != nil {
			return decimal.Decimal{}, err
		}
		return val.Neg(), nil
	}
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return decimal.Decimal{}, err
		}
		if p.peek() != ')' {
			return decimal.Decimal{}, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (decimal.Decimal, error) {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.input) &&
		(unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return decimal.Decimal{}, fmt.Errorf("expected number at position %d", p.pos)
	}
	d, err := decimal.NewFromString(p.input[start:p.pos])
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("invalid number: %w", err)
	}
	return d, nil
}
