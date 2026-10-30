package datapagestest

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// errNeedsBrowser reports an expression outside the subset evalExpr evaluates.
var errNeedsBrowser = errors.New("needs a browser")

// evalExpr evaluates the subset of JavaScript that Datastar attributes such as
// data-show and data-text mostly hold. It covers signals, literals, !, unary -,
// + and -, comparisons, && and ||, ?: and parentheses.
// Comparisons are strict, so 1 == "1" is false. Anything else returns errNeedsBrowser.
func (p *page) evalExpr(expr string) (any, error) {
	if p.signals == nil {
		return nil, errNeedsBrowser
	}
	e := &exprParser{p: p, s: expr}
	v, err := e.cond()
	if err != nil {
		return nil, err
	}
	e.space()
	if e.i != len(e.s) {
		return nil, errNeedsBrowser
	}
	return v, nil
}

type exprParser struct {
	p     *page
	s     string
	i     int
	depth int // computed signals being evaluated, to stop a cycle
}

func (e *exprParser) space() {
	for e.i < len(e.s) && strings.IndexByte(" \t\r\n", e.s[e.i]) >= 0 {
		e.i++
	}
}

// eat consumes op if it comes next and is not the start of a longer operator.
func (e *exprParser) eat(op string) bool {
	e.space()
	if !strings.HasPrefix(e.s[e.i:], op) {
		return false
	}
	if next := e.i + len(op); next < len(e.s) {
		c := e.s[next]
		if (op == "=" || op == "!" || op == "<" || op == ">") && c == '=' ||
			(op == "==" || op == "!=") && c == '=' ||
			op == "&" && c == '&' || op == "|" && c == '|' {
			return false
		}
	}
	e.i += len(op)
	return true
}

func (e *exprParser) cond() (any, error) {
	c, err := e.or()
	if err != nil || !e.eat("?") {
		return c, err
	}
	a, err := e.cond()
	if err != nil {
		return nil, err
	}
	if !e.eat(":") {
		return nil, errNeedsBrowser
	}
	b, err := e.cond()
	if err != nil {
		return nil, err
	}
	if truthy(c) {
		return a, nil
	}
	return b, nil
}

func (e *exprParser) or() (any, error) {
	v, err := e.and()
	for err == nil && e.eat("||") {
		var w any
		if w, err = e.and(); !truthy(v) {
			v = w
		}
	}
	return v, err
}

func (e *exprParser) and() (any, error) {
	v, err := e.equality()
	for err == nil && e.eat("&&") {
		var w any
		if w, err = e.equality(); truthy(v) {
			v = w
		}
	}
	return v, err
}

func (e *exprParser) equality() (any, error) {
	v, err := e.relation()
	for err == nil {
		var not bool
		switch {
		case e.eat("==="), e.eat("=="):
		case e.eat("!=="), e.eat("!="):
			not = true
		default:
			return v, nil
		}
		var w any
		if w, err = e.relation(); err == nil {
			v = equal(v, w) != not
		}
	}
	return v, err
}

func equal(a, b any) bool {
	switch a := a.(type) {
	case nil, string, float64, bool:
		return a == b
	}
	return false
}

func (e *exprParser) relation() (any, error) {
	v, err := e.sum()
	for err == nil {
		var op string
		for _, o := range []string{"<=", ">=", "<", ">"} {
			if e.eat(o) {
				op = o
				break
			}
		}
		if op == "" {
			return v, nil
		}
		var w any
		if w, err = e.sum(); err != nil {
			break
		}
		var c int
		switch a := v.(type) {
		case float64:
			b, ok := w.(float64)
			if !ok {
				return nil, errNeedsBrowser
			}
			c = compare(a, b)
		case string:
			b, ok := w.(string)
			if !ok {
				return nil, errNeedsBrowser
			}
			c = strings.Compare(a, b)
		default:
			return nil, errNeedsBrowser
		}
		switch op {
		case "<":
			v = c < 0
		case ">":
			v = c > 0
		case "<=":
			v = c <= 0
		case ">=":
			v = c >= 0
		}
	}
	return v, err
}

func compare(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func (e *exprParser) sum() (any, error) {
	v, err := e.unary()
	for err == nil {
		var minus bool
		switch {
		case e.eat("+"):
		case e.eat("-"):
			minus = true
		default:
			return v, nil
		}
		var w any
		if w, err = e.unary(); err != nil {
			break
		}
		a, aNum := v.(float64)
		b, bNum := w.(float64)
		_, aStr := v.(string)
		_, bStr := w.(string)
		switch {
		case aNum && bNum && minus:
			v = a - b
		case aNum && bNum:
			v = a + b
		case !minus && (aStr || bStr):
			v = jsString(v) + jsString(w)
		default:
			return nil, errNeedsBrowser
		}
	}
	return v, err
}

func (e *exprParser) unary() (any, error) {
	switch {
	case e.eat("!"):
		v, err := e.unary()
		return !truthy(v), err
	case e.eat("-"):
		v, err := e.unary()
		if f, ok := v.(float64); ok && err == nil {
			return -f, nil
		}
		return nil, errNeedsBrowser
	}
	return e.primary()
}

func (e *exprParser) primary() (any, error) {
	e.space()
	if e.i == len(e.s) {
		return nil, errNeedsBrowser
	}
	switch c := e.s[e.i]; {
	case c == '(':
		e.i++
		v, err := e.cond()
		if err != nil {
			return nil, err
		}
		if !e.eat(")") {
			return nil, errNeedsBrowser
		}
		return v, nil
	case c == '$':
		start := e.i + 1
		e.i = start
		for e.i < len(e.s) &&
			(isIdentByte(e.s[e.i]) && e.s[e.i] != '$' || e.s[e.i] == '.') {
			e.i++
		}
		path := e.s[start:e.i]
		if !signalPath.MatchString(path) || e.i < len(e.s) && e.s[e.i] == '(' {
			return nil, errNeedsBrowser
		}
		return e.signal(path)
	case c == '\'' || c == '"':
		end, err := skipString(e.s, e.i)
		if err != nil {
			return nil, errNeedsBrowser
		}
		s, err := unquoteJS(e.s[e.i+1 : end])
		e.i = end + 1
		return s, err
	case '0' <= c && c <= '9' || c == '.':
		start := e.i
		for e.i < len(e.s) && (e.s[e.i] >= '0' && e.s[e.i] <= '9' || e.s[e.i] == '.') {
			e.i++
		}
		f, err := strconv.ParseFloat(e.s[start:e.i], 64)
		if err != nil {
			return nil, errNeedsBrowser
		}
		return f, nil
	}
	for word, v := range map[string]any{
		"true": true, "false": false, "null": nil, "undefined": nil,
	} {
		if strings.HasPrefix(e.s[e.i:], word) &&
			(e.i+len(word) == len(e.s) || !isIdentByte(e.s[e.i+len(word)])) {
			e.i += len(word)
			return v, nil
		}
	}
	return nil, errNeedsBrowser
}

// signal returns a signal, or the value of the data-computed attribute that defines it.
func (e *exprParser) signal(path string) (any, error) {
	if v, ok := getSignal(e.p.signals, path); ok {
		return v, nil
	}
	// A property of a string or a list, such as length, needs a browser.
	keys := strings.Split(path, ".")
	for i := 1; i < len(keys); i++ {
		v, ok := getSignal(e.p.signals, strings.Join(keys[:i], "."))
		if _, isMap := v.(map[string]any); ok && !isMap {
			return nil, errNeedsBrowser
		}
	}
	expr, ok := e.p.computed[path]
	if !ok {
		return nil, nil
	}
	if e.depth > 16 {
		return nil, errNeedsBrowser
	}
	sub := &exprParser{p: e.p, s: expr, depth: e.depth + 1}
	v, err := sub.cond()
	sub.space()
	if err == nil && sub.i != len(sub.s) {
		err = errNeedsBrowser
	}
	return v, err
}

// jsString converts v as JavaScript's String does.
func jsString(v any) string {
	if v == nil {
		return "null"
	}
	return signalString(v)
}

// render applies the attributes through which Datastar shows signals in the DOM:
// data-text, data-attr and data-class. An attribute whose expression
// needs a browser leaves the element as the server rendered it.
// It also records the data-computed signals for evalExpr.
func (p *page) render() {
	p.computed = map[string]string{}
	for n := range p.doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		for _, a := range n.Attr {
			if rest, ok := strings.CutPrefix(a.Key, "data-computed:"); ok {
				name, mods := splitModifiers(rest)
				p.computed[convertCase(name, mods["case"])] = a.Val
			}
		}
	}
	if p.signals == nil {
		return
	}
	for n := range p.doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		var text *string
		attrs := map[string]any{}
		classes := map[string]bool{}
		for _, a := range n.Attr {
			switch {
			case a.Key == "data-text":
				if v, err := p.evalExpr(a.Val); err == nil {
					s := jsString(v)
					if v == nil {
						s = ""
					}
					text = &s
				}
			case strings.HasPrefix(a.Key, "data-attr:"):
				name, _ := splitModifiers(strings.TrimPrefix(a.Key, "data-attr:"))
				if v, err := p.evalExpr(a.Val); err == nil {
					attrs[name] = v
				}
			case strings.HasPrefix(a.Key, "data-class:"):
				name, _ := splitModifiers(strings.TrimPrefix(a.Key, "data-class:"))
				if v, err := p.evalExpr(a.Val); err == nil {
					classes[name] = truthy(v)
				}
			}
		}
		if text != nil {
			for c := n.FirstChild; c != nil; c = n.FirstChild {
				n.RemoveChild(c)
			}
			n.AppendChild(&html.Node{Type: html.TextNode, Data: *text})
		}
		for name, v := range attrs {
			if v == nil || v == false {
				removeAttr(n, name)
			} else {
				setAttr(n, name, jsString(v))
			}
		}
		if len(classes) > 0 {
			list := strings.Fields(attr(n, "class"))
			for name, on := range classes {
				i := slices.Index(list, name)
				switch {
				case on && i < 0:
					list = append(list, name)
				case !on && i >= 0:
					list = append(list[:i], list[i+1:]...)
				}
			}
			setAttr(n, "class", strings.Join(list, " "))
		}
	}
}
