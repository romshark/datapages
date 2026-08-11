// This file is copied into every acceptance case module by the harness. It
// holds the assertions that apply to every generated server regardless of the
// application: how the server is constructed and configured, what it wraps a
// page in, whether the URLs it generates address the routes it serves, and
// whether it starts and stops.
//
// The server is generated per model. The same assertion therefore runs against
// a different piece of code in each case, which is why the file is shared.
//
// Each case supplies a contractCase value. See README.md.

package acceptance_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/href"
)

// contract is what a case tells the shared suite about itself.
type contract struct {
	// newServer builds this case's server with the given extra options. The
	// suite cannot do this itself. The argument list of NewServer depends on
	// the model, and stateful or metered apps require options of their own.
	newServer func(t *testing.T, opts ...datapagesgen.ServerOption) *datapagesgen.Server

	// index is a URL that renders a page. Defaults to "/".
	index string

	// links are URLs built by the generated href package. Every one of them
	// must address a route the server serves.
	links []string

	// actions are expressions built by the generated action package, in the
	// form the templates carry them: "@post('/x/')". Every one of them must
	// address a route the server serves with that method.
	actions []string

	// signalActions are the subset of actions whose handlers read signals.
	// Each is sent a body that is not signals. The server must refuse the
	// request instead of crashing.
	signalActions []string

	// streamPath is the SSE route of a page that has one, "" when the app has
	// no page that handles events or hooks a stream.
	streamPath string

	// dispatchAction is an action that dispatches an event the page behind
	// streamPath handles, and dispatchBody the signals it reads.
	dispatchAction string
	dispatchBody   string

	// stateAction is an action that needs the calling tab's state, and
	// stateActionBody the signals it reads. Set together with stateGrace by
	// cases whose pages hold per-tab state.
	stateAction     string
	stateActionBody string

	// stateGrace is the grace period newServer configures. The suite waits it
	// out to observe a tab's state being released. The production default of
	// half a minute is too long for a test.
	stateGrace time.Duration

	// hasAssets says whether the case was generated with asset serving
	// configured. It decides what WithAssets is allowed to do.
	hasAssets bool

	// optionedAction is one of the case's action expressions built with every
	// option the generated package offers, in the order listed by
	// contractOptionKeys. A template can pass any combination to any action.
	// Every generated builder must write one well-formed expression out of
	// whatever it is given.
	optionedAction string
}

// contractStateGrace is the grace period a stateful case configures in the
// server it builds for this suite. It is short enough to wait out and long
// enough that a stream closing does not race it.
const contractStateGrace = 300 * time.Millisecond

// contractOptionKeys are the option keys of the expression a case builds for
// optionedAction, in the order the generated writer emits them.
var contractOptionKeys = []string{
	"contentType", "selector", "headers", "filterSignals", "openWhenHidden",
	"payload", "retry", "retryInterval", "retryScaler", "retryMaxWaitMs",
	"retryMaxCount", "requestCancellation",
}

// TestContractActionOptions covers the expression a template carries when it
// passes options to an action.
//
// The expression is a string with no runtime behind it. Nothing on the server
// can report one the browser cannot parse. The application does not find out
// either.
func TestContractActionOptions(t *testing.T) {
	expr := contractCase.optionedAction
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
	open := strings.Index(call, ", {")
	if open < 0 {
		t.Fatalf("the call carries no options object: %s", call)
	}
	if !strings.HasSuffix(call, "})") {
		t.Errorf("the options object is not closed: %s", call)
	}
	for _, key := range contractOptionKeys {
		if !strings.Contains(call, key+": ") {
			t.Errorf("the options object has no %s: %s", key, call)
		}
	}

	// A value carrying a quote is escaped rather than closing the string it
	// sits in, in a selector and in a header alike.
	if !strings.Contains(call, `selector: '#it\'s'`) {
		t.Errorf("the selector is not escaped: %s", call)
	}
	if !strings.Contains(call, `'it\'s here'`) {
		t.Errorf("the header value is not escaped: %s", call)
	}
	if !strings.Contains(call, "'X-Trace': 'abc'") {
		t.Errorf("the headers object lost a header: %s", call)
	}
	if contractUnbalanced(call) {
		t.Errorf("the expression's braces do not balance: %s", call)
	}
}

// contractUnbalanced reports whether the braces of an expression fail to pair
// up. That is the shape of a malformed options object.
func contractUnbalanced(s string) bool {
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

func contractIndex() string {
	if contractCase.index != "" {
		return contractCase.index
	}
	return "/"
}

func contractServer(
	t *testing.T, opts ...datapagesgen.ServerOption,
) *httptest.Server {
	t.Helper()
	srv, _ := contractServerAndHandle(t, opts...)
	return srv
}

// contractServerAndHandle returns the test server together with the generated
// server behind it, for the assertions that call its own methods.
func contractServerAndHandle(
	t *testing.T, opts ...datapagesgen.ServerOption,
) (*httptest.Server, *datapagesgen.Server) {
	t.Helper()
	handle := contractCase.newServer(t, opts...)
	srv := httptest.NewServer(handle)
	t.Cleanup(srv.Close)
	return srv, handle
}

// contractGet performs a plain page load without content negotiation. The
// bytes it reads are then the bytes the handler wrote.
func contractGet(
	t *testing.T, srv *httptest.Server, path string,
) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil)
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

// TestContractPageShell covers what the server wraps every page body in.
// Nothing else assembles the document. Every page that renders at all renders
// inside this.
func TestContractPageShell(t *testing.T) {
	srv := contractServer(t)

	resp, body := contractGet(t, srv, contractIndex())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", contractIndex(), resp.StatusCode)
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

// TestContractCompression covers the response a browser actually asks for.
// Compression is on by default. A page that renders only uncompressed reaches
// no browser.
func TestContractCompression(t *testing.T) {
	srv := contractServer(t)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+contractIndex(), nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	// The transport strips Content-Encoding when it added the header itself.
	// Setting the header here keeps what the server chose visible.
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", contractIndex(), err)
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

// TestContractUnknownRoute covers a URL no page claims. Whatever the app
// supplies for it, the router must not serve it something else.
func TestContractUnknownRoute(t *testing.T) {
	srv := contractServer(t)

	resp, body := contractGet(t, srv, "/no-such-page-a1b2c3/")
	if resp.StatusCode == http.StatusOK && strings.Contains(body, "<!DOCTYPE html>") {
		// An app with a PageError404 renders it here. That page is allowed;
		// serving some other page at this URL is not.
		if !strings.Contains(strings.ToLower(body), "not found") {
			t.Errorf("an unclaimed URL was served a page:\n%s", body)
		}
	}
}

// TestContractGeneratedLinksAreRouted covers every URL the generated href
// package builds.
//
// The builder and the router are written from one model and form the two
// halves of every link in the application. If they disagree the link is dead.
// Neither half can detect that by itself.
func TestContractGeneratedLinksAreRouted(t *testing.T) {
	if len(contractCase.links) == 0 {
		t.Skip("the case lists no links")
	}
	srv := contractServer(t)

	unrouted := contractUnroutedBody(t, srv, http.MethodGet)

	for _, link := range contractCase.links {
		t.Run(link, func(t *testing.T) {
			resp, body := contractGet(t, srv, link)
			if resp.StatusCode == http.StatusNotFound && body == unrouted {
				t.Errorf("href builds %s, which the router does not serve", link)
			}
			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("href builds %s, which the router does not serve by GET", link)
			}
		})
	}
}

// contractUnroutedBody asks for a URL nothing claims and returns what the
// server answers with.
//
// A 404 alone does not mean a URL is unrouted. A handler may answer 404
// because the thing it was asked for does not exist. The body separates the
// two cases. An unrouted URL is answered by the router, with the app's own
// error page when it has one.
func contractUnroutedBody(t *testing.T, srv *httptest.Server, method string) string {
	t.Helper()
	const nowhere = "/no-such-route-a1b2c3d4/"
	if method == http.MethodGet {
		resp, body := contractGet(t, srv, nowhere)
		if resp.StatusCode != http.StatusNotFound {
			// An app whose 404 page is served with 200 has no distinguishable
			// body here. The comparison is left to match nothing.
			return "\x00 unrouted responses are not 404"
		}
		return body
	}
	resp := contractSend(t, srv, method, nowhere, "")
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

// TestContractGeneratedActionsAreRouted covers every expression the generated
// action package builds, in the same way and for the same reason.
func TestContractGeneratedActionsAreRouted(t *testing.T) {
	if len(contractCase.actions) == 0 {
		t.Skip("the case lists no actions")
	}
	srv := contractServer(t)

	unrouted := map[string]string{}
	for _, expr := range contractCase.actions {
		t.Run(expr, func(t *testing.T) {
			method, target := contractParseAction(t, expr)
			if _, ok := unrouted[method]; !ok {
				unrouted[method] = contractUnroutedBody(t, srv, method)
			}
			resp := contractSend(t, srv, method, target, "")
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

// TestContractMalformedSignals covers the actions that read signals, sent a
// body that is not signals. The server must refuse such a request. Refusing it
// must not take the server down.
func TestContractMalformedSignals(t *testing.T) {
	if len(contractCase.signalActions) == 0 {
		t.Skip("the case lists no actions that read signals")
	}
	srv := contractServer(t)

	for _, expr := range contractCase.signalActions {
		t.Run(expr, func(t *testing.T) {
			method, target := contractParseAction(t, expr)
			resp := contractSend(t, srv, method, target, `{"broken": `)
			_ = resp.Body.Close()
			if resp.StatusCode < 400 {
				t.Errorf("%s %s: status = %d on a body that is not signals",
					method, target, resp.StatusCode)
			}
		})
	}

	// The server keeps serving afterwards.
	if resp, _ := contractGet(t, srv, contractIndex()); resp.StatusCode != http.StatusOK {
		t.Errorf("the server stopped serving pages: status = %d", resp.StatusCode)
	}
}

// contractParseAction takes the method and URL out of an expression such as
// "@post('/form/submit/')". That is the form a template carries.
func contractParseAction(t *testing.T, expr string) (method, target string) {
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

func contractSend(
	t *testing.T, srv *httptest.Server, method, target, body string,
) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+target, r)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, target, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return resp
}

// TestContractStream covers the SSE connection of a page with events or
// stream hooks.
//
// The stream is the one long-lived resource a generated server holds, and a
// test that only sends requests never touches it. Here it is opened, closed,
// and opened again, the way a browser does across a reload.
func TestContractStream(t *testing.T) {
	if contractCase.streamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv := contractServer(t)

	for _, round := range []string{"first", "after a reload"} {
		t.Run(round, func(t *testing.T) {
			body, _, cancel := contractOpenStream(t, srv)
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
	if resp, _ := contractGet(t, srv, contractIndex()); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s: status = %d while a stream is open",
			contractIndex(), resp.StatusCode)
	}
}

// TestContractTrailingSlash covers a URL typed without its trailing slash.
//
// Every route the generator writes ends in one and every link it builds
// carries it. A visitor who types the URL, or a service that trims it, does
// not. The server normalizes the path instead of redirecting. Both forms must
// render the same page.
func TestContractTrailingSlash(t *testing.T) {
	var target string
	for _, link := range contractCase.links {
		if len(link) > 1 && strings.HasSuffix(link, "/") &&
			!strings.Contains(link, "?") {
			target = link
			break
		}
	}
	if target == "" {
		t.Skip("the case has no link below the root")
	}
	srv := contractServer(t)

	withSlash, slashBody := contractGet(t, srv, target)
	without, plainBody := contractGet(t, srv, strings.TrimSuffix(target, "/"))

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

// TestContractShutdownClosesStreams covers what happens to an open stream when
// the server is told to stop. A shutdown that waits for connections the client
// will not close never finishes. The deployment then has to kill the process
// to roll forward.
func TestContractShutdownClosesStreams(t *testing.T) {
	if contractCase.streamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv, handle := contractServerAndHandle(t)

	body, _, cancel := contractOpenStream(t, srv)
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

// TestContractStreamRequiresDatastar covers the stream route reached by a
// client that is not the Datastar runtime. A plain browser navigating there
// would hang on a response it cannot read.
func TestContractStreamRequiresDatastar(t *testing.T) {
	if contractCase.streamPath == "" {
		t.Skip("the app has no page with a stream")
	}
	srv := contractServer(t)

	resp, _ := contractGet(t, srv, contractCase.streamPath)
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Errorf("GET %s: status = %d, want %d",
			contractCase.streamPath, resp.StatusCode, http.StatusNotAcceptable)
	}
}

// contractOpenStream connects to the case's stream the way the client does.
//
// A stateful page hands out an instance id on the page load and expects it
// back on the stream. The page load therefore comes first in either case.
func contractOpenStream(
	t *testing.T, srv *httptest.Server,
) (body io.ReadCloser, instanceID string, cancel context.CancelFunc) {
	t.Helper()

	page, err := srv.Client().Get(srv.URL + contractIndex())
	if err != nil {
		t.Fatalf("GET %s: %v", contractIndex(), err)
	}
	_ = page.Body.Close()
	instance := page.Header.Get("Datapages-Instance")

	// The context's cancel is kept under its own name. The returned closure
	// is also called cancel. Calling that one from inside itself gives a stack
	// overflow instead of a closed stream.
	ctx, closeConn := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+contractCase.streamPath, nil)
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

// TestContractDispatchReachesTheStream covers the central path of the
// framework. An action dispatches an event and the page that handles it sees
// the result on the connection it already holds.
//
// The dispatch closure, the subject, the subscription, the event loop and the
// handler call are all generated. No single piece can tell whether the next
// one is listening.
func TestContractDispatchReachesTheStream(t *testing.T) {
	if contractCase.streamPath == "" || contractCase.dispatchAction == "" {
		t.Skip("the app has no action that dispatches to a stream")
	}
	srv := contractServer(t)

	body, instance, cancel := contractOpenStream(t, srv)
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

	method, target := contractParseAction(t, contractCase.dispatchAction)
	resp := contractSendAs(
		t, srv, method, target, contractCase.dispatchBody, instance)
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

// TestContractStateIsReleased covers the end of a tab's life.
//
// The state a tab holds is kept for a grace period after its stream drops. A
// network blip then does not wipe it. After the grace period the state has to
// be released. Otherwise every tab that ever connected stays in memory.
func TestContractStateIsReleased(t *testing.T) {
	if contractCase.stateAction == "" {
		t.Skip("the app holds no per-tab state")
	}
	srv := contractServer(t)

	_, instance, cancel := contractOpenStream(t, srv)
	if instance == "" {
		cancel()
		t.Fatal("a stateful page load minted no instance id")
	}

	method, target := contractParseAction(t, contractCase.stateAction)
	resp := contractSendAs(
		t, srv, method, target, contractCase.stateActionBody, instance)
	status := resp.StatusCode
	_ = resp.Body.Close()
	if status != http.StatusOK {
		cancel()
		t.Fatalf("%s %s: status = %d while the stream is open",
			method, target, status)
	}

	cancel()
	time.Sleep(contractCase.stateGrace + 500*time.Millisecond)

	after := contractSendAs(
		t, srv, method, target, contractCase.stateActionBody, instance)
	defer func() { _ = after.Body.Close() }()
	if after.StatusCode != http.StatusConflict {
		t.Errorf("%s %s after the grace period: status = %d, want %d",
			method, target, after.StatusCode, http.StatusConflict)
	}
	if retry := after.Header.Get("Datapages-Retry"); retry != "reconnect" {
		t.Errorf("Datapages-Retry = %q, want %q", retry, "reconnect")
	}
}

// contractSendAs sends an action request as the tab named by instanceID.
func contractSendAs(
	t *testing.T, srv *httptest.Server, method, target, body, instanceID string,
) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+target, r)
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

// TestContractExternalHref covers href.External. A template uses it for a URL
// this application does not own. It hands the URL back unchanged and warns
// when it is given one the application does own. Such a URL belongs in a
// generated builder, which keeps up with the routes.
func TestContractExternalHref(t *testing.T) {
	var buf contractBuffer
	href.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { href.SetLogger(nil) })

	const external = "https://example.com/docs"
	if got := href.External(external); got != external {
		t.Errorf("External(%q) = %q, want it unchanged", external, got)
	}
	if logged := buf.String(); logged != "" {
		t.Errorf("an external URL was warned about:\n%s", logged)
	}

	const internal = "/some/app/path"
	if got := href.External(internal); got != internal {
		t.Errorf("External(%q) = %q, want it unchanged", internal, got)
	}
	if logged := buf.String(); !strings.Contains(logged, internal) {
		t.Errorf("an app-internal URL was not warned about:\n%s", logged)
	}
}

// TestContractAssetsOption covers WithAssets against what the app configured.
// An app with no assets in its datapages.yaml has no directory to serve. The
// option has to say so instead of quietly serving nothing.
func TestContractAssetsOption(t *testing.T) {
	if contractCase.hasAssets {
		t.Skip("the case configures assets and serves them in its own tests")
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = contractCase.newServer(t, datapagesgen.WithAssets(embed.FS{}))
	}()
	if recovered == nil {
		t.Fatal("WithAssets was accepted by an app that configures no assets")
	}
	if msg := fmt.Sprint(recovered); !strings.Contains(msg, "assets") {
		t.Errorf("the refusal does not say what is wrong: %v", recovered)
	}
}

// TestContractClientGoesAway covers a page load whose client disappears while
// the page is being written.
//
// Every write of the page shell can fail from that point on and the generated
// code checks each one. It must not panic, and it must not keep writing into a
// connection that is gone. One visitor closing a tab would otherwise take the
// process down.
func TestContractClientGoesAway(t *testing.T) {
	handler := contractCase.newServer(t)

	// The cut-off points walk through the shell: the opening tags, the head,
	// the body attributes, the body itself and the closing tags.
	for _, after := range []int{0, 1, 32, 128, 512, 2048} {
		t.Run(fmt.Sprint("after ", after, " bytes"), func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodGet, contractIndex(), nil)
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			w := &contractFailingWriter{header: http.Header{}, left: after}

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

// contractFailingWriter accepts left bytes and fails every write after that.
// A real ResponseWriter behaves this way once its client is gone.
type contractFailingWriter struct {
	header  http.Header
	left    int
	written int
	status  int
}

func (w *contractFailingWriter) Header() http.Header { return w.header }

func (w *contractFailingWriter) WriteHeader(status int) { w.status = status }

func (w *contractFailingWriter) Write(p []byte) (int, error) {
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
func (w *contractFailingWriter) Flush() {}

// TestContractMiddleware covers WithMiddleware: several are applied in the
// order they were given, and one of them can answer instead of passing the
// request on.
func TestContractMiddleware(t *testing.T) {
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

	srv := contractServer(t,
		datapagesgen.WithMiddleware(tag("first")),
		datapagesgen.WithMiddleware(tag("second")))
	_, _ = contractGet(t, srv, contractIndex())

	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if want := "first,second"; got != want {
		t.Errorf("middleware ran in order %q, want %q", got, want)
	}

	refusing := contractServer(t,
		datapagesgen.WithMiddleware(func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "no", http.StatusTeapot)
			})
		}))
	if resp, _ := contractGet(t, refusing, contractIndex()); resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
}

// TestContractDatastarJS covers WithDatastarJS. The page shell loads the client
// script from the source the operator chose. A deployment uses this to serve
// the script from its own origin instead of a CDN.
func TestContractDatastarJS(t *testing.T) {
	const src = "/vendor/datastar-a1b2c3.js"
	srv := contractServer(t, datapagesgen.WithDatastarJS(src))

	if _, body := contractGet(t, srv, contractIndex()); !strings.Contains(body, src) {
		t.Errorf("the page does not load the configured script:\n%s", body)
	}
}

// TestContractHTTPServerOption covers WithHTTPServer. A deployment uses it to
// set its own timeouts.
func TestContractHTTPServerOption(t *testing.T) {
	srv := contractServer(t, datapagesgen.WithHTTPServer(
		&http.Server{ReadHeaderTimeout: 3 * time.Second}))

	if resp, _ := contractGet(t, srv, contractIndex()); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestContractMessageBrokerStreamSubjects covers what the server exports for
// an operator to create streams with before it starts. An empty subject, or
// one listed twice, cannot be turned into a stream.
func TestContractMessageBrokerStreamSubjects(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range datapagesgen.MessageBrokerStreamSubjects() {
		if strings.TrimSpace(s) == "" {
			t.Error("an exported stream subject is empty")
		}
		if seen[s] {
			t.Errorf("stream subject %q is exported twice", s)
		}
		seen[s] = true
	}
}

// TestContractListenAndServe covers the lifecycle a main.go runs. The server
// listens on a port, serves, and stops when it is told to.
//
// It runs with WithLogger. The lifecycle is where every server logs something
// of its own. A configured logger that receives none of it leaves the
// deployment without a record of its server starting.
func TestContractListenAndServe(t *testing.T) {
	var buf contractBuffer
	s := contractCase.newServer(t, datapagesgen.WithLogger(
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))))
	addr := contractFreeAddr(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx, addr) }()

	client := &http.Client{Timeout: 5 * time.Second}
	contractAwaitPage(t, client, "http://"+addr+contractIndex())

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("ListenAndServe returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("ListenAndServe did not return after Shutdown")
	}

	logged := buf.String()
	for _, want := range []string{"listening", "shutdown"} {
		if !strings.Contains(logged, want) {
			t.Errorf("the configured logger received no %q line:\n%s", want, logged)
		}
	}
	if !strings.Contains(logged, addr) {
		t.Errorf("the server did not log the address it listens on:\n%s", logged)
	}
}

// TestContractListenAndServeTLS covers the same over TLS. The server runs this
// way when nothing terminates HTTPS in front of it.
func TestContractListenAndServeTLS(t *testing.T) {
	s := contractCase.newServer(t)
	certFile, keyFile := contractSelfSignedCert(t)
	addr := contractFreeAddr(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.ListenAndServeTLS(ctx, addr, certFile, keyFile) }()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	contractAwaitPage(t, client, "https://"+addr+contractIndex())

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("ListenAndServeTLS returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("ListenAndServeTLS did not return after Shutdown")
	}
}

// contractAwaitPage waits for the server to accept connections and asserts
// that what it serves is the page.
func contractAwaitPage(t *testing.T, client *http.Client, target string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("building GET %s: %v", target, err)
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := client.Do(req)
		if err == nil {
			b, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("reading %s: %v", target, readErr)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status = %d", target, resp.StatusCode)
			}
			if !strings.Contains(string(b), "<!DOCTYPE html>") {
				t.Errorf("the listening server did not serve a page:\n%s", b)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server never accepted a connection: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// contractFreeAddr reserves a port and releases it. That is the closest a test
// can get to an address it knows is free.
func contractFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("releasing the port: %v", err)
	}
	return addr
}

// contractSelfSignedCert writes a certificate and key for 127.0.0.1.
func contractSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating a certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the key: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)
	return certFile, keyFile
}

func writePEM(t *testing.T, path, kind string, der []byte) {
	t.Helper()
	b := pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// contractBuffer is a bytes.Buffer a logger and a test can both touch.
type contractBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *contractBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *contractBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
