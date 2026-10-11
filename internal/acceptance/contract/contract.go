// Package contract holds the assertions that apply to every generated server
// regardless of the application: how the server is constructed and configured,
// what it wraps a page in, whether the URLs it generates address the routes it serves,
// and whether it starts and stops.
//
// The server is generated per model. The same assertion therefore runs against
// a different piece of code in each case, which is why the suite is shared.
//
// A case module wires itself in with a Case value and 1 test:
//
//	func TestContract(t *testing.T) { contract.Run(t, contract.Case{...}) }
//
// The suite cannot name the generated package: it is a different package in every case.
// The Case therefore carries the few generated symbols the assertions need,
// as closures the case module writes.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/romshark/datapages/datapagestest"
)

// Server is the generated *Server, named by what the suite calls on it.
// ListenAndServe, ListenAndServeTLS and Addr are here so that every generated
// server is asserted to expose them. What they do is asserted in
// runtime/httpserve, which holds the code behind them.
type Server interface {
	http.Handler
	ListenAndServe(ctx context.Context, addr string) error
	ListenAndServeTLS(ctx context.Context, addr, certFile, keyFile string) error
	Shutdown(ctx context.Context) error
	Addr() string
}

// Case is what a case tells the shared suite about itself.
type Case struct {
	// NewServer builds this case's server with the given extra options.
	// The suite cannot do this itself. The argument list of NewServer depends on
	// the model, and stateful or metered apps require options of their own.
	//
	// The options are the values the With* fields below return. The case
	// converts them back to the generated option type.
	NewServer func(t *testing.T, opts ...any) Server

	// The fields below hand the suite 1 generated
	// server option each. A nil field skips the assertions that need it.
	WithMiddleware func(...func(http.Handler) http.Handler) any
	WithDatastarJS func(src string) any
	WithHTTPServer func(*http.Server) any
	WithLogger     func(*slog.Logger) any
	WithBuildID    func(id string) any

	// StreamSubjects is the generated MessageBrokerStreamSubjects.
	StreamSubjects func() []string

	// HrefExternal and HrefSetLogger are External and
	// SetLogger of the generated href package.
	HrefExternal  func(string) string
	HrefSetLogger func(*slog.Logger)

	// Index is a URL that renders a page. Defaults to "/".
	Index string

	// Links are URLs built by the generated href package.
	// Every one of them must address a route the server serves.
	Links []string

	// Actions are expressions built by the generated action package,
	// in the form the templates carry them: "@post('/x/')". Every one of them must
	// address a route the server serves with that method.
	Actions []string

	// SignalActions are the subset of Actions whose handlers read signals.
	// Each is sent a body that is not signals.
	// The server must refuse the request instead of crashing.
	SignalActions []string

	// StreamPath is the SSE route of the Index page,
	// "" when that page neither handles events nor hooks a stream.
	StreamPath string

	// DispatchAction is an action that dispatches an event the page behind
	// StreamPath handles, and DispatchBody the signals it reads.
	DispatchAction string
	DispatchBody   string

	// StateAction is an action that needs the calling tab's state,
	// and StateActionBody the signals it reads.
	// Set by cases whose pages hold per-tab state.
	StateAction     string
	StateActionBody string

	// OptionedAction is one of the case's action expressions built with every
	// option the generated package offers, in the order listed by optionKeys.
	// A template can pass any combination to any action. Every generated
	// builder must write 1 well-formed expression out of whatever it is given.
	OptionedAction string
}

// Opt adapts a generated option constructor to what a Case field takes.
// The suite cannot name the generated option type; the case keeps it.
func Opt[A, O any](f func(A) O) func(A) any {
	return func(a A) any { return f(a) }
}

// OptVariadic adapts a variadic option constructor the way [Opt] adapts a
// unary one.
func OptVariadic[A, O any](f func(...A) O) func(...A) any {
	return func(a ...A) any { return f(a...) }
}

// Options converts what the suite passed to NewServer back to the option type
// of the case's generated package.
//
//	datapagesgen.NewServer(app, broker, contract.Options[datapages.ServerOption](opts)...)
func Options[O any](opts []any) []O {
	out := make([]O, len(opts))
	for i, o := range opts {
		out[i] = o.(O)
	}
	return out
}

// optionKeys are the option keys of the expression a case builds for OptionedAction,
// in the order the generated writer emits them.
var optionKeys = []string{
	"contentType", "selector", "headers", "filterSignals", "openWhenHidden",
	"payload", "retry", "retryInterval", "retryScaler", "retryMaxWaitMs",
	"retryMaxCount", "requestCancellation",
}

// Run applies the whole suite to c.
func Run(t *testing.T, c Case) {
	t.Helper()
	// testExternalHref sets the package-level logger of the generated href
	// package. It runs before the parallel tests, which the testing package
	// holds until this function returns, hence alone.
	t.Run("ExternalHref", c.testExternalHref)

	for name, run := range map[string]func(*testing.T){
		"ActionOptions":              c.testActionOptions,
		"AttributeSeparation":        c.testAttributeSeparation,
		"BuildCheck":                 c.testBuildCheck,
		"ClientGoesAway":             c.testClientGoesAway,
		"Compression":                c.testCompression,
		"DatastarJS":                 c.testDatastarJS,
		"DispatchReachesTheStream":   c.testDispatchReachesTheStream,
		"GeneratedActionsAreRouted":  c.testGeneratedActionsAreRouted,
		"GeneratedLinksAreRouted":    c.testGeneratedLinksAreRouted,
		"HTTPServerOption":           c.testHTTPServerOption,
		"LoggerOption":               c.testLoggerOption,
		"MalformedSignals":           c.testMalformedSignals,
		"MessageBrokerStreamSubject": c.testMessageBrokerStreamSubjects,
		"Middleware":                 c.testMiddleware,
		"PageShell":                  c.testPageShell,
		"ShutdownClosesStreams":      c.testShutdownClosesStreams,
		"StateIsReleased":            c.testStateIsReleased,
		"Stream":                     c.testStream,
		"StreamRequiresDatastar":     c.testStreamRequiresDatastar,
		"TrailingSlash":              c.testTrailingSlash,
		"UnknownRoute":               c.testUnknownRoute,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run(t)
		})
	}
}

// testActionOptions tests the expression a template carries when it passes
// options to an action.
//
// The expression is a string with no runtime behind it. Nothing on the server
// can report one the browser cannot parse. The application does not find out either.
func (c Case) testActionOptions(t *testing.T) {
	expr := c.OptionedAction
	if expr == "" {
		t.Skip("the case has no actions")
	}

	if !strings.HasPrefix(expr, "$busy = true; ") {
		t.Errorf("the before expression does not open the expression: %s", expr)
	}
	if !strings.HasSuffix(expr, "; $busy = false") {
		t.Errorf("the after expression does not close the expression: %s", expr)
	}

	call := strings.TrimPrefix(expr, "$busy = true; ")
	call = strings.TrimSuffix(call, "; $busy = false")
	if !strings.HasPrefix(call, "@") || !strings.Contains(call, "('/") {
		t.Fatalf("the call is not an action expression: %s", call)
	}
	if !strings.HasSuffix(call, ")") {
		t.Errorf("the call is not closed: %s", call)
	}

	// The options are 1 JavaScript object literal.
	found := strings.Contains(call, ", {")
	if !found {
		t.Fatalf("the call carries no options object: %s", call)
	}
	if !strings.HasSuffix(call, "})") {
		t.Errorf("the options object is not closed: %s", call)
	}
	for _, key := range optionKeys {
		if !strings.Contains(call, key+": ") {
			t.Errorf("the options object has no %s: %s", key, call)
		}
	}

	// A value carrying a quote is escaped rather than closing the string it sits in,
	// in a selector and in a header alike.
	if !strings.Contains(call, `selector: '#it\'s'`) {
		t.Errorf("the selector is not escaped: %s", call)
	}
	if !strings.Contains(call, `'it\'s here'`) {
		t.Errorf("the header value is not escaped: %s", call)
	}
	if !strings.Contains(call, "'X-Trace': 'abc'") {
		t.Errorf("the headers object lost a header: %s", call)
	}
	if unbalanced(call) {
		t.Errorf("the expression's braces do not balance: %s", call)
	}
}

// unbalanced reports whether the braces of an expression fail to pair up.
// That is the shape of a malformed options object.
func unbalanced(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return true
			}
		}
	}
	return depth != 0
}

func (c Case) index() string {
	if c.Index != "" {
		return c.Index
	}
	return "/"
}

func (c Case) serve(t *testing.T, opts ...any) *datapagestest.Server {
	t.Helper()
	srv, _ := c.serveHandle(t, opts...)
	return srv
}

// serveHandle returns the served server together with the generated server behind it,
// for the assertions that call its own methods.
func (c Case) serveHandle(t *testing.T, opts ...any) (*datapagestest.Server, Server) {
	t.Helper()
	handle := c.NewServer(t, opts...)
	return datapagestest.New(t, handle), handle
}

func (c Case) visit(t *testing.T, opts ...any) *datapagestest.Visitor {
	t.Helper()
	return c.serve(t, opts...).Visitor(t)
}

// testPageShell tests what the server wraps every page body in.
// Nothing else assembles the document.
// Every page that renders at all renders inside this.
func (c Case) testPageShell(t *testing.T) {
	resp := c.visit(t).Get(t, c.index())
	if resp.Status != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", c.index(), resp.Status)
	}
	if got, want := resp.Header.Get("Content-Type"), "text/html"; !strings.HasPrefix(got, want) {
		t.Errorf("Content-Type = %q, want prefix %q", got, want)
	}
	for _, want := range []string{"<!DOCTYPE html>", "<html", "<head", "<body", "</html>"} {
		if !strings.Contains(resp.Body, want) {
			t.Errorf("the page shell does not carry %q:\n%s", want, resp.Body)
		}
	}
	if !strings.Contains(resp.Body, "<script") {
		t.Errorf("the page shell loads no client script:\n%s", resp.Body)
	}

	checkAttributeSeparation(t, c.index(), resp.Body)
}

// checkAttributeSeparation tests the separator between the attributes the
// server writes.
//
// Every attribute writer opens with its own space and the shell writes none,
// which is the only convention that keeps 2 attributes apart. A quote followed
// by an attribute name is the HTML parse error
// missing-whitespace-between-attributes: browsers recover from it, validators
// refuse it.
func checkAttributeSeparation(t *testing.T, url, body string) {
	t.Helper()
	if i := strings.Index(body, `"data-`); i >= 0 {
		t.Errorf("%s writes 2 attributes without a space:\n%s",
			url, body[max(0, i-80):min(len(body), i+80)])
	}
	for _, tag := range []string{"<body >", "<body  ", "<template >", "<template  "} {
		if strings.Contains(body, tag) {
			t.Errorf("%s writes %q, a separator nothing follows", url, tag)
		}
	}
}

// testAttributeSeparation tests every page the case lists, not only the index.
// The attributes that sit next to each other are the ones a page with several
// reflect signals or with both a stream and a reflect signal writes.
func (c Case) testAttributeSeparation(t *testing.T) {
	if len(c.Links) == 0 {
		t.Skip("the case lists no links")
	}
	v := c.visit(t)
	for _, link := range c.Links {
		resp := v.Get(t, link)
		if resp.Status != http.StatusOK {
			continue // A page the case answers with a status of its own.
		}
		checkAttributeSeparation(t, link, resp.Body)
	}
}

// testCompression tests the response a browser actually asks for.
// Compression is on by default.
// A page that renders only uncompressed reaches no browser.
func (c Case) testCompression(t *testing.T) {
	v := c.visit(t)

	// The visitor's transport decodes no response, which keeps what the server
	// chose visible.
	req := v.NewRequest(t, http.MethodGet, c.index(), nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp := v.Send(t, req)

	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc == "gzip" {
		if strings.Contains(resp.Body, "<!DOCTYPE html>") {
			t.Error("the response claims to be gzip and is not encoded")
		}
		return
	}
	if !strings.Contains(resp.Body, "<!DOCTYPE html>") {
		t.Errorf("an uncompressed response is not the page:\n%s", resp.Body)
	}
}

// testUnknownRoute tests a URL no page claims.
// Whatever the app supplies for it, the router must not serve it something else.
func (c Case) testUnknownRoute(t *testing.T) {
	resp := c.visit(t).Get(t, "/no-such-page-a1b2c3/")
	if resp.Status == http.StatusOK && strings.Contains(resp.Body, "<!DOCTYPE html>") {
		// An app with a PageError404 renders it here. That page is allowed;
		// serving some other page at this URL is not.
		if !strings.Contains(strings.ToLower(resp.Body), "not found") {
			t.Errorf("an unclaimed URL was served a page:\n%s", resp.Body)
		}
	}
}

// testGeneratedLinksAreRouted tests every URL the generated href package builds.
//
// The builder and the router are written from 1 model and form the 2
// halves of every link in the application. If they disagree the link is dead.
// Neither half can detect that by itself.
func (c Case) testGeneratedLinksAreRouted(t *testing.T) {
	if len(c.Links) == 0 {
		t.Skip("the case lists no links")
	}
	v := c.visit(t)

	unrouted := unroutedBody(t, v, nil, http.MethodGet)

	for _, link := range c.Links {
		t.Run(link, func(t *testing.T) {
			resp := v.Get(t, link)
			if resp.Status == http.StatusNotFound && resp.Body == unrouted {
				t.Errorf("href builds %s, which the router does not serve", link)
			}
			if resp.Status == http.StatusMethodNotAllowed {
				t.Errorf("href builds %s, which the router does not serve by GET", link)
			}
		})
	}
}

// unroutedBody asks for a URL nothing claims and returns what the server answers with:
// a page load for GET and an action of tab for any other method.
//
// A 404 alone does not mean a URL is unrouted. A handler may answer 404
// because the thing it was asked for does not exist. The body separates the 2 cases.
// An unrouted URL is answered by the router, with the app's own
// error page when it has one.
func unroutedBody(
	t *testing.T, v *datapagestest.Visitor, tab *datapagestest.Tab, method string,
) string {
	t.Helper()
	const nowhere = "/no-such-route-a1b2c3d4/"
	var resp *datapagestest.Response
	if method == http.MethodGet {
		resp = v.Get(t, nowhere)
	} else {
		resp = tab.Do(t, method, nowhere, nil)
	}
	if resp.Status != http.StatusNotFound {
		// An app whose 404 page is served with 200 has no distinguishable body here.
		// The comparison is left to match nothing.
		return "\x00 unrouted responses are not 404"
	}
	return resp.Body
}

// testGeneratedActionsAreRouted tests every expression the generated action
// package builds, in the same way and for the same reason.
func (c Case) testGeneratedActionsAreRouted(t *testing.T) {
	if len(c.Actions) == 0 {
		t.Skip("the case lists no actions")
	}
	v := c.visit(t)
	tab := v.Load(t, c.index())

	unrouted := map[string]string{}
	for _, expr := range c.Actions {
		t.Run(expr, func(t *testing.T) {
			method, target := datapagestest.ParseAction(t, expr)
			if _, ok := unrouted[method]; !ok {
				unrouted[method] = unroutedBody(t, v, tab, method)
			}
			resp := tab.Do(t, method, target, nil)
			if resp.Status == http.StatusNotFound && resp.Body == unrouted[method] {
				t.Errorf("action builds %s %s, which the router does not serve",
					method, target)
			}
			if resp.Status == http.StatusMethodNotAllowed {
				t.Errorf("action builds %s %s, which the router serves by another method",
					method, target)
			}
		})
	}
}

// testMalformedSignals tests the actions that read signals,
// sent a body that is not signals. The server must refuse such a request.
// Refusing it must not take the server down.
func (c Case) testMalformedSignals(t *testing.T) {
	if len(c.SignalActions) == 0 {
		t.Skip("the case lists no actions that read signals")
	}
	v := c.visit(t)
	tab := v.Load(t, c.index())

	for _, expr := range c.SignalActions {
		t.Run(expr, func(t *testing.T) {
			method, target := datapagestest.ParseAction(t, expr)
			resp := tab.Do(t, method, target, json.RawMessage(`{"broken": `))
			if resp.Status < 400 {
				t.Errorf("%s %s: status = %d on a body that is not signals",
					method, target, resp.Status)
			}
		})
	}

	// The server keeps serving afterwards.
	if resp := v.Get(t, c.index()); resp.Status != http.StatusOK {
		t.Errorf("the server stopped serving pages: status = %d", resp.Status)
	}
}

// signals turns a Case body into the signals of [datapagestest.Tab.Do]: the JSON as
// it is, or nil for an empty body, which sends "{}" the way Datastar does.
func signals(body string) any {
	if body == "" {
		return nil
	}
	return json.RawMessage(body)
}

// testStream tests the SSE connection of a page with events or stream hooks.
//
// The stream is the only long-lived resource a generated server holds, and a
// test that only sends requests never touches it.
// Here it is opened, closed, and opened again, the way a browser does across a reload.
func (c Case) testStream(t *testing.T) {
	if c.StreamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	// The bubble's clock lets an hour pass at once. t.Run panics inside a bubble,
	// which keeps both rounds in one test.
	synctest.Test(t, func(t *testing.T) {
		v := c.visit(t)

		var st *datapagestest.Stream
		for _, round := range []string{"first", "after a reload"} {
			// A reload closes the stream of the page it leaves.
			if st != nil {
				st.Close()
			}
			st = v.Load(t, c.index()).Connect(t, nil)

			// The connection stays open rather than ending.
			time.Sleep(time.Hour)
			if st.Ended() {
				t.Errorf("%s: the stream ended by itself", round)
			}
		}

		// The page keeps serving while a stream of it is open.
		if resp := v.Get(t, c.index()); resp.Status != http.StatusOK {
			t.Errorf("GET %s: status = %d while a stream is open", c.index(), resp.Status)
		}
	})
}

// testTrailingSlash tests a URL typed without its trailing slash.
//
// Every route the generator writes ends in one and every link it builds carries it.
// A visitor who types the URL, or a service that trims it, does not.
// The server normalizes the path instead of redirecting.
// Both forms must render the same page.
func (c Case) testTrailingSlash(t *testing.T) {
	var target string
	for _, link := range c.Links {
		if len(link) > 1 && strings.HasSuffix(link, "/") &&
			!strings.Contains(link, "?") {
			target = link
			break
		}
	}
	if target == "" {
		t.Skip("the case has no link below the root")
	}
	v := c.visit(t)

	withSlash := v.Get(t, target)
	without := v.Get(t, strings.TrimSuffix(target, "/"))

	if without.Status != withSlash.Status {
		t.Errorf("GET %s: status = %d with a trailing slash and %d without",
			target, withSlash.Status, without.Status)
	}
	if withSlash.Instance() != "" || withSlash.Header.Get("Set-Cookie") != "" {
		// A stateful page mints an id per load, a page that issues a session
		// mints a token the CSRF script carries. 2 loads of either differ
		// by design and only the status can be compared.
		return
	}
	if without.Body != withSlash.Body {
		t.Errorf("GET %s renders differently without its trailing slash", target)
	}
}

// testShutdownClosesStreams tests what happens to an open stream when the
// server is told to stop. A shutdown that waits for connections the client will not
// close never finishes. The deployment then has to kill the process to roll forward.
func (c Case) testShutdownClosesStreams(t *testing.T) {
	if c.StreamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv, handle := c.serveHandle(t)

	st := srv.Visitor(t).Load(t, c.index()).Connect(t, nil)

	ctx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := handle.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	st.WaitEnd(t, "an open stream outlived the shutdown")
}

// testStreamRequiresDatastar tests the stream route reached by a client that
// is not the Datastar runtime. A plain browser navigating there would hang on
// a response it cannot read.
func (c Case) testStreamRequiresDatastar(t *testing.T) {
	if c.StreamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	resp := c.visit(t).Get(t, c.StreamPath)
	if resp.Status != http.StatusNotAcceptable {
		t.Errorf("GET %s: status = %d, want %d",
			c.StreamPath, resp.Status, http.StatusNotAcceptable)
	}
}

// testDispatchReachesTheStream tests the central path of the framework.
// An action dispatches an event and the page that handles it sees the result on
// the connection it already holds.
//
// The dispatch closure, the subject, the subscription, the event loop and the
// handler call are all generated.
// No single piece can tell whether the next one is listening.
func (c Case) testDispatchReachesTheStream(t *testing.T) {
	if c.StreamPath == "" || c.DispatchAction == "" {
		t.Skip("the app has no action that dispatches to a stream")
	}
	tab := c.visit(t).Load(t, c.index())
	st := tab.Connect(t, nil)

	method, target := datapagestest.ParseAction(t, c.DispatchAction)
	resp := tab.Do(t, method, target, signals(c.DispatchBody))
	if resp.Status != http.StatusOK {
		t.Fatalf("%s %s: status = %d", method, target, resp.Status)
	}

	st.WaitFor(t, datapagestest.Func("an event", func(datapagestest.Event) bool {
		return true
	}))
}

// testStateIsReleased covers the end of a tab's life.
//
// The state a tab holds lives exactly as long as its stream. A stream that
// drops releases it. Otherwise every tab that ever connected stays in memory.
func (c Case) testStateIsReleased(t *testing.T) {
	if c.StateAction == "" {
		t.Skip("the app holds no per-tab state")
	}
	srv := c.serve(t)
	tab := srv.Visitor(t).Load(t, c.index())
	if tab.Instance() == "" {
		t.Fatal("a stateful page load minted no instance id")
	}
	st := tab.Connect(t, nil)

	method, target := datapagestest.ParseAction(t, c.StateAction)
	act := func() *datapagestest.Response {
		return tab.Do(t, method, target, signals(c.StateActionBody))
	}
	if resp := act(); resp.Status != http.StatusOK {
		t.Fatalf("%s %s: status = %d while the stream is open",
			method, target, resp.Status)
	}

	st.Close()

	// The release is prompt, but the server learns of the drop on its own
	// schedule, which is what this polls for rather than sleeps out.
	var after *datapagestest.Response
	srv.WaitFor(t, fmt.Sprintf("%s %s to answer %d after the stream closed",
		method, target, http.StatusConflict), func() bool {
		after = act()
		return after.Status == http.StatusConflict
	})
	if retry := after.Retry(); retry != "reconnect" {
		t.Errorf("Datapages-Retry = %q, want %q", retry, "reconnect")
	}
}

// testExternalHref tests href.External. A template uses it for a URL this
// application does not own. It hands the URL back unchanged and warns when it
// is given one the application does own. Such a URL belongs in a generated builder,
// which keeps up with the routes.
func (c Case) testExternalHref(t *testing.T) {
	if c.HrefExternal == nil || c.HrefSetLogger == nil {
		t.Skip("the case wires no href package")
	}
	var buf buffer
	c.HrefSetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { c.HrefSetLogger(nil) })

	const external = "https://example.com/docs"
	if got := c.HrefExternal(external); got != external {
		t.Errorf("External(%q) = %q, want it unchanged", external, got)
	}
	if logged := buf.String(); logged != "" {
		t.Errorf("an external URL was warned about:\n%s", logged)
	}

	const internal = "/some/app/path"
	if got := c.HrefExternal(internal); got != internal {
		t.Errorf("External(%q) = %q, want it unchanged", internal, got)
	}
	if logged := buf.String(); !strings.Contains(logged, internal) {
		t.Errorf("an app-internal URL was not warned about:\n%s", logged)
	}

	// The templ sanitizer keeps 6 schemes and rewrites the rest to
	// about:invalid, which is a link the visitor cannot follow.
	buf.Reset()
	const dropped = "sms:+15550100"
	if got := c.HrefExternal(dropped); got != dropped {
		t.Errorf("External(%q) = %q, want it unchanged", dropped, got)
	}
	if logged := buf.String(); !strings.Contains(logged, "about:invalid") {
		t.Errorf("a URL the sanitizer drops was not warned about:\n%s", logged)
	}
}

// testClientGoesAway tests a page load whose client disappears while
// the page is being written.
//
// Every write of the page shell can fail from that point on and the generated code
// checks each one. It must not panic, and it must not keep writing into a connection
// that is gone. 1 visitor closing a tab would otherwise take the process down.
func (c Case) testClientGoesAway(t *testing.T) {
	handler := c.NewServer(t)

	// The cut-off points walk through the shell: the opening tags, the head,
	// the body attributes, the body itself and the closing tags.
	for _, after := range []int{0, 1, 32, 128, 512, 2048} {
		t.Run(fmt.Sprint("after ", after, " bytes"), func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodGet, c.index(), nil,
			)
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			w := &failingWriter{header: http.Header{}, left: after}

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("a client that went away panicked the server: %v", r)
				}
			}()
			handler.ServeHTTP(w, req)

			if w.written > after {
				t.Errorf("the server wrote %d bytes past a failing connection",
					w.written-after)
			}
		})
	}
}

// failingWriter accepts left bytes and fails every write after that.
// A real ResponseWriter behaves this way once its client is gone.
type failingWriter struct {
	header  http.Header
	left    int
	written int
	status  int
}

func (w *failingWriter) Header() http.Header { return w.header }

func (w *failingWriter) WriteHeader(status int) { w.status = status }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.left <= 0 {
		return 0, errors.New("write: connection reset by peer")
	}
	n := min(len(p), w.left)
	w.left -= n
	w.written += n
	if n < len(p) {
		return n, errors.New("write: connection reset by peer")
	}
	return n, nil
}

// Flush is required by the compression middleware and by SSE.
func (w *failingWriter) Flush() {}

// testMiddleware tests WithMiddleware: several are applied in the order they
// were given, and one of them can answer instead of passing the request on.
func (c Case) testMiddleware(t *testing.T) {
	if c.WithMiddleware == nil {
		t.Skip("the case wires no WithMiddleware")
	}
	var mu sync.Mutex
	var order []string
	tag := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				next.ServeHTTP(w, r)
			})
		}
	}

	// 1 call carrying 2 and a second call carrying 1: all 3 run in
	// the order they were written, across calls.
	c.visit(t,
		c.WithMiddleware(tag("first"), tag("second")),
		c.WithMiddleware(tag("third"))).Get(t, c.index())

	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if want := "first,second,third"; got != want {
		t.Errorf("middleware ran in order %q, want %q", got, want)
	}

	refusing := c.visit(t,
		c.WithMiddleware(func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "no", http.StatusTeapot)
			})
		}))
	if resp := refusing.Get(t, c.index()); resp.Status != http.StatusTeapot {
		t.Errorf("status = %d, want %d", resp.Status, http.StatusTeapot)
	}
}

// testDatastarJS tests WithDatastarJS. The page shell loads the client script
// from the source the operator chose. A deployment uses this to serve the
// script from its own origin instead of a CDN.
func (c Case) testDatastarJS(t *testing.T) {
	if c.WithDatastarJS == nil {
		t.Skip("the case wires no WithDatastarJS")
	}
	const src = "/vendor/datastar-a1b2c3.js"
	resp := c.visit(t, c.WithDatastarJS(src)).Get(t, c.index())
	if !strings.Contains(resp.Body, src) {
		t.Errorf("the page does not load the configured script:\n%s", resp.Body)
	}
}

// testBuildCheck tests that a Datastar request from another build is answered
// 205 Reset Content instead of running a handler. A request with the current
// build ID or none passes.
func (c Case) testBuildCheck(t *testing.T) {
	if c.WithBuildID == nil {
		t.Skip("the case wires no WithBuildID")
	}
	const build = "contract-build"
	v := c.visit(t, c.WithBuildID(build))
	tab := v.Load(t, c.index())

	meta := `<meta name="datapages-build" content="` + build + `"/>`
	if !strings.Contains(tab.Page().Body, meta) {
		t.Errorf("the page does not carry %s:\n%s", meta, tab.Page().Body)
	}

	var routes [][2]string
	if c.StreamPath != "" {
		routes = append(routes, [2]string{http.MethodGet, c.StreamPath})
	}
	for _, expr := range c.Actions {
		method, target := datapagestest.ParseAction(t, expr)
		routes = append(routes, [2]string{method, target})
	}
	for _, route := range routes {
		method, target := route[0], route[1]
		t.Run(method+" "+target, func(t *testing.T) {
			status, header, body := sendBuild(t, v, tab, method, target, "another-build")
			if status != http.StatusResetContent {
				t.Errorf("another build: status = %d, want 205", status)
			}
			if header != build {
				t.Errorf("another build: Datapages-Build = %q, want %q", header, build)
			}
			if body != "" {
				t.Errorf("another build: the handler wrote %q", body)
			}
			for _, sent := range []string{build, ""} {
				if status, _, _ := sendBuild(t, v, tab, method, target, sent); status ==
					http.StatusResetContent {
					t.Errorf("build %q: status 205, want the handler's answer", sent)
				}
			}
		})
	}
}

// sendBuild sends a Datastar request of tab with an optional build ID through the
// visitor's client. It reads the body of a 205 only: an open stream does not end.
func sendBuild(
	t *testing.T, v *datapagestest.Visitor, tab *datapagestest.Tab,
	method, target, build string,
) (status int, header, body string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req := tab.NewRequest(t, method, target, nil).WithContext(ctx)
	if build != "" {
		req.Header.Set("Datapages-Build", build)
	}
	resp, err := v.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusResetContent {
		b, _ := io.ReadAll(resp.Body)
		body = string(b)
	}
	return resp.StatusCode, resp.Header.Get("Datapages-Build"), body
}

// testHTTPServerOption tests WithHTTPServer.
// A deployment uses it to set its own timeouts.
func (c Case) testHTTPServerOption(t *testing.T) {
	if c.WithHTTPServer == nil {
		t.Skip("the case wires no WithHTTPServer")
	}
	v := c.visit(t, c.WithHTTPServer(
		&http.Server{ReadHeaderTimeout: 3 * time.Second},
	))

	if resp := v.Get(t, c.index()); resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

// testMessageBrokerStreamSubjects tests what the server exports for an
// operator to create streams with before it starts. An empty subject,
// or one listed 2 times, cannot be turned into a stream.
func (c Case) testMessageBrokerStreamSubjects(t *testing.T) {
	if c.StreamSubjects == nil {
		t.Skip("the case wires no MessageBrokerStreamSubjects")
	}
	seen := map[string]bool{}
	for _, s := range c.StreamSubjects() {
		if strings.TrimSpace(s) == "" {
			t.Error("an exported stream subject is empty")
		}
		if seen[s] {
			t.Errorf("stream subject %q is exported 2 times", s)
		}
		seen[s] = true
	}
}

// testLoggerOption tests that the logger the option carries is the one the
// generated server logs to. The shutdown line is what every server logs
// without serving a request.
//
// The lifecycle behind [Server.ListenAndServe] belongs to the runtime,
// which tests it once instead of once per case.
func (c Case) testLoggerOption(t *testing.T) {
	if c.WithLogger == nil {
		t.Skip("the case wires no WithLogger")
	}
	var buf buffer
	s := c.NewServer(t, c.WithLogger(
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	if logged := buf.String(); !strings.Contains(logged, "shutdown") {
		t.Errorf("the configured logger received no shutdown line:\n%s", logged)
	}
}

// buffer is a bytes.Buffer a logger and a test can both touch.
type buffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
