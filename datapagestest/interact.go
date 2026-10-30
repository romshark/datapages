package datapagestest

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Click clicks the element q selects and returns the response of the last
// action it sent, nil when it sent none. The tab applies the responses.
//
// It runs the element's data-on:click or, when the element has none, that of
// its closest ancestor with one. A checkbox or radio toggles first and then
// runs its data-on:input and data-on:change. A submit button submits its form
// as [Tab.Submit] does, unless its data-on:click has __prevent.
//
// An expression may hold generated action calls and assignments of literals
// and signals, such as $_open = false. It fails on any other JavaScript,
// on a disabled element and on an action response with a status of 400 or more.
// Send a failing request with [Tab.Do].
func (tb *Tab) Click(t testing.TB, q Query) *Response {
	t.Helper()
	n := tb.reachable(t, q)
	var resps []*Response
	acted := false
	checkable := n.Data == "input" &&
		(inputType(n) == "checkbox" || inputType(n) == "radio")
	if checkable {
		tb.toggle(t, n)
		acted = true
	}
	target, hs := n, handlers(n, "click")
	if len(hs) == 0 && !checkable {
		for a := n.Parent; a != nil && a.Type == html.ElementNode; a = a.Parent {
			if hs = handlers(a, "click"); len(hs) > 0 {
				target = a
				break
			}
		}
	}
	prevent := false
	for _, h := range hs {
		resps = append(resps, tb.run(t, h.source(target), h.expr, target)...)
		acted = true
		prevent = prevent || h.mods["prevent"] != nil
	}
	if checkable {
		resps = append(resps, tb.fire(t, n, "input", "change")...)
	}
	if !prevent && isSubmitter(n) {
		if form := closest(n, "form"); form != nil {
			resps = append(resps, tb.submit(t, form)...)
			acted = true
		}
	}
	if !acted {
		if n.Data == "a" && hasAttr(n, "href") {
			t.Fatalf("%s: %s is a link without data-on:click; load it with Tab.Follow",
				tb.desc(), q)
		}
		t.Fatalf("%s: %s has no data-on:click: %s", tb.desc(), q, describeNode(n))
	}
	return tb.applyAll(resps)
}

// Fill types value into the text input or textarea q selects. It sets the
// element's value and data-bind signal, then runs its data-on:input and
// data-on:change without their debounce. A signal holding a number receives
// value as a number when it is one.
func (tb *Tab) Fill(t testing.TB, q Query, value string) {
	t.Helper()
	n := tb.reachable(t, q)
	switch {
	case n.Data == "textarea":
	case n.Data == "input" && fillable(inputType(n)):
	default:
		t.Fatalf("%s: %s is %s; Fill takes a text input or a textarea, "+
			"Select a select and Click a checkbox", tb.desc(), q, describeNode(n))
	}
	if hasAttr(n, "readonly") {
		t.Fatalf("%s: %s is read-only", tb.desc(), q)
	}
	if n.Data == "textarea" {
		for c := n.FirstChild; c != nil; c = n.FirstChild {
			n.RemoveChild(c)
		}
		n.AppendChild(&html.Node{Type: html.TextNode, Data: value})
	} else {
		setAttr(n, "value", value)
	}
	if path, ok := bindPath(n); ok {
		var v any = value
		if old, _ := getSignal(tb.signals(t), path); isNumber(old) {
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				v = f
			}
		}
		setSignal(tb.signals(t), path, v, false)
	}
	tb.applyAll(tb.fire(t, n, "input", "change"))
}

// Select chooses the option labelled option in the select q selects. It sets
// the select's data-bind signal to the option's value, then runs data-on:input
// and data-on:change. In a multiple select it adds the option to the chosen ones.
func (tb *Tab) Select(t testing.TB, q Query, option string) {
	t.Helper()
	n := tb.reachable(t, q)
	if n.Data != "select" {
		t.Fatalf("%s: %s is %s, not a select", tb.desc(), q, describeNode(n))
	}
	option = collapse(option)
	var chosen *html.Node
	var labels []string
	for _, o := range options(n) {
		labels = append(labels, strconv.Quote(Text(o)))
		if Text(o) == option && !disabled(o) {
			chosen = o
		}
	}
	if chosen == nil {
		t.Fatalf("%s: %s has no enabled option %q; options: %s",
			tb.desc(), q, option, strings.Join(labels, ", "))
	}
	_, multiple := Attr(n, "multiple")
	var values []any
	for _, o := range options(n) {
		if o == chosen {
			setAttr(o, "selected", "")
		} else if !multiple {
			removeAttr(o, "selected")
		}
		if hasAttr(o, "selected") {
			values = append(values, optionValue(o))
		}
	}
	if path, ok := bindPath(n); ok {
		var v any = optionValue(chosen)
		if multiple {
			v = values
		}
		setSignal(tb.signals(t), path, v, false)
	}
	tb.applyAll(tb.fire(t, n, "input", "change"))
}

// Submit submits the form q selects as the browser does. It requires its
// required fields to hold a value and runs its data-on:submit. It returns
// the response of the last action sent. A form without data-on:submit fails,
// since the browser would submit it without the CSRF token.
func (tb *Tab) Submit(t testing.TB, q Query) *Response {
	t.Helper()
	n := tb.Get(t, q).n
	if n.Data != "form" {
		t.Fatalf("%s: %s is %s, not a form", tb.desc(), q, describeNode(n))
	}
	return tb.applyAll(tb.submit(t, n))
}

// Follow loads the href of the link q selects as a new tab of the visitor.
func (tb *Tab) Follow(t testing.TB, q Query) *Tab {
	t.Helper()
	n := tb.Get(t, q).n
	href, ok := Attr(n, "href")
	if !ok {
		t.Fatalf("%s: %s has no href: %s", tb.desc(), q, describeNode(n))
	}
	return tb.v.Load(t, tb.resolve(t, href))
}

// Call runs expr as a handler of the page would, such as an expression of the
// generated action package for a request that a script sends, and returns the
// response of the last action it sent. It runs what [Tab.Click] runs.
func (tb *Tab) Call(t testing.TB, expr string) *Response {
	t.Helper()
	tb.current(t)
	return tb.applyAll(tb.run(t, "the expression", expr, nil))
}

// reachable returns the element q selects and fails when a visitor cannot reach it,
// because it is hidden or disabled.
func (tb *Tab) reachable(t testing.TB, q Query) *html.Node {
	t.Helper()
	el := tb.Get(t, q)
	switch v, expr := el.p.visibility(el.n); v {
	case hidden:
		t.Fatalf("%s: %s is hidden", tb.desc(), q)
	case unknown:
		t.Fatalf("%s: %v", tb.desc(), &unknownShown{q: q, n: el.n, expr: expr})
	}
	if disabled(el.n) {
		t.Fatalf("%s: %s is disabled", tb.desc(), q)
	}
	return el.n
}

// resolve resolves href against the tab's URL and fails when it leaves the server.
func (tb *Tab) resolve(t testing.TB, href string) string {
	t.Helper()
	base, err := url.Parse(tb.url)
	if err != nil {
		t.Fatalf("%s: parsing its URL: %v", tb.desc(), err)
	}
	ref, err := url.Parse(href)
	if err != nil {
		t.Fatalf("%s: parsing %q: %v", tb.desc(), href, err)
	}
	if ref.Scheme != "" || ref.Host != "" {
		t.Fatalf("%s: %q leads off the server", tb.desc(), href)
	}
	u := base.ResolveReference(ref)
	u.Fragment = ""
	return u.RequestURI()
}

func (tb *Tab) signals(t testing.TB) map[string]any {
	t.Helper()
	p := tb.current(t)
	if p.signals == nil {
		t.Fatalf("%s: the page's signals: %v", tb.desc(), p.sigErr)
	}
	return p.signals
}

// applyAll applies resps in order and returns the last, nil when there is none.
func (tb *Tab) applyAll(resps []*Response) *Response {
	var last *Response
	for _, r := range resps {
		tb.applyResponse(r)
		last = r
	}
	return last
}

// toggle checks a radio or flips a checkbox,
// through its data-bind signal when it has one.
func (tb *Tab) toggle(t testing.TB, n *html.Node) {
	t.Helper()
	p := tb.current(t)
	radio := inputType(n) == "radio"
	on := radio || !p.checked(n)
	if path, ok := bindPath(n); ok {
		signals := tb.signals(t)
		var v any = on
		switch old, _ := getSignal(signals, path); {
		case radio:
			v = p.value(n)
		case isList(old):
			var list []any
			for _, e := range old.([]any) {
				if signalString(e) != p.value(n) {
					list = append(list, e)
				}
			}
			if on {
				list = append(list, p.value(n))
			}
			v = list
		}
		setSignal(signals, path, v, false)
		return
	}
	if !on {
		removeAttr(n, "checked")
		return
	}
	if radio {
		root := closest(n, "form")
		if root == nil {
			root = p.doc
		}
		for o := range root.Descendants() {
			if o.Type == html.ElementNode && o.Data == "input" &&
				inputType(o) == "radio" && attr(o, "name") == attr(n, "name") {
				removeAttr(o, "checked")
			}
		}
	}
	setAttr(n, "checked", "")
}

// fire runs the handlers of n for events in order.
func (tb *Tab) fire(t testing.TB, n *html.Node, events ...string) []*Response {
	t.Helper()
	var resps []*Response
	for _, ev := range events {
		for _, h := range handlers(n, ev) {
			resps = append(resps, tb.run(t, h.source(n), h.expr, n)...)
		}
	}
	return resps
}

// submit validates form and runs its data-on:submit.
func (tb *Tab) submit(t testing.TB, form *html.Node) []*Response {
	t.Helper()
	p := tb.current(t)
	if !hasAttr(form, "novalidate") {
		if field := p.missingRequired(form); field != nil {
			t.Fatalf(
				"%s: the browser does not submit the form: %s is required and empty",
				tb.desc(), describeNode(field),
			)
		}
	}
	hs := handlers(form, "submit")
	if len(hs) == 0 {
		t.Fatalf("%s: %s has no data-on:submit, so the browser would submit it "+
			"without the CSRF token", tb.desc(), describeNode(form))
	}
	var resps []*Response
	for _, h := range hs {
		resps = append(resps, tb.run(t, h.source(form), h.expr, form)...)
	}
	return resps
}

// missingRequired returns the first required control of form without a value.
func (p *page) missingRequired(form *html.Node) *html.Node {
	for n := range form.Descendants() {
		if n.Type != html.ElementNode || !hasAttr(n, "required") || disabled(n) {
			continue
		}
		switch n.Data {
		case "input":
			switch inputType(n) {
			case "checkbox", "radio":
				if !p.checked(n) {
					return n
				}
				continue
			}
		case "select", "textarea":
		default:
			continue
		}
		if p.value(n) == "" {
			return n
		}
	}
	return nil
}

// run runs the statements of a handler's expression for el, nil for [Tab.Call],
// and returns the responses of its actions without applying them.
// The browser runs the whole expression before any response arrives.
func (tb *Tab) run(t testing.TB, source, expr string, el *html.Node) []*Response {
	t.Helper()
	sts, err := parseExpr(expr)
	if err != nil {
		t.Fatalf("%s: %s needs a browser: %v; send its request with Tab.Do",
			tb.desc(), source, err)
	}
	var resps []*Response
	for _, st := range sts {
		if st.action == nil {
			tb.assign(t, st)
			continue
		}
		resps = append(resps, tb.send(t, st.action, el))
	}
	return resps
}

func (tb *Tab) assign(t testing.TB, st statement) {
	t.Helper()
	signals := tb.signals(t)
	v := st.value
	if st.from != "" {
		v, _ = getSignal(signals, st.from)
		if st.not {
			v = !truthy(v)
		}
	}
	setSignal(signals, st.path, v, false)
}

// send sends an action as Datastar does and fails on a status of 400 or more.
func (tb *Tab) send(t testing.TB, a *action, el *html.Node) *Response {
	t.Helper()
	path := tb.resolve(t, a.url)
	var resp *Response
	if a.form {
		form := tb.formOf(t, a, el)
		vals, err := tb.current(t).formValues(form)
		if err != nil {
			t.Fatalf("%s: %s %s: %v", tb.desc(), a.method, path, err)
		}
		resp = tb.sendForm(t, a.method, path, vals, a.headers)
	} else {
		resp = tb.sendJSON(t, a.method, path, tb.requestSignals(t, a.filter), a.headers)
	}
	if resp.Status >= 400 {
		t.Fatalf("%s: %s: %d; send a request expected to fail with Tab.Do:\n%s",
			tb.desc(), resp.desc, resp.Status, excerpt(resp.Body))
	}
	return resp
}

// formOf returns the form a form action sends, which is the one its selector
// names or else the closest form around el.
func (tb *Tab) formOf(t testing.TB, a *action, el *html.Node) *html.Node {
	t.Helper()
	if a.selector != "" {
		ns, err := tb.current(t).selectAll(a.selector)
		if err != nil || len(ns) == 0 || ns[0].Data != "form" {
			t.Fatalf("%s: no form matches selector %q: %v", tb.desc(), a.selector, err)
		}
		return ns[0]
	}
	if el != nil {
		if form := closest(el, "form"); form != nil {
			return form
		}
	}
	t.Fatalf("%s: %s %s sends a form, and no form is around the element",
		tb.desc(), a.method, a.url)
	return nil
}

// formValues returns the fields of form as the browser's FormData does.
func (p *page) formValues(form *html.Node) (url.Values, error) {
	vals := url.Values{}
	for n := range form.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		name := attr(n, "name")
		if name == "" || disabled(n) {
			continue
		}
		switch n.Data {
		case "input":
			switch inputType(n) {
			case "button", "image", "reset", "submit":
			case "file":
				return nil, fmt.Errorf("file input %q needs Tab.Do", name)
			case "checkbox", "radio":
				if p.checked(n) {
					vals.Add(name, p.value(n))
				}
			default:
				vals.Add(name, p.value(n))
			}
		case "textarea":
			vals.Add(name, p.value(n))
		case "select":
			if _, multiple := Attr(n, "multiple"); !multiple {
				if o := p.selectedOption(n); o != nil {
					vals.Add(name, optionValue(o))
				}
				continue
			}
			for _, o := range options(n) {
				if hasAttr(o, "selected") {
					vals.Add(name, optionValue(o))
				}
			}
		}
	}
	return vals, nil
}

// handler is a data-on attribute.
type handler struct {
	key  string
	expr string
	mods map[string][]string
}

func (h handler) source(n *html.Node) string {
	return fmt.Sprintf("%s=%q of %s", h.key, h.expr, describeNode(n))
}

// handlers returns the data-on handlers of n for event.
// It leaves out those listening on the window or outside n.
func handlers(n *html.Node, event string) []handler {
	var out []handler
	for _, a := range n.Attr {
		rest, ok := strings.CutPrefix(a.Key, "data-on:")
		if !ok {
			continue
		}
		name, mods := splitModifiers(rest)
		if name != event || mods["window"] != nil || mods["outside"] != nil {
			continue
		}
		out = append(out, handler{key: a.Key, expr: a.Val, mods: mods})
	}
	return out
}

func isSubmitter(n *html.Node) bool {
	switch n.Data {
	case "button":
		t, ok := Attr(n, "type")
		return !ok || strings.EqualFold(t, "submit")
	case "input":
		return inputType(n) == "submit" || inputType(n) == "image"
	}
	return false
}

func fillable(t string) bool {
	switch t {
	case "button", "checkbox", "file", "hidden", "image", "radio", "reset", "submit":
		return false
	}
	return true
}

func isNumber(v any) bool {
	_, ok := v.(float64)
	return ok
}

func closest(n *html.Node, tag string) *html.Node {
	for a := n; a != nil; a = a.Parent {
		if a.Type == html.ElementNode && a.Data == tag {
			return a
		}
	}
	return nil
}

func setAttr(n *html.Node, key, value string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

func removeAttr(n *html.Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}

// signalFilter selects signals by their dotted paths, as Datastar's
// filterSignals option does.
type signalFilter struct {
	include, exclude *regexp.Regexp
}

// defaultExclude leaves out local signals, those with a name starting with _.
// Datastar applies it when an action sets no exclude pattern.
var defaultExclude = regexp.MustCompile(`(^_|\._).*`)

func (f signalFilter) apply(m map[string]any) map[string]any {
	exclude := f.exclude
	if exclude == nil {
		exclude = defaultExclude
	}
	return filterSignals(m, "", f.include, exclude)
}

// filterSignals keeps the leaves whose paths include matches, or all for nil,
// and exclude does not. It leaves out the objects it empties.
func filterSignals(
	m map[string]any, prefix string, include, exclude *regexp.Regexp,
) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			if o := filterSignals(sub, path, include, exclude); len(o) > 0 {
				out[k] = o
			}
			continue
		}
		if (include == nil || include.MatchString(path)) && !exclude.MatchString(path) {
			out[k] = v
		}
	}
	return out
}

// statement is 1 statement of a handler expression, which is an action call or
// an assignment to a signal.
type statement struct {
	action *action

	path  string // the signal assigned
	value any    // the literal assigned
	from  string // the signal copied, "" for a literal
	not   bool   // negates the copied signal
}

// action is a call of a Datastar action such as @post('/add/', {...}).
type action struct {
	method   string
	url      string
	form     bool
	selector string
	filter   signalFilter
	headers  map[string]string
}

var (
	actionCall = regexp.MustCompile(`^@(get|post|put|patch|delete|query)\(`)
	assignment = regexp.MustCompile(`^\$([\w.]+)\s*=\s*(.*)$`)
)

// parseExpr parses a handler expression of action calls and assignments
// separated by semicolons.
func parseExpr(expr string) ([]statement, error) {
	parts, err := splitStatements(expr)
	if err != nil {
		return nil, err
	}
	var out []statement
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		st, err := parseStatement(s)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", s, err)
		}
		out = append(out, st)
	}
	return out, nil
}

var errStatement = errors.New("not a generated action call or a signal assignment")

func parseStatement(s string) (statement, error) {
	if m := actionCall.FindStringSubmatch(s); m != nil {
		a, err := parseAction(strings.ToUpper(m[1]), s[len(m[0]):])
		return statement{action: a}, err
	}
	m := assignment.FindStringSubmatch(s)
	if m == nil || strings.HasPrefix(m[2], "=") || !signalPath.MatchString(m[1]) {
		return statement{}, errStatement
	}
	st := statement{path: m[1]}
	rhs := strings.TrimSpace(m[2])
	if from, ok := strings.CutPrefix(rhs, "!$"); ok && signalPath.MatchString(from) {
		st.from, st.not = from, true
		return st, nil
	}
	if from, ok := strings.CutPrefix(rhs, "$"); ok && signalPath.MatchString(from) {
		st.from = from
		return st, nil
	}
	v, err := literal(rhs)
	if err != nil {
		return statement{}, errStatement
	}
	st.value = v
	return st, nil
}

// parseAction parses the arguments of an action call, which follow its "(".
func parseAction(method, args string) (*action, error) {
	p := &jsParser{s: args}
	p.space()
	u, err := p.str()
	if err != nil {
		return nil, fmt.Errorf("the URL: %w", err)
	}
	a := &action{method: method, url: u}
	p.space()
	if p.peek() == ',' {
		p.i++
		p.space()
		if p.peek() != ')' {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			if v.obj == nil {
				return nil, errors.New("the options are not an object literal")
			}
			if err := a.setOptions(v.obj); err != nil {
				return nil, err
			}
		}
	}
	p.space()
	if p.peek() != ')' {
		return nil, errors.New("unexpected arguments")
	}
	p.i++
	p.space()
	if p.i != len(p.s) {
		return nil, errors.New("unexpected text after the call")
	}
	return a, nil
}

func (a *action) setOptions(opts map[string]jsValue) error {
	for k, v := range opts {
		switch k {
		case "contentType":
			switch v.str {
			case "form":
				a.form = true
			case "json":
			default:
				return fmt.Errorf("unknown contentType %q", v.raw)
			}
		case "selector":
			a.selector = v.str
		case "headers":
			a.headers = map[string]string{}
			for hk, hv := range v.obj {
				a.headers[hk] = hv.str
			}
		case "filterSignals":
			for fk, fv := range v.obj {
				re, err := fv.regexp()
				if err != nil {
					return fmt.Errorf("filterSignals.%s: %w", fk, err)
				}
				switch fk {
				case "include":
					a.filter.include = re
				case "exclude":
					a.filter.exclude = re
				}
			}
		case "payload":
			return errors.New("the payload option needs a browser")
		}
	}
	return nil
}

// jsValue is a value of a JavaScript object literal.
type jsValue struct {
	raw   string // the source
	str   string // the value of a string literal, or the body of a regex
	regex bool
	flags string
	obj   map[string]jsValue
}

// regexp compiles a regex literal. Go's RE2 accepts the patterns Datapages writes.
// Flags other than i, m and s make no difference to a match test.
func (v jsValue) regexp() (*regexp.Regexp, error) {
	if !v.regex {
		return nil, fmt.Errorf("%s is not a regex literal", v.raw)
	}
	flags := strings.Map(func(f rune) rune {
		if f == 'i' || f == 'm' || f == 's' {
			return f
		}
		return -1
	}, v.flags)
	pattern := v.str
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}
	return regexp.Compile(pattern)
}

// jsParser reads the JavaScript literals that generated action calls contain.
type jsParser struct {
	s string
	i int
}

func (p *jsParser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

func (p *jsParser) space() {
	for p.i < len(p.s) && strings.IndexByte(" \t\r\n", p.s[p.i]) >= 0 {
		p.i++
	}
}

// str reads a single- or double-quoted string literal.
func (p *jsParser) str() (string, error) {
	q := p.peek()
	if q != '\'' && q != '"' {
		return "", errors.New("not a string literal")
	}
	end, err := skipString(p.s, p.i)
	if err != nil {
		return "", err
	}
	s, err := unquoteJS(p.s[p.i+1 : end])
	p.i = end + 1
	return s, err
}

func (p *jsParser) value() (jsValue, error) {
	start := p.i
	switch c := p.peek(); c {
	case '\'', '"':
		s, err := p.str()
		return jsValue{raw: p.s[start:p.i], str: s}, err
	case '/':
		end, err := skipRegex(p.s, p.i)
		if err != nil {
			return jsValue{}, err
		}
		body := p.s[p.i+1 : end]
		p.i = end + 1
		for p.i < len(p.s) && isIdentByte(p.s[p.i]) {
			p.i++
		}
		return jsValue{
			raw: p.s[start:p.i], str: body, regex: true, flags: p.s[end+1 : p.i],
		}, nil
	case '{':
		p.i++
		obj := map[string]jsValue{}
		for {
			p.space()
			if p.peek() == '}' {
				p.i++
				return jsValue{raw: p.s[start:p.i], obj: obj}, nil
			}
			var key string
			if q := p.peek(); q == '\'' || q == '"' {
				k, err := p.str()
				if err != nil {
					return jsValue{}, err
				}
				key = k
			} else {
				k := p.i
				for p.i < len(p.s) && isIdentByte(p.s[p.i]) {
					p.i++
				}
				if k == p.i {
					return jsValue{}, errors.New("an object key is missing")
				}
				key = p.s[k:p.i]
			}
			p.space()
			if p.peek() != ':' {
				return jsValue{}, fmt.Errorf("no value for key %q", key)
			}
			p.i++
			p.space()
			v, err := p.value()
			if err != nil {
				return jsValue{}, err
			}
			obj[key] = v
			p.space()
			switch p.peek() {
			case ',':
				p.i++
			case '}':
			default:
				return jsValue{}, errors.New("unterminated object literal")
			}
		}
	}
	// Any other expression, such as a number or $signal,
	// up to the next separator outside brackets.
	depth := 0
	for ; p.i < len(p.s); p.i++ {
		switch p.s[p.i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return jsValue{raw: strings.TrimSpace(p.s[start:p.i])}, nil
			}
			depth--
		case ',':
			if depth == 0 {
				return jsValue{raw: strings.TrimSpace(p.s[start:p.i])}, nil
			}
		case '\'', '"', '`':
			end, err := skipString(p.s, p.i)
			if err != nil {
				return jsValue{}, err
			}
			p.i = end
		}
	}
	return jsValue{raw: strings.TrimSpace(p.s[start:p.i])}, nil
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9'
}

// splitStatements splits expr at the semicolons outside strings,
// regex literals and brackets.
func splitStatements(expr string) ([]string, error) {
	var out []string
	depth, start := 0, 0
	// prev is the last byte that is not a space. A "/" after an operand divides,
	// and after anything else starts a regex literal.
	var prev byte
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch c {
		case '\'', '"', '`':
			end, err := skipString(expr, i)
			if err != nil {
				return nil, err
			}
			i = end
		case '/':
			if prev == 0 || strings.IndexByte("(,=:[!&|?{};", prev) >= 0 {
				end, err := skipRegex(expr, i)
				if err != nil {
					return nil, err
				}
				i = end
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced %q", c)
			}
		case ';':
			if depth == 0 {
				out = append(out, expr[start:i])
				start = i + 1
			}
		}
		if strings.IndexByte(" \t\r\n", c) < 0 {
			prev = expr[i]
		}
	}
	if depth != 0 {
		return nil, errors.New("unbalanced brackets")
	}
	return append(out, expr[start:]), nil
}

// skipString returns the index of the quote that closes the string starting at i.
func skipString(s string, i int) (int, error) {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case q:
			return j, nil
		}
	}
	return 0, errors.New("unterminated string")
}

// skipRegex returns the index of the "/" that closes the regex starting at i.
func skipRegex(s string, i int) (int, error) {
	class := false
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			class = true
		case ']':
			class = false
		case '/':
			if !class {
				return j, nil
			}
		case '\n':
			return 0, errors.New("unterminated regex")
		}
	}
	return 0, errors.New("unterminated regex")
}

// unquoteJS decodes the escapes of a JavaScript string literal's body.
func unquoteJS(s string) (string, error) {
	if !strings.Contains(s, `\`) {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i == len(s) {
			return "", errors.New("unterminated escape")
		}
		switch c := s[i]; c {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '0':
			b.WriteByte(0)
		case 'x', 'u':
			n := 2
			if c == 'u' {
				n = 4
			}
			if c == 'u' && i+1 < len(s) && s[i+1] == '{' {
				end := strings.IndexByte(s[i:], '}')
				if end < 0 {
					return "", errors.New("unterminated escape")
				}
				r, err := strconv.ParseUint(s[i+2:i+end], 16, 32)
				if err != nil {
					return "", err
				}
				b.WriteRune(rune(r))
				i += end
				continue
			}
			if i+n >= len(s) {
				return "", errors.New("short escape")
			}
			r, err := strconv.ParseUint(s[i+1:i+1+n], 16, 32)
			if err != nil {
				return "", err
			}
			b.WriteRune(rune(r))
			i += n
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// sendJSON sends signals, a JSON object, as a Datastar action without applying
// the response. GET and DELETE carry them in the datastar query parameter,
// the other methods in the body.
func (tb *Tab) sendJSON(
	t testing.TB, method, path, signals string, headers map[string]string,
) *Response {
	t.Helper()
	var req *http.Request
	if queryCarries(method) {
		req = tb.NewRequest(t, method, withQuery(t, path, url.Values{"datastar": {signals}}), nil)
	} else {
		req = tb.NewRequest(t, method, path, strings.NewReader(signals))
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return tb.request(t, req)
}

// sendForm sends form fields as a Datastar action without applying the response:
// in the query for GET and DELETE, else in a URL-encoded body.
func (tb *Tab) sendForm(
	t testing.TB, method, path string, form url.Values, headers map[string]string,
) *Response {
	t.Helper()
	var req *http.Request
	if queryCarries(method) {
		req = tb.NewRequest(t, method, withQuery(t, path, form), nil)
	} else {
		req = tb.NewRequest(t, method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return tb.request(t, req)
}
