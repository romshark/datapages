// Package contract holds the assertions that apply to every generated server
// regardless of the application: how the server is constructed and configured,
// what it wraps a page in, whether the URLs it generates address the routes it serves,
// and whether it starts and stops.
//
// The server is generated per model. The same assertion therefore runs against
// a different piece of code in each case, which is why the suite is shared.
//
// A case module wires itself in with a Case value and one test:
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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server is the generated *Server, named by what the suite calls on it.
// ListenAndServe and ListenAndServeTLS are here so that every generated
// server is asserted to expose them. What they do is asserted in
// runtime/httpserve, which holds the code behind them.
type Server interface {
	http.Handler
	ListenAndServe(ctx context.Context, addr string) error
	ListenAndServeTLS(ctx context.Context, addr, certFile, keyFile string) error
	Shutdown(ctx context.Context) error
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

	// The fields below hand the suite one generated
	// server option each. A nil field skips the assertions that need it.
	WithMiddleware func(...func(http.Handler) http.Handler) any
	WithDatastarJS func(src string) any
	WithHTTPServer func(*http.Server) any
	WithLogger     func(*slog.Logger) any

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

	// StreamPath is the SSE route of a page that has one,
	// "" when the app has no page that handles events or hooks a stream.
	StreamPath string

	// DispatchAction is an action that dispatches an event the page behind
	// StreamPath handles, and DispatchBody the signals it reads.
	DispatchAction string
	DispatchBody   string

	// OptionedAction is one of the case's action expressions built with every
	// option the generated package offers, in the order listed by optionKeys.
	// A template can pass any combination to any action. Every generated
	// builder must write one well-formed expression out of whatever it is given.
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

	// The options are one JavaScript object literal.
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

func (c Case) server(t *testing.T, opts ...any) *httptest.Server {
	t.Helper()
	srv, _ := c.serverAndHandle(t, opts...)
	return srv
}

// serverAndHandle returns the test server together with the generated server behind it,
// for the assertions that call its own methods.
func (c Case) serverAndHandle(t *testing.T, opts ...any) (*httptest.Server, Server) {
	t.Helper()
	handle := c.NewServer(t, opts...)
	srv := httptest.NewServer(handle)
	t.Cleanup(srv.Close)
	return srv, handle
}

// get performs a plain page load without content negotiation.
// The bytes it reads are then the bytes the handler wrote.
func get(t *testing.T, srv *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil,
	)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return resp, string(b)
}

// testPageShell tests what the server wraps every page body in.
// Nothing else assembles the document.
// Every page that renders at all renders inside this.
func (c Case) testPageShell(t *testing.T) {
	srv := c.server(t)

	resp, body := get(t, srv, c.index())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", c.index(), resp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"), "text/html"; !strings.HasPrefix(got, want) {
		t.Errorf("Content-Type = %q, want prefix %q", got, want)
	}
	for _, want := range []string{"<!DOCTYPE html>", "<html", "<head", "<body", "</html>"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page shell does not carry %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "<script") {
		t.Errorf("the page shell loads no client script:\n%s", body)
	}
}

// testCompression tests the response a browser actually asks for.
// Compression is on by default.
// A page that renders only uncompressed reaches no browser.
func (c Case) testCompression(t *testing.T) {
	srv := c.server(t)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+c.index(), nil,
	)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	// The transport strips Content-Encoding when it added the header itself.
	// Setting the header here keeps what the server chose visible.
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", c.index(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc == "gzip" {
		if bytes.Contains(b, []byte("<!DOCTYPE html>")) {
			t.Error("the response claims to be gzip and is not encoded")
		}
		return
	}
	if !bytes.Contains(b, []byte("<!DOCTYPE html>")) {
		t.Errorf("an uncompressed response is not the page:\n%s", b)
	}
}

// testUnknownRoute tests a URL no page claims.
// Whatever the app supplies for it, the router must not serve it something else.
func (c Case) testUnknownRoute(t *testing.T) {
	srv := c.server(t)

	resp, body := get(t, srv, "/no-such-page-a1b2c3/")
	if resp.StatusCode == http.StatusOK && strings.Contains(body, "<!DOCTYPE html>") {
		// An app with a PageError404 renders it here. That page is allowed;
		// serving some other page at this URL is not.
		if !strings.Contains(strings.ToLower(body), "not found") {
			t.Errorf("an unclaimed URL was served a page:\n%s", body)
		}
	}
}

// testGeneratedLinksAreRouted tests every URL the generated href package builds.
//
// The builder and the router are written from one model and form the two
// halves of every link in the application. If they disagree the link is dead.
// Neither half can detect that by itself.
func (c Case) testGeneratedLinksAreRouted(t *testing.T) {
	if len(c.Links) == 0 {
		t.Skip("the case lists no links")
	}
	srv := c.server(t)

	unrouted := unroutedBody(t, srv, http.MethodGet)

	for _, link := range c.Links {
		t.Run(link, func(t *testing.T) {
			resp, body := get(t, srv, link)
			if resp.StatusCode == http.StatusNotFound && body == unrouted {
				t.Errorf("href builds %s, which the router does not serve", link)
			}
			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("href builds %s, which the router does not serve by GET", link)
			}
		})
	}
}

// unroutedBody asks for a URL nothing claims and returns what the server answers with.
//
// A 404 alone does not mean a URL is unrouted. A handler may answer 404
// because the thing it was asked for does not exist. The body separates the two cases.
// An unrouted URL is answered by the router, with the app's own
// error page when it has one.
func unroutedBody(t *testing.T, srv *httptest.Server, method string) string {
	t.Helper()
	const nowhere = "/no-such-route-a1b2c3d4/"
	if method == http.MethodGet {
		resp, body := get(t, srv, nowhere)
		if resp.StatusCode != http.StatusNotFound {
			// An app whose 404 page is served with 200 has no distinguishable body here.
			// The comparison is left to match nothing.
			return "\x00 unrouted responses are not 404"
		}
		return body
	}
	resp := send(t, srv, method, nowhere, "")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response of an unrouted URL: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		return "\x00 unrouted responses are not 404"
	}
	return string(b)
}

// testGeneratedActionsAreRouted tests every expression the generated action
// package builds, in the same way and for the same reason.
func (c Case) testGeneratedActionsAreRouted(t *testing.T) {
	if len(c.Actions) == 0 {
		t.Skip("the case lists no actions")
	}
	srv := c.server(t)

	unrouted := map[string]string{}
	for _, expr := range c.Actions {
		t.Run(expr, func(t *testing.T) {
			method, target := parseAction(t, expr)
			if _, ok := unrouted[method]; !ok {
				unrouted[method] = unroutedBody(t, srv, method)
			}
			resp := send(t, srv, method, target, "")
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading %s %s: %v", method, target, err)
			}
			if resp.StatusCode == http.StatusNotFound && string(b) == unrouted[method] {
				t.Errorf("action builds %s %s, which the router does not serve",
					method, target)
			}
			if resp.StatusCode == http.StatusMethodNotAllowed {
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
	srv := c.server(t)

	for _, expr := range c.SignalActions {
		t.Run(expr, func(t *testing.T) {
			method, target := parseAction(t, expr)
			resp := send(t, srv, method, target, `{"broken": `)
			_ = resp.Body.Close()
			if resp.StatusCode < 400 {
				t.Errorf("%s %s: status = %d on a body that is not signals",
					method, target, resp.StatusCode)
			}
		})
	}

	// The server keeps serving afterwards.
	if resp, _ := get(t, srv, c.index()); resp.StatusCode != http.StatusOK {
		t.Errorf("the server stopped serving pages: status = %d", resp.StatusCode)
	}
}

// parseAction takes the method and URL out of an expression such as
// "@post('/form/submit/')". That is the form a template carries.
func parseAction(t *testing.T, expr string) (method, target string) {
	t.Helper()
	at := strings.Index(expr, "@")
	open := strings.Index(expr, "('")
	closing := strings.LastIndex(expr, "')")
	if at < 0 || open < at || closing < open {
		t.Fatalf("not an action expression: %s", expr)
	}
	switch verb := expr[at+1 : open]; verb {
	case "post":
		method = http.MethodPost
	case "put":
		method = http.MethodPut
	case "patch":
		method = http.MethodPatch
	case "delete":
		method = http.MethodDelete
	case "get":
		method = http.MethodGet
	default:
		t.Fatalf("unknown action verb %q in %s", verb, expr)
	}
	target = expr[open+2 : closing]
	if i := strings.Index(target, "'"); i >= 0 {
		target = target[:i] // the expression carries options after the URL
	}
	if _, err := url.Parse(target); err != nil {
		t.Fatalf("action %s builds an unparsable URL: %v", expr, err)
	}
	return method, target
}

func send(
	t *testing.T, srv *httptest.Server, method, target, body string,
) *http.Response {
	t.Helper()
	return sendAs(t, srv, method, target, body, "")
}

// sendAs sends an action request as the tab named by instanceID.
func sendAs(
	t *testing.T, srv *httptest.Server, method, target, body, instanceID string,
) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+target, r,
	)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, target, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if instanceID != "" {
		req.Header.Set("Datapages-Instance", instanceID)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return resp
}

// testStream tests the SSE connection of a page with events or stream hooks.
//
// The stream is the one long-lived resource a generated server holds, and a
// test that only sends requests never touches it.
// Here it is opened, closed, and opened again, the way a browser does across a reload.
func (c Case) testStream(t *testing.T) {
	if c.StreamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv := c.server(t)

	for _, round := range []string{"first", "after a reload"} {
		t.Run(round, func(t *testing.T) {
			body, _, cancel := c.openStream(t, srv)
			defer cancel()

			// The connection stays open: reading blocks rather than ending.
			read := make(chan error, 1)
			go func() {
				buf := make([]byte, 1)
				_, err := body.Read(buf)
				read <- err
			}()
			select {
			case err := <-read:
				if err != nil {
					t.Errorf("the stream ended by itself: %v", err)
				}
			case <-time.After(300 * time.Millisecond):
				// Still open, as a stream should be.
			}
		})
	}

	// The page keeps serving while a stream of it is open.
	if resp, _ := get(t, srv, c.index()); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s: status = %d while a stream is open",
			c.index(), resp.StatusCode)
	}
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
	srv := c.server(t)

	withSlash, slashBody := get(t, srv, target)
	without, plainBody := get(t, srv, strings.TrimSuffix(target, "/"))

	if without.StatusCode != withSlash.StatusCode {
		t.Errorf("GET %s: status = %d with a trailing slash and %d without",
			target, withSlash.StatusCode, without.StatusCode)
	}
	if withSlash.Header.Get("Datapages-Instance") != "" {
		// A stateful page mints an id per load. Two loads of it differ by
		// design and only the status can be compared.
		return
	}
	if plainBody != slashBody {
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
	srv, handle := c.serverAndHandle(t)

	body, _, cancel := c.openStream(t, srv)
	defer cancel()

	ended := make(chan struct{})
	go func() {
		defer close(ended)
		_, _ = io.Copy(io.Discard, body)
	}()

	ctx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := handle.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("an open stream outlived the shutdown")
	}
}

// testStreamRequiresDatastar tests the stream route reached by a client that
// is not the Datastar runtime. A plain browser navigating there would hang on
// a response it cannot read.
func (c Case) testStreamRequiresDatastar(t *testing.T) {
	if c.StreamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv := c.server(t)

	resp, _ := get(t, srv, c.StreamPath)
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Errorf("GET %s: status = %d, want %d",
			c.StreamPath, resp.StatusCode, http.StatusNotAcceptable)
	}
}

// openStream connects to the case's stream the way the client does.
//
// A stateful page hands out an instance id on the page load and expects it
// back on the stream. The page load therefore comes first in either case.
func (c Case) openStream(
	t *testing.T, srv *httptest.Server,
) (body io.ReadCloser, instanceID string, cancel context.CancelFunc) {
	t.Helper()

	page, err := srv.Client().Get(srv.URL + c.index())
	if err != nil {
		t.Fatalf("GET %s: %v", c.index(), err)
	}
	_ = page.Body.Close()
	instance := page.Header.Get("Datapages-Instance")

	// The context's cancel is kept under its own name. The returned closure
	// is also called cancel. Calling that one from inside itself gives a stack
	// overflow instead of a closed stream.
	ctx, closeConn := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+c.StreamPath, nil,
	)
	if err != nil {
		closeConn()
		t.Fatalf("building the stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if instance != "" {
		req.Header.Set("Datapages-Instance", instance)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		closeConn()
		t.Fatalf("opening the stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		closeConn()
		t.Fatalf("opening the stream: status %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"),
		"text/event-stream"; !strings.HasPrefix(got, want) {
		t.Errorf("Content-Type = %q, want prefix %q", got, want)
	}
	return resp.Body, instance, func() {
		closeConn()
		_ = resp.Body.Close()
		// Give the server the moment it needs to run the close hooks.
		time.Sleep(100 * time.Millisecond)
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
	srv := c.server(t)

	body, instance, cancel := c.openStream(t, srv)
	defer cancel()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := body.Read(buf)
		if err != nil {
			received <- ""
			return
		}
		received <- string(buf[:n])
	}()

	method, target := parseAction(t, c.DispatchAction)
	resp := sendAs(t, srv, method, target, c.DispatchBody, instance)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s: status = %d", method, target, resp.StatusCode)
	}

	select {
	case got := <-received:
		if got == "" {
			t.Error("the stream ended instead of carrying the event")
		}
		if !strings.Contains(got, "event:") {
			t.Errorf("what arrived is not an SSE event:\n%s", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("the dispatched event never reached the stream")
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

	// The templ sanitizer keeps six schemes and rewrites the rest to
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
// that is gone. One visitor closing a tab would otherwise take the process down.
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

	// One call carrying two and a second call carrying one: all three run in
	// the order they were written, across calls.
	srv := c.server(t,
		c.WithMiddleware(tag("first"), tag("second")),
		c.WithMiddleware(tag("third")))
	_, _ = get(t, srv, c.index())

	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if want := "first,second,third"; got != want {
		t.Errorf("middleware ran in order %q, want %q", got, want)
	}

	refusing := c.server(t,
		c.WithMiddleware(func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "no", http.StatusTeapot)
			})
		}))
	if resp, _ := get(t, refusing, c.index()); resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTeapot)
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
	srv := c.server(t, c.WithDatastarJS(src))

	if _, body := get(t, srv, c.index()); !strings.Contains(body, src) {
		t.Errorf("the page does not load the configured script:\n%s", body)
	}
}

// testHTTPServerOption tests WithHTTPServer.
// A deployment uses it to set its own timeouts.
func (c Case) testHTTPServerOption(t *testing.T) {
	if c.WithHTTPServer == nil {
		t.Skip("the case wires no WithHTTPServer")
	}
	srv := c.server(t, c.WithHTTPServer(
		&http.Server{ReadHeaderTimeout: 3 * time.Second},
	))

	if resp, _ := get(t, srv, c.index()); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// testMessageBrokerStreamSubjects tests what the server exports for an
// operator to create streams with before it starts. An empty subject,
// or one listed twice, cannot be turned into a stream.
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
			t.Errorf("stream subject %q is exported twice", s)
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
