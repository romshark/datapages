// Package actionexpr builds the Datastar action expression a generated
// action helper writes into an attribute. It holds the option vocabulary
// and the JavaScript that runs before and after the call.
//
// Application code must not import this package. It calls the generated
// action package, which forwards here.
package actionexpr

import (
	"slices"
	"strconv"
	"strings"
)

// Option is one entry of an action expression. It is either an option of
// the call or JavaScript that runs before or after it.
type Option struct {
	key   string
	value string
	kind  uint8 // 0=option, 1=before, 2=after
}

// WithOption creates an action option key-value pair.
// The key is an option name and the value is a raw JavaScript expression.
//
// WARNING: Use WithOption only when no typed helper is available.
// Typed helpers provide compile-time safety:
//   - WithContentType
//   - WithFilterSignals
//   - WithHeaders
//   - WithOpenWhenHidden
//   - WithPayload
//   - WithSelector
//   - WithRetry
//   - WithRetryInterval
//   - WithRetryScaler
//   - WithRetryMaxWaitMs
//   - WithRetryMaxCount
//   - WithRequestCancellation
//   - WithRequestCancellationController
//
// See https://data-star.dev/reference/actions#options
func WithOption(key, value string) Option {
	return Option{key: key, value: value}
}

// WithBefore prepends a JavaScript expression before the action call.
// Multiple before expressions are joined with "; " separators.
func WithBefore(expr string) Option {
	return Option{value: expr, kind: 1}
}

// WithAfter appends a JavaScript expression after the action call.
// Multiple after expressions are joined with "; " separators.
func WithAfter(expr string) Option {
	return Option{value: expr, kind: 2}
}

// ContentType is the type of content to send with an action request.
//
// See https://data-star.dev/reference/actions#options
type ContentType string

const (
	// ContentTypeJSON sends all signals in a JSON request (default).
	ContentTypeJSON ContentType = "'json'"
	// ContentTypeForm looks for the closest form to the element,
	// performs validation on form elements, and sends them as a form request.
	// No signals are sent. Use WithSelector to target a specific form.
	ContentTypeForm ContentType = "'form'"
)

// WithContentType creates an action option that controls the content type:
//   - ContentTypeJSON (default)
//   - ContentTypeForm
func WithContentType(ct ContentType) Option {
	return Option{key: "contentType", value: string(ct)}
}

// WithFilterSignals creates an action option with a regex pattern to match
// signal paths to include. If exclude is non-empty, it specifies a regex
// pattern to exclude. Defaults to include all (/.*/),
// exclude signals with a _ prefix (/(^_|\._).*/).
//
// See https://data-star.dev/reference/actions#options
func WithFilterSignals(include, exclude string) Option {
	if include == "" {
		include = ".*"
	}
	n := len("{include: /") + len(include) + len("/}")
	if exclude != "" {
		n += len(", exclude: /") + len(exclude) + len("/") // before closing }
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString("{include: /")
	b.WriteString(include)
	if exclude != "" {
		b.WriteString("/, exclude: /")
		b.WriteString(exclude)
	}
	b.WriteString("/}")
	return Option{key: "filterSignals", value: b.String()}
}

// WithHeaders creates an action option with HTTP headers to send with the request.
func WithHeaders(headers map[string]string) Option {
	if len(headers) == 0 {
		return Option{}
	}
	// Pre-calculate size assuming no escaping needed (lower bound).
	n := 2 // {}
	i := 0
	for k, v := range headers {
		if i > 0 {
			n += 2 // ", "
		}
		i++
		n += len(k) + len(v) + 6 // 'k': 'v'
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteByte('{')
	first := true
	for k, v := range headers {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString("'")
		b.WriteString(escapeJS(k))
		b.WriteString("': '")
		b.WriteString(escapeJS(v))
		b.WriteString("'")
	}
	b.WriteByte('}')
	return Option{key: "headers", value: b.String()}
}

// WithOpenWhenHidden creates an action option that controls whether to keep
// the connection open when the page is hidden. Useful for dashboards but can
// cause a drain on battery life. Defaults to false for get requests,
// and true for all other HTTP methods.
func WithOpenWhenHidden(open bool) Option {
	return Option{key: "openWhenHidden", value: strconv.FormatBool(open)}
}

// WithPayload creates an action option with a JavaScript expression
// for the request payload.
func WithPayload(expr string) Option {
	return Option{key: "payload", value: expr}
}

// WithSelector creates an action option that specifies a CSS selector for
// the form to send when ContentType is ContentTypeForm.
// If not specified, the closest form to the element is used.
func WithSelector(selector string) Option {
	return Option{key: "selector", value: "'" + escapeJS(selector) + "'"}
}

// Retry determines when to retry requests.
//
// See https://data-star.dev/reference/actions#options
type Retry string

const (
	// RetryAuto retries on network errors only (default).
	RetryAuto Retry = "'auto'"
	// RetryError retries on 4xx and 5xx responses.
	RetryError Retry = "'error'"
	// RetryAlways retries on all non-204 responses except redirects.
	RetryAlways Retry = "'always'"
	// RetryNever disables retries.
	RetryNever Retry = "'never'"
)

// WithRetry creates an action option that determines when to retry requests:
//   - RetryAuto (default)
//   - RetryError
//   - RetryAlways
//   - RetryNever
func WithRetry(r Retry) Option {
	return Option{key: "retry", value: string(r)}
}

// WithRetryInterval creates an action option for the retry interval in milliseconds.
// Defaults to 1000 (one second).
func WithRetryInterval(ms int) Option {
	return Option{key: "retryInterval", value: strconv.Itoa(ms)}
}

// WithRetryScaler creates an action option for the numeric multiplier
// applied to scale retry wait times. Defaults to 2.
func WithRetryScaler(multiplier float64) Option {
	return Option{key: "retryScaler", value: strconv.FormatFloat(multiplier, 'f', -1, 64)}
}

// WithRetryMaxWaitMs creates an action option for the maximum allowable wait time
// in milliseconds between retries. Defaults to 30000 (30 seconds).
func WithRetryMaxWaitMs(ms int) Option {
	return Option{key: "retryMaxWaitMs", value: strconv.Itoa(ms)}
}

// WithRetryMaxCount creates an action option for the maximum number
// of retry attempts. Defaults to 10.
func WithRetryMaxCount(count int) Option {
	return Option{key: "retryMaxCount", value: strconv.Itoa(count)}
}

// RequestCancellation controls request cancellation behavior.
//
// See https://data-star.dev/reference/actions#request-cancellation
type RequestCancellation string

const (
	// RequestCancellationAuto cancels existing requests on the same element (default).
	RequestCancellationAuto RequestCancellation = "'auto'"
	// RequestCancellationCleanup cancels existing requests on the same element
	// and on element or attribute cleanup.
	RequestCancellationCleanup RequestCancellation = "'cleanup'"
	// RequestCancellationDisabled allows concurrent requests.
	RequestCancellationDisabled RequestCancellation = "'disabled'"
)

// WithRequestCancellation creates an action option that controls
// request cancellation behavior:
//   - RequestCancellationAuto (default)
//   - RequestCancellationCleanup
//   - RequestCancellationDisabled
func WithRequestCancellation(rc RequestCancellation) Option {
	return Option{key: "requestCancellation", value: string(rc)}
}

// WithRequestCancellationController creates an action option that uses
// a JavaScript AbortController expression for custom request cancellation.
// The expression should reference a signal holding an AbortController instance,
// for example "$controller".
//
// See https://data-star.dev/reference/actions#request-cancellation
func WithRequestCancellationController(expr string) Option {
	return Option{key: "requestCancellation", value: expr}
}

// jsStringEscaper escapes what a JavaScript single-quoted string cannot carry.
// A line break ends such a string, which leaves the whole attribute unparsable.
var jsStringEscaper = strings.NewReplacer(
	"\\", `\\`,
	"'", `\'`,
	"\n", `\n`,
	"\r", `\r`,
)

// escapeJS escapes s for a JS single-quoted string.
func escapeJS(s string) string { return jsStringEscaper.Replace(s) }

// isEntry reports whether an option belongs in the options object.
//
// A helper given nothing to say returns the zero option.
// WithHeaders of an empty map is one, which is what a template computing its
// headers produces whenever the map comes out empty.
// Writing it would put "{: }" in the expression, which no browser can parse.
func isEntry(o Option) bool {
	return o.kind == 0 && o.key != ""
}

// WriteOptions writes the options object of an action call to b.
// It writes nothing when no option belongs in it.
func WriteOptions(b *strings.Builder, options []Option) {
	if !slices.ContainsFunc(options, isEntry) {
		return
	}
	b.WriteString(", {")
	first := true
	for _, o := range options {
		if !isEntry(o) {
			continue
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(o.key)
		b.WriteString(": ")
		b.WriteString(o.value)
	}
	b.WriteString("}")
}

// OptionsLen is the number of bytes [WriteOptions] writes for options.
func OptionsLen(options []Option) int {
	if len(options) == 0 {
		return 0
	}
	n := 0
	count := 0
	for _, o := range options {
		if !isEntry(o) {
			continue
		}
		if count > 0 {
			n += len(", ")
		}
		count++
		n += len(o.key) + len(": ") + len(o.value)
	}
	if count == 0 {
		return 0
	}
	return n + len(", {}")
}

// WriteBefore writes the JavaScript that runs before the action call.
func WriteBefore(b *strings.Builder, options []Option) {
	for _, o := range options {
		if o.kind == 1 {
			b.WriteString(o.value)
			b.WriteString("; ")
		}
	}
}

// WriteAfter writes the JavaScript that runs after the action call.
func WriteAfter(b *strings.Builder, options []Option) {
	for _, o := range options {
		if o.kind == 2 {
			b.WriteString("; ")
			b.WriteString(o.value)
		}
	}
}

// BeforeAfterLen is the number of bytes [WriteBefore] and [WriteAfter] write.
func BeforeAfterLen(options []Option) (before, after int) {
	for _, o := range options {
		switch o.kind {
		case 1:
			before += len(o.value) + len("; ")
		case 2:
			after += len("; ") + len(o.value)
		}
	}
	return
}
