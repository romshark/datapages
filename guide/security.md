# Security

The generated server checks request shape, cross-origin requests, CSRF tokens and stale pages. Application code must still authorize access, escape JavaScript values, limit custom request bodies and protect secrets.

## What Datapages protects

- **Request limits.** The server gives a client 5s to send the request headers and 30s for the whole request, and reads at most 1 MiB of headers. It reads at most 1 MiB of the body of an action that takes `Signals` and answers a larger one with 400 `reading signals`. `datapages.WithBodySizeLimit` changes that limit.
- **Request checks.** An action that takes `Signals`, `SSE` or `PageCacheWriter` answers 406 to a request without `Datastar-Request: true`. Every other action accepts plain form submissions and passes them through `http.CrossOriginProtection`, which answers 403 to requests from other origins.
- **CSRF.** An app with a session type checks a token on every state-changing action of a signed-in visitor. See [Use CSRF protection](#use-csrf-protection).
- **Session cookie.** A session token holds 32 random bytes. Its cookie carries `HttpOnly`, `Secure`, `SameSite=Lax` and `Path=/`, and `natskv` encrypts it.
- **Per-tab state isolation.** A tab reaches its state with an identifier of 16 random bytes in the `Datapages-Instance` header. The identifier stays out of cookies, browser storage and broker subjects.
- **Escaping.** templ escapes the values a template writes into text and attributes. The generated `href` and `action` helpers escape the path and query values of the URLs they build, and Datapages escapes the values of event subjects.
- **Stale pages.** A page of an older build cannot call the actions of a newer one. See [Deploy a new version](deployment.md#deploy-a-new-version).
- **Files.** A `datapages.File` response carries `X-Content-Type-Options: nosniff`.

## What the application protects

The application or deployment is responsible for the controls below. [SECURITY.md](../SECURITY.md) treats their absence as an application issue, not a Datapages vulnerability:

- who may see and change what, see [Authorize every request](#authorize-every-request)
- values in Datastar attributes and `templ.Raw`, see [Prevent cross-site scripting](#prevent-cross-site-scripting)
- the `Content-Security-Policy`, see [Set a Content Security Policy](#set-a-content-security-policy)
- the storage and rotation of the session key, see [Manage session keys](#manage-session-keys)
- what per-tab state and offline snapshots keep after a sign-out, see [Bind per-tab state to a session](#bind-per-tab-state-to-a-session) and [Clear offline snapshots](#clear-offline-snapshots)
- secrets next to attacker-controlled input in compressed responses, see [Prevent compression side channels](#prevent-compression-side-channels)
- request bodies that a handler reads itself, see [Limit request bodies](#limit-request-bodies)
- volumetric abuse and slow clients, see [Limit abuse](#limit-abuse)
- TLS, see [Use TLS](#use-tls)

## Use CSRF protection

Every app that declares a session type has CSRF protection. A page rendered for a session cookie contains a script that adds the `X-CSRF-Token` header to each `POST`, `PUT`, `PATCH` and `DELETE` Datastar request to the page's origin. The token is an HMAC of the session token, masked with new random bytes on every render. The server answers 403 when a request with a session cookie carries no valid token. Requests without a session cookie pass, and so do `GET`, `HEAD`, `OPTIONS` and `QUERY` requests.

For application code this means:

- Never change state in a `GETXXX` or `QUERYXXX` action. Neither is checked.
- Submit forms through Datastar actions. A plain form submission of a signed-in visitor carries no token and receives 403.
- An action without a `session` parameter checks the token against the cookie and does not read the session store. The cookie and token of a closed or expired session pass. Take `session Session` and check `IsGuest()` in each action that only signed-in visitors may call, as in [Authorize every request](#authorize-every-request).

A form that sends its fields as signals:

```templ title="app/app.templ"
<form
	data-signals:name="''"
	data-on:submit__prevent={ action.PageAccount.Save.POST() }
>
	<input data-bind="name" required/>
	<button type="submit">Save</button>
</form>
```

`datapages.WithCSRFProtection` replaces the tokens with `CSRFConfig.Tokens` or turns the protection off with `Disabled`. Turn it off only when something in front of the server rejects cross-origin requests. See [Sessions and auth](sessions.md) and [Plain Forms and CSRF](../SPECIFICATION.md#plain-forms-and-csrf).

## Authorize every request

The session tells who sends a request. Whether that visitor may see or change a record is the application's decision, in every handler that reads or changes it, which includes `GET`, actions, `GETXXX` files, `StreamOpen` and `OnXXX`. Return `datapages.ErrForbidden` or `datapages.ErrNotFound`, bare or wrapped:

```go title="app/posts.go"
// POSTDelete is /posts/{id}/delete
func (p PagePosts) POSTDelete(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	postDeleted datapages.Dispatcher[EventPostDeleted],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	post, err := p.App.posts.Get(r.Context(), path.Values.ID)
	if errors.Is(err, ErrPostNotFound) {
		return datapages.ErrNotFound
	}
	if err != nil {
		return err
	}
	if post.AuthorID != session.UserID() {
		return datapages.ErrForbidden
	}
	if err := p.App.posts.Delete(r.Context(), post.ID); err != nil {
		return err
	}
	return postDeleted.Dispatch(EventPostDeleted{})
}
```

An event handler runs in every tab that has the page open, and the visitor of each tab may differ from the one who dispatched the event. Take the session of the tab and render only what it may see:

```go title="app/posts.go"
func (p PagePosts) OnPostDeleted(
	event EventPostDeleted, sse datapages.SSE, session Session,
) error {
	posts, err := p.App.posts.VisibleTo(sse.Context(), session.UserID())
	if err != nil {
		return err
	}
	return sse.PatchElement(postList(posts))
}
```

A path value arrives decoded, `%2F` as `/` and `%2E%2E` as `..`. Look it up as a key, or open files through an `os.Root`. Never join it into a file path.

## Prevent cross-site scripting

templ escapes the values a template writes into text and attributes. Datastar attributes and `templ.Raw` need more.

The value of a Datastar attribute is a JavaScript expression. The browser decodes templ's HTML escaping before Datastar parses the expression, and an apostrophe in an interpolated value ends a string literal. What follows it runs as script. Encode such values with `templ.JSONString`, which writes a JavaScript string literal with `encoding/json`:

```templ title="app/app.templ"
templ greeting(name string) {
	<main data-signals:name={ templ.JSONString(name) }>
		<p data-text="'Hello, ' + $name"></p>
	</main>
}
```

With `data-signals:name={ "'" + name + "'" }` instead, the name `x'+alert(document.domain)+'` runs `alert`.

The generated `action` and `href` helpers escape the values they put into URLs. `action.WithBefore`, `action.WithAfter`, `action.WithPayload`, `action.WithOption` and `action.WithRequestCancellationController` take JavaScript and use it as written. Never build their arguments from request data. `action.WithHeaders` and `action.WithSelector` escape their values.

`templ.Raw` writes markup as it is. Pass it only markup that the server built, or markup from visitors after an HTML sanitizer.

Do not render HTML into a `datapages.File`, whose bytes bypass templ. Serve a file that a visitor uploaded as a download with `Disposition: datapages.FileDisposition{Download: true}`, or inline only with a type from an allowlist. A `Content-Security-Policy: sandbox` in `File.Header` adds a second layer.

Keep secrets out of signals. The browser can read every signal, and requests send them to the server.

## Set a Content Security Policy

Datapages writes inline scripts into documents:

- the build script, in the head of every document,
- the CSRF script, in each document rendered for a session cookie,
- the instance ID script, on pages with per-tab state.

Datastar also compiles every `data-*` expression at run time. Without a nonce, a policy must allow `script-src 'unsafe-inline' 'unsafe-eval'` and the origin of the Datastar bundle, `https://cdn.jsdelivr.net` unless you [self-host it](assets.md#self-host-the-datastar-bundle). A policy that blocks any of these scripts breaks the page:

- Without the build script, requests carry no build ID and pass the check, and after a deployment old pages call the actions of the new build.
- Without the CSRF script, every state-changing action of a signed-in visitor receives 403.
- Without the instance ID script, the stream of a page with per-tab state receives 409.

With `datapages.WithCSPNonce`, the policy needs neither `'unsafe-inline'` nor `'unsafe-eval'`. Datapages writes the nonce on every script it writes and on the `html` element as `data-nonce`, which turns on Datastar's CSP mode. Mint the nonce in a middleware, write it into the policy and store it in the request context:

```go title="cmd/server/main.go"
// withCSP sends a policy with a fresh nonce and stores the nonce
// for datapages.WithCSPNonce and for templ.GetNonce in templates.
func withCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := rand.Text()
		w.Header().Set("Content-Security-Policy", "default-src 'self'; "+
			"script-src 'self' 'nonce-"+nonce+"'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r.WithContext(templ.WithNonce(r.Context(), nonce)))
	})
}

func cspNonce(r *http.Request) string { return templ.GetNonce(r.Context()) }
```

```go title="cmd/server/main.go"
opts = append(opts,
	datapages.WithMiddleware(withCSP),
	datapages.WithCSPNonce(cspNonce),
)
```

`rand` is `crypto/rand`. Datapages calls `cspNonce` several times per response, and each call returns the value that the middleware stored. Write the nonce on the application's own inline scripts:

```templ title="app/app.templ"
<script nonce={ templ.GetNonce(ctx) }>
	console.log("inline script with the nonce")
</script>
```

Scripts that the application serves from its own origin pass `'self'`. `'unsafe-inline'` in `style-src` allows the `style` attributes of templates. Adjust it and the other directives to the application. Every response gets a new nonce, which is why a response that contains one must not be cached. CSP mode needs Datastar 1.0.3 or later, which matters only with `datapages.WithDatastarJS`.

The offline service worker serves a cached page without a policy header, and the hydration trigger and connectivity script of a cached page carry no nonce. A policy in a `meta` element of a cached page must not block them. See [Content Security Policy](../SPECIFICATION.md#content-security-policy).

## Manage session keys

The scaffold reads `SESSION_ENCRYPTION_KEY`, 16 bytes in hex, and passes it to `natskv`. `natskv` encrypts each session cookie with it using AES-128-GCM. The cookie holds the key of the session record in the `SESSIONS` bucket in encrypted form, which hides the user ID that the key contains. Anyone with the encryption key and read access to the bucket can build a working cookie for every session. Neither alone is enough.

- Generate a key with `openssl rand -hex 16`.
- Keep it out of the repository. `datapages init` writes it to `.env` and lists `.env` in `.gitignore`. In a deployment, pass it to the environment from the platform's secret store.
- Give every process the same key.

To rotate the key, pass the new key as `EncryptionKey` and the old ones as `PreviousEncryptionKeys`:

```go title="cmd/server/main.go"
natskv.Config{
	EncryptionKey:          newKey,
	PreviousEncryptionKeys: [][]byte{oldKey},
}
```

New cookies use the new key. The old keys only decrypt, which keeps existing cookies working. Remove an old key once every session issued under it has expired. Replacing the key without keeping the old one signs every visitor out, because their cookies no longer decrypt and the server clears them. The [complete example](deployment.md#complete-example) of the deployment page reads the old keys from `SESSION_PREVIOUS_ENCRYPTION_KEYS`.

The `inmem` session manager uses the token itself as the cookie and takes no key. A custom `sessions.Manager` decides how it protects its tokens. [`example/sqlitesessions`](../example/sqlitesessions/) stores only their hashes.

## Bind per-tab state to a session

A tab reaches its state with the identifier in its `Datapages-Instance` header, 16 random bytes from `crypto/rand`. Whoever holds the identifier can use the state. The page receives it in an inline script that removes itself, and no cookie, browser storage or DOM node keeps it.

An instance belongs to a tab, not to a session or a user, and it survives a sign-out. When a page keeps data of the signed-in visitor in its state, store the user ID with it and start over when the session of a request differs:

```go title="app/app.go"
// StateDraft holds the unsaved draft of 1 tab.
type StateDraft struct {
	UserID string // the user the other fields belong to
	Draft  string
}

// POSTSaveDraft is /draft/save
func (PageDraft) POSTSaveDraft(
	r *http.Request,
	session Session,
	state datapages.State[StateDraft],
	signals datapages.Signals[struct {
		Draft string `json:"draft"`
	}],
) error {
	if state.Values.UserID != session.UserID() {
		*state.Values = StateDraft{UserID: session.UserID()}
	}
	state.Values.Draft = signals.Values.Draft
	return nil
}
```

See [Per-tab state](state.md#security).

## Clear offline snapshots

`pageCache.Set` stores a page body in the cache of the service worker. The snapshot belongs to the origin, not to a session. It survives a sign-out, and any script on the origin can read it. Do not cache what the next visitor of the device must not see, and call `pageCache.ClearAll()` in the actions that sign visitors in and out. See [Clear the cache at sign-in and sign-out](offline.md#clear-the-cache-at-sign-in-and-sign-out).

The worker also stores the same-origin responses that elements and `fetch` load, such as an image that a `GETXXX` action serves, unless the response carries `Cache-Control: no-store`. Serve private files with `FileCache{Private: true, NoStore: true}`, which sends `Cache-Control: private, no-store`:

```go title="app/app.go"
return datapages.File{
	Type:  "image/png",
	Body:  avatar,
	Cache: datapages.FileCache{Private: true, NoStore: true},
}, nil
```

## Prevent compression side channels

A compressed response that contains a secret and input an attacker controls can reveal the secret through its size. The attacker makes the victim's browser send many requests with guessed input and watches the sizes of the responses, an attack known as BREACH. All events of 1 stream share 1 compression window, which exposes a secret sent in an earlier event to input in a later one.

Datapages masks its CSRF tokens with new random bytes on every render. Its `SameSite=Lax` session cookie leaves the requests of other sites unauthenticated, except top-level navigations. Other secrets are the application's. A page is affected when it shows a secret, such as an API key or a recovery code, together with input that a link or another user sets, such as a search term or a message.

Send such pages and their streams with `Cache-Control: private, no-transform`, which keeps them uncompressed:

```go title="cmd/server/main.go"
// noTransform keeps the account pages and their streams uncompressed.
// They show an API key next to a search term that a link can set.
func noTransform(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/account/") {
			w.Header().Set("Cache-Control", "private, no-transform")
		}
		next.ServeHTTP(w, r)
	})
}
```

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithMiddleware(noTransform))
```

Datapages appends a slash to page paths before middleware runs. The prefix `/account/` therefore covers the page `/account`, its stream `/account/_$/` and the actions below it. To keep every response uncompressed, add `datapages.WithNoResponseCompression()` and `datapages.WithNoSSECompression()`. A `StreamConfig` of the page that returns `Compression: &datapages.SSECompressionConfig{Disabled: true}` keeps only its streams uncompressed. See [Performance](performance.md).

## Limit request bodies

Datapages reads at most 1 MiB of the body of an action that takes `Signals`. A handler that reads `r.Body` itself gets no limit. Wrap the body in `http.MaxBytesReader` in a middleware, which has the `http.ResponseWriter` that `MaxBytesReader` takes:

```go title="cmd/server/main.go"
// limitImport caps the body that PUTImport reads itself.
func limitImport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/import/" {
			r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		}
		next.ServeHTTP(w, r)
	})
}
```

A read beyond the limit returns an `*http.MaxBytesError`. Answer it with 400:

```go title="app/app.go"
// PUTImport is /import
func (p PageIndex) PUTImport(r *http.Request) error {
	err := p.App.importCSV(r.Context(), r.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	return err
}
```

The `ReadTimeout` of the server, 30s by default, also limits how long a body may take. See [HTTP server](deployment.md#http-server).

## Limit abuse

Datapages meters nothing per client. Each open stream holds a connection, goroutines and a broker subscription. `datapages.StateConfig.MaxConcurrentInstances` caps the per-tab state of 1 server, 10,000 instances by default, and further streams of stateful pages receive 503 with `Retry-After`. Nothing else is counted. Limit the connections and requests of each client in the proxy or load balancer in front of the server, or cap the open streams of 1 client address in a middleware:

```go title="cmd/server/main.go"
// limitStreams caps the open streams of 1 client address.
// Behind a proxy, r.RemoteAddr is the address of the proxy.
func limitStreams(max int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	open := make(map[string]int)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "/_$/") {
				next.ServeHTTP(w, r)
				return
			}
			client, _, _ := net.SplitHostPort(r.RemoteAddr)
			mu.Lock()
			if open[client] >= max {
				mu.Unlock()
				http.Error(w, "too many streams", http.StatusTooManyRequests)
				return
			}
			open[client]++
			mu.Unlock()
			defer func() {
				mu.Lock()
				if open[client]--; open[client] == 0 {
					delete(open, client)
				}
				mu.Unlock()
			}()
			next.ServeHTTP(w, r)
		})
	}
}
```

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithMiddleware(limitStreams(20)))
```

A stream over the limit receives 429, and the page retries it after 1s and then up to 30s apart. Behind a proxy, key the map by the client address that a trusted proxy forwards, or by session.

The server has no write timeout because streams stay open. A client that stops reading keeps the goroutine and connection of its stream. `datapages.WithFileSendTimeout` ends the response of a `datapages.File` or a static asset whose writes block for 1 minute.

## Use TLS

TLS versions, ciphers, HSTS and the certificate lifecycle belong to whatever terminates TLS, also when that is `ListenAndServeTLS`. The `TLSConfig` in [HTTP server](deployment.md#http-server) sets a minimum version. When the server terminates TLS itself, send HSTS from a middleware:

```go title="cmd/server/main.go"
// hsts tells browsers to use HTTPS for this host for 2 years.
func hsts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000")
		next.ServeHTTP(w, r)
	})
}
```

`datapages.SessionsConfig` sets the attributes of the session cookie:

- `DisableSecureCookie` drops `Secure`. Set it only when the whole deployment uses plain HTTP.
- `DisableHTTPOnly` lets scripts read the token, also a script that a cross-site scripting bug injects.
- `Cookie.Domain` set to a parent domain, such as `example.com`, sends the token to every host below it.

## Report a vulnerability

Report a vulnerability through [GitHub's private vulnerability reporting](https://github.com/romshark/datapages/security/advisories/new), not in a public issue. [SECURITY.md](../SECURITY.md) lists the supported versions, the response time and the scope.

## See also

- [SECURITY.md](../SECURITY.md): the reporting process and the scope of the framework.
- [Content Security Policy](../SPECIFICATION.md#content-security-policy): the inline scripts and the rules of `WithCSPNonce`.
- [Sessions and auth](sessions.md): signing in and out, session managers and CSRF.
- [Per-tab state](state.md): the instance identifier and its lifecycle.
- [Offline and shims](offline.md): what the service worker stores.
- [Deployment](deployment.md): TLS, proxies and the HTTP server limits.
