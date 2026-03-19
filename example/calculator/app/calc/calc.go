package calc

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

// precision is the number of bits of mantissa used for calculations.
const precision = 256

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
	return result.Text('f', -1)
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

func (p *parser) parseExpr() (*big.Float, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		if op == '+' {
			left.Add(left, right)
		} else {
			left.Sub(left, right)
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (*big.Float, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek()
		if op == '*' || op == '/' {
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			if op == '*' {
				left.Mul(left, right)
			} else {
				if right.Sign() == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				left.Quo(left, right)
			}
		} else if op == '(' || unicode.IsDigit(rune(op)) {
			// Implicit multiplication: 6(2) or (2)(3).
			right, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			left.Mul(left, right)
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseFactor() (*big.Float, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return val.Neg(val), nil
	}
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ')' {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (*big.Float, error) {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.input) &&
		(unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return nil, fmt.Errorf("expected number at position %d", p.pos)
	}
	f, _, err := new(big.Float).SetPrec(precision).Parse(p.input[start:p.pos], 10)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %w", err)
	}
	return f, nil
}
