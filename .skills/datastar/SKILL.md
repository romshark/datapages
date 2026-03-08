---
name: datastar
description: >-
  Use Datastar data-* HTML attributes and backend actions
  (@get, @post, @put, @delete) in templates.
  Activate when writing Templ or HTML templates that use Datastar
  for frontend reactivity, signals, SSE, or dynamic DOM updates.
---

# Datastar HTML Attribute and Action Reference

Datastar adds frontend reactivity via HTML `data-*` attributes and SSE.
Docs: https://data-star.dev

This skill covers the HTML template side only. Server-side SSE wiring is handled by Datapages. See [datapages/SKILL.md](../datapages/SKILL.md).
Templates are written in Templ. Docs: https://templ.guide/developer-tools/llm/

### IMPORTANT: Datapages Rules (ALWAYS follow these)

- **Never hardcode action URLs** (`@get('/path')`, `@post('/path')`, etc.). Always use the generated functions from the `action` package (`datapagesgen/action/`). These functions return the correct Datastar action string. Example: `action.POSTPageLoginSubmit()` returns `@post('/login/submit/')`.
- **Never hardcode href URLs for app-internal links.** Always use the generated functions from the `href` package (`datapagesgen/href/`). Example: `href.Messages(href.QueryMessages{Chat: chatID})` returns `/messages/?chat=...`. External URLs (outside the app) can be hardcoded as usual.
- **CSRF protection is handled automatically** by Datapages - never set CSRF headers manually.
- **SSE streams must NOT be opened manually** — Datapages manages all SSE stream lifecycle.
- **Use Templ expression syntax for action attributes.** In `.templ` files, use `={ expr }` (not `="..."`) for attributes that call generated action functions. Example: `data-on:click={ action.POSTPageLoginSubmit() }`, not `data-on:click="@post('/login/submit/')"`.
- **No plain HTML forms for server interaction.** CSRF protection only works with Datastar `fetch` requests. Always use Datastar actions (`@get`, `@post`, `@put`, `@patch`, `@delete`) instead of plain HTML `<form>` submissions.
- **Never install the Datastar JS file manually.** Datapages includes and serves it automatically.

Note: The examples below are from Datastar's official documentation and use hardcoded URLs for illustration purposes only. In Datapages applications, always replace hardcoded URLs with the generated `action` and `href` package functions as described above.

----

### Run Expression at Interval with data-on-interval

Source: https://data-star.dev/how_tos/poll_the_backend_at_regular_intervals

Demonstrates how to use the `data-on-interval` attribute to execute an expression, such as a GET request, at a regular interval defined by the `__duration` modifier. The interval is set to 5 seconds in this example.

```html
<div id="time"
     data-on-interval__duration.5s="@get('/endpoint')"
></div>
```

----

### Initialize Data with data-init and Show Loading State with data-indicator

Source: https://data-star.dev/reference/attributes

This example demonstrates attribute evaluation order. `data-indicator:fetching` sets a loading state, and `data-init='@get('/endpoint')'` initiates a fetch request. The indicator signal must be created before the fetch request is initialized for proper feedback.

```html
<div data-indicator:fetching data-init="@get('/endpoint')"></div>
```

----

### Datastar Event Handling with Backend Endpoint

Source: https://data-star.dev/index

This example shows how to attach a click event listener to a button using Datastar's `data-on:click` attribute. When clicked, the button triggers a GET request to the specified backend endpoint and updates a target div.

```HTML
<button data-on:click="@get('/endpoint')">
    Open the pod bay doors, HAL.
</button>

<div id="hal">Waiting for an order...</div>
```

----

### Datastar: Use Generated Action Functions for URLs

Source: https://data-star.dev/errors/fetch_no_url_provided

Never hardcode `@get()` or `@post()` URLs manually. Always use the generated URL functions from the `action` package (`datapagesgen/action/`). These functions return the correct Datastar action string with the proper URL and query parameters.

```templ
// Correct - use generated action functions:
<button data-on:click={ action.POSTPageLoginSubmit() }></button>
<button data-on:click={ action.POSTPageMessagesRead(action.QueryPOSTPageMessagesRead{MessageID: msgID}) }></button>

// WRONG - never hardcode URLs:
<button data-on:click="@post('/login/submit/')"></button>
```

----

### Submit Form Data on Submit Event (HTML)

Source: https://data-star.dev/examples/form_data

This example shows how to trigger a GET request when a form's submit event is fired. The `@get()` action is attached directly to the form using `data-on:submit`, ensuring form data is sent upon submission.

```html
<form data-on:submit="@get('/endpoint', {contentType: 'form'})">
    foo: <input type="text" name="foo" required />
    <button>
        Submit form
    </button>
</form>
```

----

### Datastar Reactivity Example: HAL 9000

Source: https://data-star.dev/docs

An example demonstrating Datastar's reactivity by using `data-signals` to set an initial message, `data-on:click` to update the message on button press, and `data-text` to display the signal's value.

```html
<div data-signals:hal="'...'">
    <button data-on:click="$hal = 'Affirmative, Dave. I read you.'">
        HAL, do you read me?
    </button>
    <div data-text="$hal"></div>
</div>
```

----

### CQRS Example with Datastar

Source: https://data-star.dev/guide/the_tao_of_datastar

Demonstrates the Command Query Responsibility Segregation (CQRS) pattern using Datastar. It shows how to initialize a component with data from a backend endpoint and trigger backend actions via click events.

```html
<div id="main" data-init="@get('/cqrs_endpoint')">
    <button data-on:click="@post('/do_something')">
        Do something
    </button>
</div>
```

----

### HTML Input for Active Search

Source: https://data-star.dev/examples/active_search

This HTML input field is configured to perform an active search. It uses data attributes for binding and debounced input events to trigger a GET request to a specified URL.

```html
<input
    type="text"
    placeholder="Search..."
    data-bind:search
    data-on:input__debounce.200ms="@get('/examples/active_search/search')"
/>
```

----

### Configure DataStar GET Request with Options

Source: https://data-star.dev/reference/actions

Demonstrates how to configure a DataStar GET request with various options such as signal filtering, custom headers, enabling requests when hidden, and disabling request cancellation. This allows for fine-grained control over network requests.

```html
<button data-on:click="@get('/endpoint', {
    filterSignals: {include: /^foo\./},
    headers: {
        'X-Csrf-Token': 'JImikTbsoCYQ9oGOcvugov0Awc5LbqFsZW6ObRCxuq',
    },
    openWhenHidden: true,
    requestCancellation: 'disabled',
})" data-datastar-element="true"></button>
```

----

### GET Request with @get()

Source: https://data-star.dev/docs

Sends a GET request to the specified URI. Supports various options for controlling request behavior, including signal filtering, background tab handling, and content type.

```APIDOC
## GET /endpoint

### Description
Sends a GET request to the specified URI. The response must contain zero or more Datastar SSE events. By default, requests include a `Datastar-Request: true` header and a `{datastar: *}` object containing existing signals (excluding those starting with an underscore) as query parameters. This behavior can be customized using the `filterSignals` option.

### Method
GET

### Endpoint
`/endpoint`

### Parameters
#### Query Parameters
- **openWhenHidden** (boolean) - Optional - If `true`, the SSE connection remains open when the page is hidden.
- **contentType** (string) - Optional - Sets the `Content-Type` header. Use `'form'` for `application/x-www-form-urlencoded`.
- **filterSignals** (object) - Optional - An object with `include` and `exclude` properties (RegExp) to filter which signals are sent.

### Request Example
```html
<button data-on:click="@get('/endpoint')"></button>
<button data-on:click="@get('/endpoint', {openWhenHidden: true})"></button>
<button data-on:click="@get('/endpoint', {contentType: 'form'})"></button>
```

### Response
#### Success Response (200)
- **SSE Events** - Description of the Server-Sent Events received.

#### Response Example
(No specific JSON response example provided, as it relies on SSE events)
```

----

### Configuring Request Cancellation

Source: https://data-star.dev/docs

Shows how to control request cancellation behavior. The first example disables automatic cancellation, allowing multiple concurrent requests. The second example demonstrates using a custom AbortController for manual request management.

```html
<!-- Allow concurrent requests (no automatic cancellation) -->
<button data-on:click="@get('/endpoint', {requestCancellation: 'disabled'})">Allow Multiple</button>

<!-- Custom abort controller for fine-grained control -->
<div data-signals:controller="new AbortController()">
    <button data-on:click="@get('/endpoint', {requestCancellation: $controller})">Start Request</button>
    <button data-on:click="$controller.abort()">Cancel Request</button>
</div>
```

----

### Send GET Request with Datastar

Source: https://data-star.dev/guide/getting_started

This snippet demonstrates how to use the `@get()` action in Datastar to send a GET request to a specified URL when an element is clicked. The response is then used to update the DOM.

```html
<button data-on:click="@get('/endpoint')">
    Open the pod bay doors, HAL.
</button>
<div id="hal"></div>
```

----

### Submit Form Data with @get and @post (HTML)

Source: https://data-star.dev/examples/form_data

This snippet demonstrates how to submit form data using both GET and POST requests. The `contentType: 'form'` option is crucial for serializing form elements. A selector can be used to target specific forms.

```html
<form id="myform">
    foo:<input type="checkbox" name="checkboxes" value="foo" />
    bar:<input type="checkbox" name="checkboxes" value="bar" />
    baz:<input type="checkbox" name="checkboxes" value="baz" />
    <button data-on:click="@get('/endpoint', {contentType: 'form'})">
        Submit GET request
    </button>
    <button data-on:click="@post('/endpoint', {contentType: 'form'})">
        Submit POST request
    </button>
</form>

<button data-on:click="@get('/endpoint', {contentType: 'form', selector: '#myform'})">
    Submit GET request from outside the form
</button>
```

----

### Trigger Backend Actions with Button Clicks

Source: https://data-star.dev/guide/backend_requests

Uses the `data-on:click` attribute to trigger a `@get()` action, sending a GET request to a specified server endpoint when a button is clicked. This is demonstrated within a div that also manages signals and computed properties.

```html
<div
    data-signals="{response: '', answer: ''}"
    data-computed:correct="$response.toLowerCase() == $answer"
>
    <div id="question"></div>
    <button data-on:click="@get('/actions/quiz')">Fetch a question</button>
    <button
        data-show="$answer != ''"
        data-on:click="$response = prompt('Answer:') ?? ''"
    >
        BUZZ
    </button>
    <div data-show="$response != ''">
        You answered “<span data-text="$response"></span>”.
        <span data-show="$correct">That is correct ✅</span>
        <span data-show="!$correct">
        The correct answer is “<span data-text="$answer"></span>” 🤷
        </span>
    </div>
</div>
```

----

### GET Request API

Source: https://data-star.dev/reference/actions

Sends a GET request to the specified URI. Handles Datastar SSE events and provides options for signal filtering, background tab behavior, and content type.

```APIDOC
## GET /endpoint

### Description
Sends a `GET` request to the backend using the Fetch API. The URI can be any valid endpoint and the response must contain zero or more Datastar SSE events.

By default, requests are sent with a `Datastar-Request: true` header, and a `{datastar: *}` object containing all existing signals, except those beginning with an underscore. This behavior can be changed using the `filterSignals` option, which allows you to include or exclude specific signals using regular expressions.

When using a `get` request, the signals are sent as a query parameter, otherwise they are sent as a JSON body.

When a page is hidden (in a background tab, for example), the default behavior for `get` requests is for the SSE connection to be closed, and reopened when the page becomes visible again. To keep the connection open when the page is hidden, set the `openWhenHidden` option to `true`.

It’s possible to send form encoded requests by setting the `contentType` option to `form`. This sends requests using `application/x-www-form-urlencoded` encoding.

It’s also possible to send requests using `multipart/form-data` encoding by specifying it in the `form` element’s `enctype` attribute. This should be used when uploading files.

### Method
GET

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **uri** (string) - Required - The endpoint URI to send the GET request to.
- **options** (object) - Optional - Configuration options for the request.
  - **filterSignals** (object) - Optional - Allows inclusion or exclusion of signals using regular expressions.
  - **openWhenHidden** (boolean) - Optional - If true, keeps the SSE connection open when the page is hidden. Defaults to false.
  - **contentType** (string) - Optional - Sets the request content type. Can be 'form' for `application/x-www-form-urlencoded` or `multipart/form-data` (when used with `enctype` attribute on form).

### Request Example
```html
<button data-on:click="@get('/endpoint')"></button>
<button data-on:click="@get('/endpoint', {openWhenHidden: true})"></button>
<button data-on:click="@get('/endpoint', {contentType: 'form'})></button>
<form enctype="multipart/form-data">
    <input type="file" name="file" />
    <button data-on:click="@get('/endpoint', {contentType: 'form'})></button>
</form>
```

### Response
#### Success Response (200)
Datastar SSE events (zero or more).

#### Response Example
(Datastar SSE event format)
```

----

### HTML with DataStar and SortableJS Integration

Source: https://data-star.dev/examples/sortable

This HTML snippet sets up a sortable list using SortableJS and integrates with DataStar for dynamic updates. It includes a display element for order information, a container for sortable buttons, and a script to initialize SortableJS and handle the 'reordered' event.

```html
<div data-signals:order-info="'Initial order'" data-text="$orderInfo"></div>
<div id="sortContainer" data-on:reordered="$orderInfo = event.detail.orderInfo">
    <button>Item 1</button>
    <button>Item 2</button>
    <button>Item 3</button>
    <button>Item 4</button>
    <button>Item 5</button>
</div>
```

----

### Initial Signal Value from Element

Source: https://data-star.dev/docs

When using `data-bind`, the initial value of the signal is set to the element's current value if the signal is not already defined. This example sets `$fooBar` to `baz`.

```html
<input data-bind:foo-bar value="baz" />
```

----

### Two-Way Data Binding with data-bind (Value Specified)

Source: https://data-star.dev/docs

This example shows an alternative way to use `data-bind` where the signal name is specified as the attribute's value. This can be useful depending on the templating context.

```html
<input data-bind="foo" />
```

----

### Customizing Request Cancellation

Source: https://data-star.dev/reference/actions

Shows how to customize DataStar's request cancellation behavior. It includes an example of disabling automatic cancellation to allow concurrent requests and another example using a custom AbortController for manual control over request cancellation.

```html
<!-- Allow concurrent requests (no automatic cancellation) -->
<button data-on:click="@get('/endpoint', {requestCancellation: 'disabled'})" data-datastar-element="true">Allow Multiple</button>

<!-- Custom abort controller for fine-grained control -->
<div data-signals:controller="new AbortController()">
    <button data-on:click="@get('/endpoint', {requestCancellation: $controller})" data-datastar-element="true">Start Request</button>
    <button data-on:click="$controller.abort()" data-datastar-element="true">Cancel Request</button>
</div>
```

----

### Toggle Multiple Nested Signals with `toggleAll` Action

Source: https://data-star.dev/docs

Provides an example of managing the state of multiple related nested signals using the `toggleAll` action. This is practical for scenarios like managing the open/closed state of multiple menus, using a regular expression to target specific signal keys.

```html
<div data-signals="{menu: {isOpen: {desktop: false, mobile: false}}}">
    <button data-on:click="@toggleAll({include: /^menu\.isOpen\./})">
        Open/close menu
    </button>
</div>
```

----

### Send GET Request with multipart/form-data in DataStar

Source: https://data-star.dev/reference/actions

Sends a GET request using `multipart/form-data` encoding, typically used for file uploads.  The `enctype` attribute of the form element must be set to `multipart/form-data`. This allows for sending files as part of the request.

```html
<form enctype="multipart/form-data">
    <input type="file" name="file" />
    <button data-on:click="@get('/endpoint', {contentType: 'form'})"></button>
</form>
```

----

### Send GET Request with form content type in DataStar

Source: https://data-star.dev/reference/actions

Sends a GET request with the `contentType` option set to 'form'.  This configures the request to use `application/x-www-form-urlencoded` encoding. This is useful for sending data in a form-encoded format.

```html
<button data-on:click="@get('/endpoint', {contentType: 'form'})"></button>
```

----

### Datastar data-computed Object Syntax Example

Source: https://data-star.dev/errors/computed_expected_function

Demonstrates the correct usage of the `data-computed` attribute with object syntax in Datastar. It requires each leaf value within the object to be a function. This example shows a computed property `bar` that depends on another computed property `foo`.

```html
<div data-computed="{foo: () => 1, bar: () => $foo + 1}"></div>
```

----

### data-on-signal-patch with Patch Variable in HTML

Source: https://data-star.dev/docs

Demonstrates using the `patch` variable within the `data-on-signal-patch` directive to access signal patch details. This example logs the patch information to the console.

```html
<div data-on-signal-patch="console.log('Signal patch:', patch)"></div>
```

----

### POST Request with @post()

Source: https://data-star.dev/docs

Sends a POST request to the specified URI. Similar to @get(), but uses the POST HTTP method.

```APIDOC
## POST /endpoint

### Description
Sends a POST request to the specified URI. This function works similarly to `@get()` but utilizes the POST HTTP method. Signals are sent as a JSON body by default.

### Method
POST

### Endpoint
`/endpoint`

### Parameters
#### Query Parameters
- **openWhenHidden** (boolean) - Optional - If `true`, the SSE connection remains open when the page is hidden.
- **contentType** (string) - Optional - Sets the `Content-Type` header. Use `'form'` for `application/x-www-form-urlencoded`.
- **filterSignals** (object) - Optional - An object with `include` and `exclude` properties (RegExp) to filter which signals are sent.

### Request Example
```html
<button data-on:click="@post('/endpoint')"></button>
```

### Response
#### Success Response (200)
- **SSE Events** - Description of the Server-Sent Events received.

#### Response Example
(No specific JSON response example provided, as it relies on SSE events)
```

----

### Handle DataStar Fetch Events in HTML

Source: https://data-star.dev/reference/actions

Listens for 'datastar-fetch' events on an HTML element. This example demonstrates logging a message to the console when a 'error' event occurs during a fetch request lifecycle.

```html
<div data-on:datastar-fetch="
    evt.detail.type === 'error' && console.log('Fetch error encountered')
"></div>
```

----

### data-on-signal-patch Directive in HTML

Source: https://data-star.dev/docs

Illustrates the `data-on-signal-patch` directive, which executes an expression whenever any signals are patched. This basic example logs a message to the console.

```html
<div data-on-signal-patch="console.log('A signal changed!')"></div>
```

----

### POST Request API

Source: https://data-star.dev/reference/actions

Sends a POST request to the specified URI. Similar to GET requests but uses the POST HTTP method.

```APIDOC
## POST /endpoint

### Description
Sends a `POST` request to the backend using the Fetch API. Works the same as `@get()` but sends a `POST` request to the backend.

### Method
POST

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **uri** (string) - Required - The endpoint URI to send the POST request to.
- **options** (object) - Optional - Configuration options for the request. (Refer to `@get()` for details on available options like `filterSignals`, `openWhenHidden`, `contentType`)

### Request Example
```html
<button data-on:click="@post('/endpoint')"></button>
```

### Response
#### Success Response (200)
(Response format depends on the backend implementation for POST requests.)

#### Response Example
(Example response body)
```

----

### HTML for Bulk Update Table

Source: https://data-star.dev/examples/bulk_update

This HTML structure defines a table for displaying user data with checkboxes for row selection. It includes a header checkbox to select all rows and individual checkboxes for each user. Buttons for 'Activate' and 'Deactivate' trigger PUT requests to server endpoints.

```html
<div
    id="demo"
    data-signals__ifmissing="{_fetching: false, selections: Array(4).fill(false)}"
>
    <table>
        <thead>
            <tr>
                <th>
                    <input
                        type="checkbox"
                        data-bind:_all
                        data-on:change="$selections = Array(4).fill($_all)"
                        data-effect="$selections; $_all = $selections.every(Boolean)"
                        data-attr:disabled="$_fetching"
                    />
                </th>
                <th>Name</th>
                <th>Email</th>
                <th>Status</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td>
                    <input
                        type="checkbox"
                        data-bind:selections
                        data-attr:disabled="$_fetching"
                    />
                </td>
                <td>Joe Smith</td>
                <td>joe@smith.org</td>
                <td>Active</td>
            </tr>
            <!-- More rows... -->
        </tbody>
    </table>
    <div role="group">
        <button
            class="success"
            data-on:click="@put('/examples/bulk_update/activate')"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
        >
            <i class="pixelarticons:user-plus"></i>
            Activate
        </button>
        <button
            class="error"
            data-on:click="@put('/examples/bulk_update/deactivate')"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
        >
            <i class="pixelarticons:user-x"></i>
            Deactivate
        </button>
    </div>
</div>
```

----

### HTML Button for Click-to-Load

Source: https://data-star.dev/examples/click_to_load

This HTML snippet represents the 'Load More' button. It includes attributes for managing the loading state and defining the click event handler that triggers data fetching. The `data-on:click` attribute specifies the action to perform when the button is clicked, preventing multiple simultaneous fetches.

```html
<button
    class="info wide"
    data-indicator:_fetching
    data-attr:aria-disabled="`${$_fetching}`"
    data-on:click="!$_fetching && @get('/examples/click_to_load/more')"
>
    Load More
</button>
```

----

### HTML File Upload Form with Fetch

Source: https://data-star.dev/examples/file_upload

This snippet shows an HTML structure for a file upload form. It includes a file input element and a submit button. The file contents are automatically encoded as base64 and submitted via fetch to the '/examples/file_upload' endpoint when the button is clicked. A size limit of 1MB is enforced, with errors logged to the console for oversized files.

```html
<label>
    <p>Pick anything less than 1MB</p>
    <input type="file" data-bind:files multiple/>
</label>
<button
    class="warning"
    data-on:click="$files.length && @post('/examples/file_upload')"
    data-attr:aria-disabled="`${!$files.length}`">
    Submit
</button>
```

----

### Initial Lazy Load HTML Structure

Source: https://data-star.dev/examples/lazy_load

This HTML snippet represents the initial state of an element before its content is loaded. It includes an ID for patching and a placeholder text indicating loading. The `data-init` attribute likely specifies the source for the content to be loaded.

```html
<div id="graph" data-init="@get('/examples/lazy_load/graph')">
    Loading...
</div>
```

----

### DELETE Request with @delete()

Source: https://data-star.dev/docs

Sends a DELETE request to the specified URI. Functionality mirrors @get() but uses the DELETE HTTP method.

```APIDOC
## DELETE /endpoint

### Description
Sends a DELETE request to the specified URI. This function works similarly to `@get()` but utilizes the DELETE HTTP method. Signals are sent as a JSON body by default.

### Method
DELETE

### Endpoint
`/endpoint`

### Parameters
#### Query Parameters
- **openWhenHidden** (boolean) - Optional - If `true`, the SSE connection remains open when the page is hidden.
- **contentType** (string) - Optional - Sets the `Content-Type` header. Use `'form'` for `application/x-www-form-urlencoded`.
- **filterSignals** (object) - Optional - An object with `include` and `exclude` properties (RegExp) to filter which signals are sent.

### Request Example
```html
<button data-on:click="@delete('/endpoint')"></button>
```

### Response
#### Success Response (200)
- **SSE Events** - Description of the Server-Sent Events received.

#### Response Example
(No specific JSON response example provided, as it relies on SSE events)
```

----

### PUT Request with @put()

Source: https://data-star.dev/docs

Sends a PUT request to the specified URI. Functionality mirrors @get() but uses the PUT HTTP method.

```APIDOC
## PUT /endpoint

### Description
Sends a PUT request to the specified URI. This function works similarly to `@get()` but utilizes the PUT HTTP method. Signals are sent as a JSON body by default.

### Method
PUT

### Endpoint
`/endpoint`

### Parameters
#### Query Parameters
- **openWhenHidden** (boolean) - Optional - If `true`, the SSE connection remains open when the page is hidden.
- **contentType** (string) - Optional - Sets the `Content-Type` header. Use `'form'` for `application/x-www-form-urlencoded`.
- **filterSignals** (object) - Optional - An object with `include` and `exclude` properties (RegExp) to filter which signals are sent.

### Request Example
```html
<button data-on:click="@put('/endpoint')"></button>
```

### Response
#### Success Response (200)
- **SSE Events** - Description of the Server-Sent Events received.

#### Response Example
(No specific JSON response example provided, as it relies on SSE events)
```

----

### PATCH Request with @patch()

Source: https://data-star.dev/docs

Sends a PATCH request to the specified URI. Functionality mirrors @get() but uses the PATCH HTTP method.

```APIDOC
## PATCH /endpoint

### Description
Sends a PATCH request to the specified URI. This function works similarly to `@get()` but utilizes the PATCH HTTP method. Signals are sent as a JSON body by default.

### Method
PATCH

### Endpoint
`/endpoint`

### Parameters
#### Query Parameters
- **openWhenHidden** (boolean) - Optional - If `true`, the SSE connection remains open when the page is hidden.
- **contentType** (string) - Optional - Sets the `Content-Type` header. Use `'form'` for `application/x-www-form-urlencoded`.
- **filterSignals** (object) - Optional - An object with `include` and `exclude` properties (RegExp) to filter which signals are sent.

### Request Example
```html
<button data-on:click="@patch('/endpoint')"></button>
```

### Response
#### Success Response (200)
- **SSE Events** - Description of the Server-Sent Events received.

#### Response Example
(No specific JSON response example provided, as it relies on SSE events)
```

----

### Sending and Nesting Signals with Datastar HTML Attributes

Source: https://data-star.dev/guide/backend_requests

Demonstrates how to use Datastar's `data-signals` and `data-bind` attributes to send and nest signals. This includes examples for basic signal assignment, object syntax for nested signals, and two-way binding. It also shows a practical use-case for managing repetitive state like menu open/closed status.

```html
<div data-signals:foo.bar="1"></div>

<div data-signals="{foo: {bar: 1}}"></div>

<input data-bind:foo.bar />

<div data-signals="{menu: {isOpen: {desktop: false, mobile: false}}}">
    <button data-on:click="@toggleAll({include: /^menu\.isOpen\./})">
        Open/close menu
    </button>
</div>
```

----

### PUT Request API

Source: https://data-star.dev/reference/actions

Sends a PUT request to the specified URI. Similar to GET requests but uses the PUT HTTP method.

```APIDOC
## PUT /endpoint

### Description
Sends a `PUT` request to the backend using the Fetch API. Works the same as `@get()` but sends a `PUT` request to the backend.

### Method
PUT

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **uri** (string) - Required - The endpoint URI to send the PUT request to.
- **options** (object) - Optional - Configuration options for the request. (Refer to `@get()` for details on available options like `filterSignals`, `openWhenHidden`, `contentType`)

### Request Example
```html
<button data-on:click="@put('/endpoint')"></button>
```

### Response
#### Success Response (200)
(Response format depends on the backend implementation for PUT requests.)

#### Response Example
(Example response body)
```

----

### DELETE Request API

Source: https://data-star.dev/reference/actions

Sends a DELETE request to the specified URI. Similar to GET requests but uses the DELETE HTTP method.

```APIDOC
## DELETE /endpoint

### Description
Sends a `DELETE` request to the backend using the Fetch API. Works the same as `@get()` but sends a `DELETE` request to the backend.

### Method
DELETE

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **uri** (string) - Required - The endpoint URI to send the DELETE request to.
- **options** (object) - Optional - Configuration options for the request. (Refer to `@get()` for details on available options like `filterSignals`, `openWhenHidden`, `contentType`)

### Request Example
```html
<button data-on:click="@delete('/endpoint')"></button>
```

### Response
#### Success Response (200)
(Response format depends on the backend implementation for DELETE requests.)

#### Response Example
(Example response body)
```

----

### PATCH Request API

Source: https://data-star.dev/reference/actions

Sends a PATCH request to the specified URI. Similar to GET requests but uses the PATCH HTTP method.

```APIDOC
## PATCH /endpoint

### Description
Sends a `PATCH` request to the backend using the Fetch API. Works the same as `@get()` but sends a `PATCH` request to the backend.

### Method
PATCH

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **uri** (string) - Required - The endpoint URI to send the PATCH request to.
- **options** (object) - Optional - Configuration options for the request. (Refer to `@get()` for details on available options like `filterSignals`, `openWhenHidden`, `contentType`)

### Request Example
```html
<button data-on:click="@patch('/endpoint')"></button>
```

### Response
#### Success Response (200)
(Response format depends on the backend implementation for PATCH requests.)

#### Response Example
(Example response body)
```

----

### HTML Structure for Templ Counter

Source: https://data-star.dev/examples/templ_counter

This HTML code defines the structure for a global and a user-specific counter. It includes buttons that trigger updates via server-sent events (SSE) when clicked. The `data-init` attribute specifies the SSE endpoint, and `data-on:click` defines the patch endpoint for updating the counters.

```html
<div data-init="@get('/examples/templ_counter/updates')">
    <!-- Global Counter -->
    <button
        id="global"
        class="info"
        data-on:click="@patch('/examples/templ_counter/global')"
    >
        Global Clicks: 0
    </button>

    <!-- User Counter -->
    <button
        id="user"
        class="success"
        data-on:click="@patch('/examples/templ_counter/user')"
    >
        User Clicks: 0
    </button>
</div>
```

----

### Predefined Signal Type Preservation (Number)

Source: https://data-star.dev/docs

When a signal is predefined, its type is preserved during binding. In this example, `$fooBar` is set to the number `10` (not the string `

----

### CQRS Pattern for Resilient SSE Updates

Source: https://data-star.dev/how_tos/prevent_sse_connections_closing

This example illustrates a Command Query Responsibility Segregation (CQRS) approach to manage SSE connections, ensuring data consistency even with interruptions. It involves initializing a main content area using `data-init` and updating it with complete state information on each event, making it resilient to connection drops. The `openWhenHidden` option can remain at its default value.

```html
<div data-init="@get('/cqrs_endpoint')"></div>
<div id="main">
    ...
</div>
```

----

### Set HTML Attributes with data-attr

Source: https://data-star.dev/docs

The `data-attr` attribute allows setting any HTML attribute to the value of a Datastar expression. This example shows setting the `aria-label` attribute dynamically.

```html
<div data-attr:aria-label="$foo"></div>
```

----

### Datastar Expression: Multiple Statements with Semicolon

Source: https://data-star.dev/guide/datastar_expressions

Shows how to execute multiple statements within a single Datastar expression by separating them with semicolons. This example updates a signal and then triggers an action.

```html
1<div data-signals:foo="1">
2    <button data-on:click="$landingGearRetracted = true; @post('/launch')">
3        Force launch
4    </button>
5</div>
```

----

### Datastar Runtime Error Logging

Source: https://data-star.dev/docs

Logs runtime errors to the browser console when data attributes are used incorrectly. Provides a link to a context-aware error page for more information and examples.

```javascript
Uncaught datastar runtime error: textKeyNotAllowed
More info: https://data-star.dev/errors/key_not_allowed?metadata=%7B%22plugin%22%3A%7B%22name%22%3A%22text%22%2C%22type%22%3A%22attribute%22%7D%2C%22element%22%3A%7B%22id%22%3A%22%22%2C%22tag%22%3A%22DIV%22%7D%2C%22expression%22%3A%7B%22rawKey%22%3A%22textFoo%22%2C%22key%22%3A%22foo%22%2C%22value%22%3A%22%22%2C%22fnContent%22%3A%22%22%7D%7D
Context: {
    "plugin": {
        "name": "text",
        "type": "attribute"
    },
    "element": {
        "id": "",
        "tag": "DIV"
    },
    "expression": {
        "rawKey": "textFoo",
        "key": "foo",
        "value": "",
        "fnContent": ""
    }
}
```

----

### Two-Way Data Binding with Attribute Casing

Source: https://data-star.dev/docs

Datastar applies attribute casing rules to signal names used with `data-bind`. These examples demonstrate how `data-bind:foo-bar` and `data-bind="fooBar"` both result in the signal `$fooBar`.

```html
<!-- Both of these create the signal `$fooBar` -->
<input data-bind:foo-bar />
<input data-bind="fooBar" />
```

----

### data-on-signal-patch-filter: Combine include and exclude filters

Source: https://data-star.dev/reference

You can combine both `include` and `exclude` properties within `data-on-signal-patch-filter` to create more specific filtering rules. This example will only trigger the listener for signals that include 'user' in their name but do not include 'password'.

```html
<!-- Combine include and exclude filters -->
<div data-on-signal-patch-filter="{include: /user/, exclude: /password/}"></div>
```

----

### HTML Button for Backend Redirect Trigger

Source: https://data-star.dev/how_tos/redirect_the_page_from_the_backend

This HTML snippet shows a button with a `data-on:click` attribute. When clicked, it triggers a `GET` request to the specified backend endpoint. An empty `div` with the ID 'indicator' is included to provide visual feedback to the user during the redirection process.

```html
<button data-on:click="@get('/endpoint')">
    Click to be redirected from the backend
</button>
<div id="indicator"></div>
```

----

### Apply Multiple Classes with data-class and Expressions

Source: https://data-star.dev/docs

The `data-class` attribute can manage multiple classes simultaneously using a set of key-value pairs. The keys are class names, and the values are expressions that determine if the class should be applied. This example applies 'success' if '$foo' is not empty and 'font-bold' if '$foo' is 'strong'.

```html
<div data-class="{success: $foo != '', 'font-bold': $foo == 'strong'}"></div>
```

----

### HTML Structure for Progressive Loading

Source: https://data-star.dev/examples/progressive_load

This HTML defines the structure for a progressively loaded page. It includes a button to trigger loading and distinct sections (header, article, comments, footer) that will be populated dynamically. The 'data-indicator:progressive-Load' attribute suggests integration with Datastar's progressive loading feature.

```html
<div>
    <div class="actions">
        <button
            id="load-button"
            data-signals:load-disabled="false"
            data-on:click="$loadDisabled=true; @get('/examples/progressive_load/updates')"
            data-attr:disabled="$loadDisabled"
            data-indicator:progressive-Load
        >
            Load
        </button>
        <!-- Indicator element -->
    </div>
    <p>
        Each part is loaded randomly and progressively.
    </p>
</div>
<div id="Load">
    <header id="header">Welcome to my blog</header>
    <section id="article">
        <h4>This is my article</h4>
        <section id="articleBody">
            <p>
                Lorem ipsum dolor sit amet...
            </p>
        </section>
    </section>
    <section id="comments">
        <h5>Comments</h5>
        <p>
            This is the comments section. It will also be progressively loaded as you scroll down.
        </p>
        <ul id="comments-list">
            <li id="1">
                <img src="https://avatar.iran.liara.run/username?username=example" alt="Avatar" class="avatar"/>
                This is a comment...
            </li>
            <!-- More comments loaded progressively -->
        </ul>
    </section>
    <div id="footer">Hope you like it</div>
</div>
```

----

### data-on-interval: Set custom interval duration

Source: https://data-star.dev/reference

This example demonstrates how to set a custom interval duration for the `data-on-interval` attribute using the `__duration` modifier. Here, the interval is set to 500 milliseconds.

```html
<div data-on-interval__duration.500ms="$count++"></div>
```

----

### Set Multiple HTML Attributes with data-attr

Source: https://data-star.dev/docs

This example illustrates using `data-attr` to set multiple HTML attributes on an element simultaneously using a key-value object. It dynamically sets `aria-label` and the `disabled` attribute.

```html
<div data-attr="{'aria-label': $foo, disabled: $bar}"></div>
```

----

### Send POST Request in DataStar

Source: https://data-star.dev/reference/actions

Sends a POST request to a specified URI, similar to `@get()`.  It uses the Fetch API to send data to the backend.  The request body and headers can be customized using options.

```html
<button data-on:click="@post('/endpoint')"></button>
```

----

### Apply ARIA Attributes with Data Attributes in HTML

Source: https://data-star.dev/guide/the_tao_of_datastar

This example demonstrates how to use custom data attributes (`data-on:click` and `data-attr`) in HTML to dynamically manage ARIA attributes. It shows toggling the `aria-expanded` state of a button and the `aria-hidden` state of a div based on a menu open/close state variable.

```html
<button data-on:click="$_menuOpen = !$_menuOpen"
        data-attr:aria-expanded="$_menuOpen ? 'true' : 'false'"
>
    Open/Close Menu
</button>
<div data-attr:aria-hidden="$_menuOpen ? 'false' : 'true'"></div>
```

----

### Send PUT Request in DataStar

Source: https://data-star.dev/reference/actions

Sends a PUT request to a specified URI, similar to `@get()`.  It uses the Fetch API to update data on the backend.  The request body and headers can be customized using options.

```html
<button data-on:click="@put('/endpoint')"></button>
```

----

### Send DELETE Request in DataStar

Source: https://data-star.dev/reference/actions

Sends a DELETE request to a specified URI, similar to `@get()`.  It uses the Fetch API to delete data on the backend.  The request body and headers can be customized using options.

```html
<button data-on:click="@delete('/endpoint')"></button>
```

----

### DataStar Runtime Error Log Example

Source: https://data-star.dev/reference/attributes

This snippet demonstrates the format of a runtime error logged to the browser console by DataStar when a data attribute is used incorrectly. It includes the error type and a URL for more detailed information.

```console
1Uncaught datastar runtime error: textKeyNotAllowed
2More info: https://data-star.dev/errors/key_not_allowed?metadata=%7B%22plugin%22%3A%7B%22name%22%3A%22text%22%2C%22type%22%3A%22attribute%22%7D%2C%22element%22%3A%7B%22id%22%3A%22%22%2C%22tag%22%3A%22DIV%22%7D%2C%22expression%22%3A%7B%22rawKey%22%3A%22textFoo%22%2C%22key%22%3A%22foo%22%2C%22value%22%3A%22%22%2C%22fnContent%22%3A%22%22%7D%7D
3Context: {
4    "plugin": {
5        "name": "text",
6        "type": "attribute"
7    },
8    "element": {
9        "id": "",
10        "tag": "DIV"
11    },
12    "expression": {
13        "rawKey": "textFoo",
14        "key": "foo",
15        "value": "",
16        "fnContent": ""
17    }
18}
```

----

### Run Expression Immediately with data-on-interval and .leading

Source: https://data-star.dev/how_tos/poll_the_backend_at_regular_intervals

Shows how to use the `.leading` modifier with `data-on-interval` to execute an expression immediately upon page load, in addition to the regular interval. This is useful for ensuring an initial data fetch or update occurs without delay.

```html
<div id="time"
     data-on-interval__duration.5s="@get('/endpoint')".leading
></div>
```

----

### Send PATCH Request in DataStar

Source: https://data-star.dev/reference/actions

Sends a PATCH request to a specified URI, similar to `@get()`.  It uses the Fetch API to partially update data on the backend.  The request body and headers can be customized using options.

```html
<button data-on:click="@patch('/endpoint')"></button>
```

----

### HTML: Display Row Before Editing

Source: https://data-star.dev/examples/edit_row

This HTML snippet represents a table row in its read-only state. It displays contact information (Name, Email) and an 'Edit' button. Clicking 'Edit' initiates the row editing process.

```html
<tr>
    <td>Joe Smith</td>
    <td>joe@smith.org</td>
    <td>
        <button data-on:click="@get('/examples/edit_row/0')">
            Edit
        </button>
    </td>
</tr>
```

----

### Color Throb Animation with CSS Transitions (HTML)

Source: https://data-star.dev/examples/animations

This example demonstrates a simple color throb animation by maintaining a stable element ID across content swaps. Datastar ensures CSS transitions can be applied between the old and new versions of the element. The provided HTML snippet shows a div with initial styles for color and background.

```html
<div
    id="color-throb"
    style="color: var(--blue-8); background-color: var(--orange-5);"
>
    blue on orange
</div>
```

----

### Create Computed Signals with data-computed

Source: https://data-star.dev/docs

The `data-computed` attribute creates a read-only signal whose value is derived from an expression. The computed signal updates automatically when its dependencies change. This example computes 'foo' based on '$bar + $baz'.

```html
<div data-computed:foo="$bar + $baz"></div>
```

----

### HTML Structure for Lazy Tabs

Source: https://data-star.dev/examples/lazy_tabs

This HTML snippet defines the structure for a tabbed interface using Datastar. It includes buttons for each tab with ARIA attributes for accessibility and data-on:click attributes to trigger tab loading. The tab content is displayed within a tabpanel element.

```html
<div id="demo">
    <div role="tablist">
        <button
            role="tab"
            aria-selected="true"
            data-on:click="@get('/examples/lazy_tabs/0')"
        >
            Tab 0
        </button>
        <button
            role="tab"
            aria-selected="false"
            data-on:click="@get('/examples/lazy_tabs/1')"
        >
            Tab 1
        </button>
        <button
            role="tab"
            aria-selected="false"
            data-on:click="@get('/examples/lazy_tabs/2')"
        >
            Tab 2
        </button>
        <!-- More tabs... -->
    </div>
    <div role="tabpanel">
        <p>Lorem ipsum dolor sit amet...</p>
        <p>Consectetur adipiscing elit...</p>
        <!-- Tab content -->
    </div>
</div>
```

----

### HTML Progress Bar with SSE Updates

Source: https://data-star.dev/examples/progress_bar

This HTML snippet defines a circular progress bar using SVG. It utilizes a data attribute to establish a Server-Sent Events connection for real-time updates. The progress bar visually represents completion and displays a restart button when finished.

```html
<div id="progress-bar"
     data-init="@get('/examples/progress_bar/updates', {openWhenHidden: true})"
>
   <svg
        width="200"
        height="200"
        viewbox="-25 -25 250 250"
        style="transform: rotate(-90deg)"
    >
        <circle
            r="90"
            cx="100"
            cy="100"
            fill="transparent"
            stroke="#e0e0e0"
            stroke-width="16px"
            stroke-dasharray="565.48px"
            stroke-dashoffset="565px"
        ></circle>
        <circle
            r="90"
            cx="100"
            cy="100"
            fill="transparent"
            stroke="#6bdba7"
            stroke-width="16px"
            stroke-linecap="round"
            stroke-dashoffset="282px"
            stroke-dasharray="565.48px"
        ></circle>
        <text
            x="44px"
            y="115px"
            fill="#6bdba7"
            font-size="52px"
            font-weight="bold"
            style="transform:rotate(90deg) translate(0px, -196px)"
        >50%</text>
    </svg>
    
    <div data-on:click="@get('/examples/progress_bar/updates', {openWhenHidden: true})">
        <!-- When progress is 100% -->
        <button>
            Completed! Try again?
        </button>
    </div>
</div>
```

----

### data-on-signal-patch-filter: Include specific signal changes

Source: https://data-star.dev/reference

The `data-on-signal-patch-filter` attribute allows you to specify which signal changes should trigger the `data-on-signal-patch` listener. This example uses the `include` property with a regular expression to only react to changes in signals named 'counter'.

```html
<!-- Only react to counter signal changes -->
<div data-on-signal-patch-filter="{include: /^counter$/}"></div>
```

----

### Loaded Lazy Load HTML Structure

Source: https://data-star.dev/examples/lazy_load

This HTML snippet shows the final state of the element after the lazy loading process is complete. The content, in this case an image, has replaced the initial loading indicator. The element is identified by its ID, which was used for patching.

```html
<div id="graph">
    <img src="/images/examples/tokyo.png" />
</div>
```

----

### HTML Structure for Web Component Binding

Source: https://data-star.dev/examples/web_component

This HTML snippet defines the structure for a web component that reverses a string. It includes an input field bound to a signal, a span to display the reversed output, and the custom web component itself, configured with event listeners and attribute bindings.

```html
<label>
    Reversed
    <input type="text" value="Your Name" data-bind:_name/>
</label>
<span data-signals:_reversed data-text="$_
eversed"></span>
<reverse-component
    data-on:reverse="$_
eversed = evt.detail.value"
    data-attr:name="$_
ame">
</reverse-component>
```

----

### data-on-signal-patch-filter: Exclude specific signal changes

Source: https://data-star.dev/reference

This example demonstrates using the `exclude` property with `data-on-signal-patch-filter` to ignore signal changes that match a given regular expression. Here, it will react to all signal changes except those whose names end with 'changes'.

```html
<!-- React to all changes except those ending with "changes" -->
<div data-on-signal-patch-filter="{exclude: /changes$/}"></div>
```

----

### Apply Casing Modifiers to data-computed Signal Names

Source: https://data-star.dev/docs

The `__case` modifier can be applied to signal names defined with `data-computed`. This allows for consistent naming conventions across your signals, regardless of how they are defined. This example applies kebab case to 'my-signal'.

```html
<div data-computed:my-signal__case.kebab="$bar + $baz"></div>
```

----

### data-on-signal-patch: Apply debounce modifier to signal patch listener

Source: https://data-star.dev/reference

This example shows how to apply a debounce modifier to the `data-on-signal-patch` attribute. The `__debounce.500ms` modifier ensures that the `doSomething()` function is called at most once every 500 milliseconds, preventing rapid re-executions.

```html
<div data-on-signal-patch__debounce.500ms="doSomething()"></div>
```

----

### Handle FetchFormNotFound with Form Selector in Datastar

Source: https://data-star.dev/errors/fetch_form_not_found

This example demonstrates how to resolve the FetchFormNotFound error in Datastar by providing a CSS selector to a form element. When the `contentType` is set to `form` and no wrapping form is present, a `selector` option must be used to specify the target form.

```html
<button data-on:click="@post('/endpoint', {contentType: 'form', selector: '#myform'})></button>
```

----

### Two-Way Data Binding with data-bind (Key Specified)

Source: https://data-star.dev/docs

The `data-bind` attribute creates a two-way data binding between an element's value and a Datastar signal. This example binds the input element's value to a signal named `foo`, specifying the signal name in the attribute key.

```html
<input data-bind:foo />
```

----

### Debounce Event Listener with Data Attributes

Source: https://data-star.dev/docs

Applies debouncing to an event listener using the `data-on-*` attribute with `__debounce` modifier. It accepts timing values like `.500ms` or `.1s`, and edge options like `.leading` or `.notrailing`. The example shows debouncing a resize event for 10ms.

```html
<div data-on-resize__debounce.10ms="$count++"></div>
```

----

### HTML Table Row Deletion with Confirmation

Source: https://data-star.dev/examples/delete_row

This snippet shows an HTML table with a delete button. The button triggers a JavaScript confirm dialog before attempting to delete the row. It utilizes custom attributes for click handling and disabling the button during an operation.

```html
<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Email</th>
            <th>Actions</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td>Joe Smith</td>
            <td>joe@smith.org</td>
            <td>
                <button
                    class="error"
                    data-on:click="confirm('Are you sure?') && @delete('/examples/delete_row/0')"
                    data-indicator:_fetching
                    data-attr:disabled="$_fetching"
                >
                    Delete
                </button>
            </td>
        </tr>
    </tbody>
</table>
```

----

### Apply Casing Modifiers to data-class Class Names

Source: https://data-star.dev/docs

Similar to `data-bind`, the `data-class` attribute supports the `__case` modifier for its class names. This allows you to automatically format class names according to specified casing rules. This example applies the camel case modifier to 'my-class'.

```html
<div data-class:my-class__case.camel="$foo"></div>
```

----

### Convert Event Name Casing with __case Modifier

Source: https://data-star.dev/reference/attributes

The `__case` modifier can be used with attributes like `data-on` to convert attribute key suffixes between different casing formats (camelCase, kebab-case, snake_case, PascalCase). This example listens for a `widgetLoaded` event by converting `widget-loaded` to camelCase.

```html
data-on:widget-loaded__case.camel
```

----

### Conditionally Apply Classes with data-class

Source: https://data-star.dev/docs

The `data-class` attribute dynamically adds or removes CSS classes from an element based on a JavaScript expression. If the expression evaluates to true, the class is added; otherwise, it's removed. This example adds 'font-bold' if '$foo' equals 'strong'.

```html
<div data-class:font-bold="$foo == 'strong'"></div>
```

----

### HTML: Display Row During Editing

Source: https://data-star.dev/examples/edit_row

This HTML snippet shows a table row in its editable state. It replaces the static data with input fields for Name and Email, and provides 'Cancel' and 'Save' buttons. The 'Cancel' button reverts the row to its read-only state, while 'Save' sends an update request.

```html
<tr>
    <td>
        <input type="text" data-bind:name>
    </td>
    <td>
        <input type="text" data-bind:email>
    </td>
    <td>
        <button data-on:click="@get('/examples/edit_row/cancel')">
            Cancel
        </button>
        <button data-on:click="@patch('/examples/edit_row/0')">
            Save
        </button>
    </td>
</tr>
```

----

### Datastar: Set Content Type to Form in Button Click

Source: https://data-star.dev/errors/fetch_invalid_content_type

This example demonstrates how to set the content type to 'form' when triggering a POST request via a button's click event in Datastar. It requires the Datastar framework to interpret the data-on attribute and the @post action.

```html
<button data-on:click="@post('/endpoint', {contentType: 'form'})></button>
```

----

### Render Current Time with data-on-interval and Backend Templating

Source: https://data-star.dev/how_tos/poll_the_backend_at_regular_intervals

Illustrates rendering the current time within an element that also uses `data-on-interval` for periodic updates. The backend templating language (e.g., `{{ now }}`) is used to display the dynamic time value, which is updated by the interval.

```html
<div id="time"
     data-on-interval__duration.5s="@get('/endpoint')"
>
     {{ now }}
</div>
```

----

### Create Computed Signals with Callable Expressions

Source: https://data-star.dev/docs

The `data-computed` attribute can also define computed signals using key-value pairs, where values are callables (like arrow functions) that return reactive values. This provides a more flexible way to define complex computed properties. This example defines 'foo' using an arrow function.

```html
<div data-computed="{foo: () => $bar + $baz}"></div>
```

----

### Use Datastar Expressions with data-text and Element Reference

Source: https://data-star.dev/reference/attributes

Datastar expressions within `data-*` attributes parse signals and support standard JavaScript syntax. The `el` variable is available in every expression, representing the element the attribute is attached to. This example displays the value of signal `$foo` concatenated with the element's ID.

```html
<div id="bar" data-text="$foo + el.id"></div>
```

----

### Integrate Web Component with Data Star

Source: https://data-star.dev/docs

Illustrates how to use a custom Web Component with Data Star. Data is passed via attributes, and results are received through custom events. This enables reusable UI components with reactive data binding. Dependencies: Data Star library, Web Components API.

```html
<div data-signals:result="''">
    <input data-bind:foo />
    <my-component
        data-attr:src="$foo"
        data-on:mycustomevent="$result = evt.detail.value"
    ></my-component>
    <span data-text="$result"></span>
</div>
```

```javascript
class MyComponent extends HTMLElement {
    static get observedAttributes() {
        return ['src'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        const value = `You entered: ${newValue}`;
        this.dispatchEvent(
            new CustomEvent('mycustomevent', {detail: {value}})
        );
    }
}

customElements.define('my-component', MyComponent);
```

----

### Use Alert Action in HTML

Source: https://data-star.dev/examples/custom_plugin

Demonstrates how to use the custom 'alert' action within an HTML element. The `@alert` syntax is used to call the action with a string argument.

```html
<button data-on:click="@alert('Hello from an action')">
    Alert using an action
</button>
```

----

### Send Nested Signals using Dot Notation

Source: https://data-star.dev/docs

Illustrates how to send nested signals to the backend using dot notation within the `data-signals` attribute. This allows for granular control and organization of frontend state sent with requests.

```html
<div data-signals:foo.bar="1"></div>
```

----

### Datastar Web Component Attributes

Source: https://data-star.dev/index

This snippet demonstrates how to use Datastar's `data-attr:*` attribute to drive web component attributes with reactive signals. It shows the HTML structure for a rocket-starfield component and the corresponding reactive variables.

```HTML
<rocket-starfield
    data-attr:center-x="$x"
    data-attr:center-y="$y"
    data-attr:speed="$speed">
</rocket-starfield>
```

----

### HTML Structure for DBMon Demo

Source: https://data-star.dev/examples/dbmon

This HTML snippet defines the structure for the DBMon demo page. It includes elements for displaying render times, input fields for mutation rate and FPS, and a table to show database cluster information. It utilizes DataStar's data attributes for dynamic behavior and event handling.

```html
<div
    id="demo"
    data-init="@get('/examples/dbmon/updates')"
    data-signals:_editing__ifmissing="false"
>
    <p>
        Average render time for entire page: { renderTime }
    </p>
    <div role="group">
        <label>
            Mutation Rate %
            <input
                type="number"
                min="0"
                max="100"
                value="20"
                data-on:focus="$_editing = true"
                data-on:blur="@put('/examples/dbmon/inputs'); $_editing = false"
                data-attr:data-bind:mutation-rate="$_editing"
                data-attr:data-bind:_mutation-rate="!$_editing"
            />
        </label>
        <label>
            FPS
            <input
                type="number"
                min="1"
                max="144"
                value="60"
                data-on:focus="$_editing = true"
                data-on:blur="@put('/examples/dbmon/inputs'); $_editing = false"
                data-attr:data-bind:fps="$_editing"
                data-attr:data-bind:_fps="!$_editing"
            />
        </label>
    </div>
    <table style="table-layout: fixed; width: 100%; word-break: break-all">
        <tbody>
            <!-- Dynamic rows generated by server -->
            <tr>
                <td>cluster1</td>
                <td style="background-color: var(--_active-color)" class="success">
                    8
                </td>
                <td aria-description="SELECT blah from something">
                    12ms
                </td>
                <!-- More query cells... -->
            </tr>
            <!-- More database rows... -->
        </tbody>
    </table>
</div>
```

----

### Import Datastar with Package Manager

Source: https://data-star.dev/guide/getting_started

Import Datastar using a package manager like npm, Deno, or Bun. This method is suitable for projects using module bundlers.

```javascript
// @ts-expect-error (only required for TypeScript projects)
import 'https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0-RC.8/bundles/datastar.js'
```

----

### Send Nested Signals using Object Syntax

Source: https://data-star.dev/docs

Shows how to send nested signals using JavaScript object literal syntax within the `data-signals` attribute. This provides a more verbose but potentially clearer way to define complex nested signal structures.

```html
<div data-signals="{foo: {bar: 1}}"></div>
```

----

### HTML Structure for Dynamic Content Loading

Source: https://data-star.dev/how_tos/load_more_list_items

Defines the HTML structure for a list container and a button to trigger loading more items. It utilizes data attributes for initial state and event binding, allowing Datastar to manage dynamic updates.

```html
1<div id="list">
2<div>Item 1</div>
3</div>
4<button id="load-more" 
5        data-signals:offset="1" 
6        data-on:click="@get('/how_tos/load_more/data')">
7Click to load another item
8</button>
```

----

### HTML Structure for Bad Apple Benchmark

Source: https://data-star.dev/examples/bad_apple

This HTML snippet sets up the structure for the Bad Apple benchmark. It uses Datastar signals to manage the percentage display and the content of the pre tag, which will be updated with video frames. The range input visually represents the playback progress.

```html
<label
    data-signals="{_percentage: 0, _contents: 'bad apple frames go here'}"
    data-init="@get('/examples/bad_apple/updates')"
>
    <span data-text="`Percentage: ${$_percentage.toFixed(2)}%`"></span>
    <input
        type="range"
        min="0"
        max="100"
        step="0.01"
        disabled
        style="cursor: default"
        data-attr:value="$_percentage"
    />
</label>
<pre style="line-height: 100%" data-text="$_contents"></pre>
```

----

### Two-Way Binding for Nested Signals

Source: https://data-star.dev/docs

Demonstrates using two-way data binding with `data-bind` on an input element to directly link its value to a nested signal. Changes in the input update the signal, and vice-versa, facilitating real-time state synchronization.

```html
<input data-bind:foo.bar />
```

----

### Display Contact Details for Click to Edit

Source: https://data-star.dev/examples/click_to_edit

This HTML snippet displays contact information (First Name, Last Name, Email) and provides 'Edit' and 'Reset' buttons. The 'Edit' button triggers fetching the editing UI from '/examples/click_to_edit/edit', while 'Reset' calls '/examples/click_to_edit/reset'. It uses data attributes for dynamic behavior.

```html
<div id="demo">
    <p>First Name: John</p>
    <p>Last Name: Doe</p>
    <p>Email: joe@blow.com</p>
    <div role="group">
        <button
            class="info"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
            data-on:click="@get('/examples/click_to_edit/edit')"
        >
            Edit
        </button>
        <button
            class="warning"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
            data-on:click="@patch('/examples/click_to_edit/reset')"
        >
            Reset
        </button>
    </div>
</div>
```

----

### DataStar HTML for Backend Script Execution

Source: https://data-star.dev/guide/datastar_expressions

Shows how to trigger a backend request using a `data-on:click` attribute on a button. The backend response, if `text/javascript`, will be executed in the browser.

```html
<button data-on:click="@get('/endpoint')">
    What are you talking about, HAL?
</button>
```

----

### DataStar Fetch Events

Source: https://data-star.dev/reference/actions

Explains the different events triggered during the DataStar fetch request lifecycle.

```APIDOC
## DataStar Fetch Events

### Description
All actions within DataStar that involve fetching data trigger `datastar-fetch` events. These events allow you to hook into different stages of the fetch request lifecycle.

### Event Types
- **`started`**: Triggered when the fetch request begins.
- **`finished`**: Triggered when the fetch request completes successfully.
- **`error`**: Triggered if the fetch request encounters an error.
- **`retrying`**: Triggered when a fetch request is being retried due to a transient error.
- **`retries-failed`**: Triggered when all retry attempts for a fetch request have failed.

### Example Usage
```html
<div data-on:datastar-fetch="
    evt.detail.type === 'error' && console.log('Fetch error encountered')
"></div>
```
```

----

### Implement Alert Action Plugin

Source: https://data-star.dev/examples/custom_plugin

Defines a custom 'alert' action plugin. This plugin takes a context and a value, then displays the value using the browser's alert function. It's registered with the name 'alert'.

```javascript
action({
    name: 'alert',
    apply(ctx, value) {
        alert(value)
    }
})
```

----

### HTML Structure with Signal Patching

Source: https://data-star.dev/examples/on_signal_patch

This HTML code demonstrates the 'On Signal Patch' plugin's functionality. It includes buttons to update message and counter, clear changes, and displays current values. It also sets up listeners for signal patches, filtering them for specific signals like 'counter' or excluding others.

```html
<div data-signals="{counter: 0, message: 'Hello World', allChanges: [], counterChanges: []}">
    <div class="actions">
        <button data-on:click="$message = `Updated: ${performance.now().toFixed(2)}`">
            Update Message
        </button>
        <button data-on:click="$counter++">
            Increment Counter
        </button>
        <button
            class="error"
            data-on:click="$allChanges.length = 0; $counterChanges.length = 0"
        >
            Clear All Changes
        </button>
    </div>
    <div>
        <h3>Current Values</h3>
        <p>Counter: <span data-text="$counter"></span></p>
        <p>Message: <span data-text="$message"></span></p>
    </div>
    <div
        data-on-signal-patch="$counterChanges.push(patch)"
        data-on-signal-patch-filter="{include: /^counter$/}"
    >
        <h3>Counter Changes Only</h3>
        <pre data-json-signals__terse="{include: /^counterChanges/}"></pre>
    </div>
    <div
        data-on-signal-patch="$allChanges.push(patch)"
        data-on-signal-patch-filter="{exclude: /allChanges|counterChanges/}"
    >
        <h3>All Signal Changes</h3>
        <pre data-json-signals__terse="{include: /^allChanges/}"></pre>
    </div>
</div>
```

----

### Declarative Signals and Event Handling in HTML

Source: https://data-star.dev/guide/reactive_signals

This HTML snippet demonstrates how to use Datastar's data attributes to create declarative signals and handle user events. It sets up a signal named 'hal' and updates it when a button is clicked, then displays the signal's value.

```html
<div data-signals:hal="'...'" >
    <button data-on:click="$hal = 'Affirmative, Dave. I read you.'">
        HAL, do you read me?
    </button>
    <div data-text="$hal"></div>
</div>
```

----

### Listen for Any Keydown Event Globally

Source: https://data-star.dev/how_tos/bind_keydown_events_to_specific_keys

Attaches a keydown event listener to the window that triggers an alert for any key press. It utilizes the `data-on:keydown__window` attribute for global event binding.

```html
<div data-on:keydown__window="alert('Key pressed')"></div>
```

----

### Datastar Loading Indicator with data-indicator

Source: https://data-star.dev/guide/the_tao_of_datastar

Illustrates how to use the `data-indicator` attribute in Datastar to display a loading state on an element while a backend request is in progress. The loading text is conditionally shown using `$ _loading`.

```html
<div>
    <button data-indicator:_loading
            data-on:click="@post('/do_something')"
    >
        Do something
        <span data-show="$_loading">Loading...</span>
    </button>
</div>
```

----

### Send POST Request with Datastar

Source: https://data-star.dev/guide/backend_requests

Demonstrates how to send a POST request to the server using Datastar's `@post()` backend action. This is typically used for submitting data, such as quiz answers, for processing.

```html
<button data-on:click="@post('/actions/quiz')">
    Submit answer
</button>
```

----

### HTML Structure for Patching Signals

Source: https://data-star.dev/guide/reactive_signals

This HTML snippet demonstrates the basic structure for interacting with backend-driven signals. It includes a button to trigger a backend request and a div to display the signal content, utilizing DataStar's `data-signals` and `data-on:click` attributes.

```html
<div data-signals:hal="'...'" >
    <button data-on:click="@get('/endpoint')">
        HAL, do you read me?
    </button>
    <div data-text="$hal"></div>
</div>
```

----

### Request Options

Source: https://data-star.dev/docs

Details the available options for customizing requests, such as contentType, filterSignals, selector, headers, openWhenHidden, payload, retry, and requestCancellation.

```APIDOC
## Request Options

All actions accept an `options` object as a second argument to customize request behavior.

### Options

- **`contentType`** (string) – The type of content to send. Options are `json` (default) or `form`. `form` validates and sends the closest form's data.
- **`filterSignals`** (object) – An object to filter signals. 
  - **`include`** (RegExp) – Regular expression to match signal paths to include. Defaults to `/.*/`.
  - **`exclude`** (RegExp) – Regular expression to exclude specific signal paths. Defaults to `/(^_|._).*/`.
- **`selector`** (string | null) – Specifies a form to send when `contentType` is `form`. Defaults to `null` (uses the closest form).
- **`headers`** (object) – An object containing custom headers for the request.
- **`openWhenHidden`** (boolean) – Whether to keep the connection open when the page is hidden. Defaults to `false` for `get` requests, `true` for others.
- **`payload`** (object) – Allows overriding the fetch payload with a custom object.
- **`retry`** (string) – Determines when to retry requests. Options: `'auto'` (default), `'error'`, `'always'`, `'never'`.
- **`retryInterval`** (number) – The retry interval in milliseconds. Defaults to `1000`.
- **`retryScaler`** (number) – Multiplier for scaling retry wait times. Defaults to `2`.
- **`retryMaxWaitMs`** (number) – Maximum wait time in milliseconds between retries. Defaults to `30000`.
- **`retryMaxCount`** (number) – Maximum number of retry attempts. Defaults to `10`.
- **`requestCancellation`** (string | AbortController) – Controls request cancellation. Options: `'auto'` (default), `'disabled'`, or an `AbortController` instance.

### Example Usage

```html
<button data-on:click="@get('/endpoint', {
    filterSignals: {include: /^foo\./},
    headers: {
        'X-Csr f-Token': 'JImikTbsoCYQ9oGOcvugov0Awc5LbqFsZW6ObRCxuq',
    },
    openWhenHidden: true,
    requestCancellation: 'disabled',
})" >Click Me</button>
```
```

----

### Editable Form for Click to Edit

Source: https://data-star.dev/examples/click_to_edit

This HTML snippet presents an editable form for contact details (First Name, Last Name, Email) with input fields. It includes 'Save' and 'Cancel' buttons. 'Save' submits changes via a PUT request to '/examples/click_to_edit', and 'Cancel' reverts to the view state by calling '/examples/click_to_edit/cancel'. Input fields are bound to signals and disabled during fetching.

```html
<div id="demo">
    <label>
        First Name
        <input
            type="text"
            data-bind:first-name
            data-attr:disabled="$_fetching"
        >
    </label>
    <label>
        Last Name
        <input
            type="text"
            data-bind:last-name
            data-attr:disabled="$_fetching"
        >
    </label>
    <label>
        Email
        <input
            type="email"
            data-bind:email
            data-attr:disabled="$_fetching"
        >
    </label>
    <div role="group">
        <button
            class="success"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
            data-on:click="@put('/examples/click_to_edit')"
        >
            Save
        </button>
        <button
            class="error"
            data-indicator:_fetching
            data-attr:disabled="$_fetching"
            data-on:click="@get('/examples/click_to_edit/cancel')"
        >
            Cancel
        </button>
    </div>
</div>
```

----

### Call Synchronous External Script Function in Data Star

Source: https://data-star.dev/docs

Demonstrates how to bind an input's value to a JavaScript function's argument and display its return value. The function is called synchronously on input change. Dependencies: Data Star library.

```html
<div data-signals:result>
    <input data-bind:foo 
        data-on:input="$result = myfunction($foo)"
    >
    <span data-text="$result"></span>
</div>
```

```javascript
function myfunction(data) {
    return `You entered: ${data}`;
}
```

----

### Use Alert Attribute in HTML

Source: https://data-star.dev/examples/custom_plugin

Demonstrates how to use the custom 'alert' attribute within an HTML element. The `data-alert` syntax is used, and it expects a string value to be alerted when the element is clicked.

```html
<button data-alert="'Hello from an attribute'">
    Alert using an attribute
</button>
```

----

### Event Listener Modifiers in HTML

Source: https://data-star.dev/docs

Demonstrates the use of various modifiers for event listeners in HTML. Modifiers like `__window`, `__debounce`, and `__case` can be chained to alter event behavior and data casing.

```html
<button data-on:click__window__debounce.500ms.leading="$foo = ''"></button>
<div data-on:my-event__case.camel="$foo = ''"></div>
```

----

### HTML Structure for Event Bubbling Demo

Source: https://data-star.dev/examples/event_bubbling

This HTML sets up a container with several buttons. A `data-on:click` attribute on the parent div handles click events, delegating the action to the closest button with a `data-id`. The `span` element displays the last pressed key.

```html
<div id="demo">
  Key pressed: <span data-text="$key"></span>
  <div id="event-bubbling-container" data-on:click="$key = evt.target.closest('button[data-id]')?.dataset.id ?? $key">
    <button data-id="KEY ELSE" class="gray">KEY<br/>ELSE</button>
    <button data-id="CM">CM</button>
    <button data-id="OM">OM</button>
    <button data-id="FETCH">FETCH</button>
    <button data-id="SET">SET</button>
    <button data-id="EXEC">EXEC</button>
    <button data-id="TEST ALARM" class="gray">TEST<br/>ALARM</button>
    <button data-id="3">3</button>
    <button data-id="2">2</button>
    <button data-id="1">1</button>
    <button data-id="ENTER">ENTER</button>
    <button data-id="CLEAR">CLEAR</button>
  </div>
</div>
```

----

### Configure Content Security Policy for Datastar

Source: https://data-star.dev/reference/security

This snippet demonstrates how to configure a Content Security Policy (CSP) to allow Datastar's expression evaluation. It specifically enables the 'unsafe-eval' directive, which is required because Datastar uses the Function() constructor for evaluating expressions. Ensure this is used in conjunction with other security best practices.

```html
<meta http-equiv="Content-Security-Policy" 
    content="script-src 'self' 'unsafe-eval';"
>
```

----

### Default Request Cancellation Behavior

Source: https://data-star.dev/docs

Illustrates the default behavior of Data-Star where clicking a button multiple times will cancel previous requests on the same element. This is useful for preventing race conditions with slow backend responses.

```html
<!-- Clicking this button multiple times will cancel previous requests (default behavior) -->
<button data-on:click="@get('/slow-endpoint')">Load Data</button>
```

----

### Nesting Signals

Source: https://data-star.dev/guide/backend_requests

Signals can be nested using dot-notation or object syntax for more granular targeting on the backend. This is particularly useful for managing repetitive state, like the open/closed status of multiple menus.

```APIDOC
## Nesting Signals

### Description
Signals can be nested to allow for more granular targeting on the backend. This can be achieved using dot-notation directly in the `data-signals` attribute or by using an object syntax. Two-way binding also supports nested signals.

### Method
N/A (This describes signal structure, not an endpoint)

### Endpoint
N/A

### Parameters
N/A

### Request Example
**Using dot-notation:**
```html
<div data-signals:foo.bar="1"></div>
```

**Using object syntax:**
```html
<div data-signals="{foo: {bar: 1}}"></div>
```

**Using two-way binding:**
```html
<input data-bind:foo.bar />
```

**Practical Use-Case Example:**
```html
<div data-signals="{menu: {isOpen: {desktop: false, mobile: false}}}">
    <button data-on:click="@toggleAll({include: /^menu\.isOpen\./})">
        Open/close menu
    </button>
</div>
```

### Response
N/A
```

----

### Datastar Fetch Events

Source: https://data-star.dev/docs

Describes the lifecycle events triggered during a datastar-fetch request.

```APIDOC
## Datastar Fetch Events

All of the actions above trigger `datastar-fetch` events during the fetch request lifecycle. The event type determines the stage of the request.

- `started` – Triggered when the fetch request is started.
- `finished` – Triggered when the fetch request is finished.
- `error` – Triggered when the fetch request encounters an error.
- `retrying` – Triggered when the fetch request is retrying.
- `retries-failed` – Triggered when all fetch retries have failed.

```html
<div data-on:datastar-fetch="
    evt.detail.type === 'error' && console.log('Fetch error encountered')
"></div>
```
```

----

### Initialize Expression with data-init

Source: https://data-star.dev/reference/attributes

The `data-init` attribute executes an expression when the element is initialized in the DOM. This includes page load, DOM patching, and attribute modifications. Modifiers like `__delay` and `__viewtransition` can alter its behavior.

```html
<div data-init="$count = 1"></div>
<div data-init__delay.500ms="$count = 1"></div>
```

----

### HTML Structure for TodoMVC

Source: https://data-star.dev/examples/todomvc

The main HTML structure for the TodoMVC application. It includes input fields for adding new todos, a list to display them, and buttons for filtering and actions like deleting and resetting. It utilizes Datastar's data attributes for event handling and data binding.

```html
<section
    id="todomvc"
    data-init="@get('/examples/todomvc/updates')"
>
    <header id="todo-header">
        <input
            type="checkbox"
            data-on:click__prevent="@post('/examples/todomvc/-1/toggle')"
            data-init="el.checked = false"
        />
        <input
            id="new-todo"
            type="text"
            placeholder="What needs to be done?"
            data-signals:input
            data-bind:input
            data-on:keydown="
                evt.key === 'Enter' && $input.trim() && @patch('/examples/todomvc/-1') && ($input = '');
            "
        />
    </header>
    <ul id="todo-list">
        <!-- Todo items are dynamically rendered here -->
    </ul>
    <div id="todo-actions">
        <span>
            <strong>0</strong> items pending
        </span>
        <button class="small info" data-on:click="@put('/examples/todomvc/mode/0')">
            All
        </button>
        <button class="small" data-on:click="@put('/examples/todomvc/mode/1')">
            Pending
        </button>
        <button class="small" data-on:click="@put('/examples/todomvc/mode/2')">
            Completed
        </button>
        <button class="error small" aria-disabled="true">
            Delete
        </button>
        <button class="warning small" data-on:click="@put('/examples/todomvc/reset')">
            Reset
        </button>
    </div>
</section>
```

----

### Listen for 'Enter' Keydown Event Globally

Source: https://data-star.dev/how_tos/bind_keydown_events_to_specific_keys

Attaches a keydown event listener to the window that triggers an alert only when the 'Enter' key is pressed. It uses `evt.key === 'Enter'` to check the specific key.

```html
<div data-on:keydown__window="evt.key === 'Enter' && alert('Key pressed')"></div>
```

----

### Datastar Action Options

Source: https://data-star.dev/reference/actions

This section details the available options that can be passed to Datastar actions, such as contentType, filterSignals, selector, headers, openWhenHidden, payload, retry, and requestCancellation.

```APIDOC
## Options

All of the actions above take a second argument of options.

* `contentType` – The type of content to send. A value of `json` sends all signals in a JSON request. A value of `form` tells the action to look for the closest form to the element on which it is placed (unless a `selector` option is provided), perform validation on the form elements, and send them to the backend using a form request (no signals are sent). Defaults to `json`.
* `filterSignals` – A filter object with an `include` property that accepts a regular expression to match signal paths (defaults to all signals: `/.*/`), and an optional `exclude` property to exclude specific signal paths (defaults to all signals that do not have a `_` prefix: `/(^_|._).*/`).
* `selector` – Optionally specifies a form to send when the `contentType` option is set to `form`. If the value is `null`, the closest form is used. Defaults to `null`.
* `headers` – An object containing headers to send with the request.
* `openWhenHidden` – Whether to keep the connection open when the page is hidden. Useful for dashboards but can cause a drain on battery life and other resources when enabled. Defaults to `false` for `get` requests, and `true` for all other HTTP methods.
* `payload` – Allows the fetch payload to be overridden with a custom object.
* `retry` – Determines when to retry requests. Can be `'auto'` (default, retries on network errors only), `'error'` (retries on `4xx` and `5xx` responses), `'always'` (retries on all non-`204` responses except redirects), or `'never'` (disables retries). Defaults to `'auto'`.
* `retryInterval` – The retry interval in milliseconds. Defaults to `1000` (one second).
* `retryScaler` – A numeric multiplier applied to scale retry wait times. Defaults to `2`.
* `retryMaxWaitMs` – The maximum allowable wait time in milliseconds between retries. Defaults to `30000` (30 seconds).
* `retryMaxCount` – The maximum number of retry attempts. Defaults to `10`.
* `requestCancellation` – Controls request cancellation behavior. Can be `'auto'` (default, cancels existing requests on the same element), `'disabled'` (allows concurrent requests), or an `AbortController` instance for custom control. Defaults to `'auto'`.

### Request Example

```html
<button data-on:click="@get('/endpoint', {
    filterSignals: {include: /^foo\./},
    headers: {
        'X-Csrf-Token': 'JImikTbsoCYQ9oGOcvugov0Awc5LbqFsZW6ObRCxuq',
    },
    openWhenHidden: true,
    requestCancellation: 'disabled',
})"></button>
```
```

----

### Listen for 'Enter' or 'Ctrl + L' Keydown Event Globally

Source: https://data-star.dev/how_tos/bind_keydown_events_to_specific_keys

Attaches a keydown event listener to the window that triggers an alert for either the 'Enter' key or the 'Ctrl + L' combination. It combines conditions using logical OR (`||`) and AND (`&&`).

```html
<div data-on:keydown__window="(evt.key === 'Enter' || (evt.ctrlKey && evt.key === 'l')) && alert('Key pressed')"></div>
```

----

### DRY Datastar Actions with Templating (Loop)

Source: https://data-star.dev/how_tos/keep_datastar_code_dry

Illustrates using a templating language's loop construct to generate multiple elements that perform the same Datastar action. This is particularly useful when dealing with lists of items.

```html
{% set labels = ['Click me', 'No, click me!', 'Click us all!'] %}
{% for label in labels %}
    <button data-on:click="@get('/endpoint')">{{ label }}</button>
{% endfor %}
```

----

### Implement Alert Attribute Plugin

Source: https://data-star.dev/examples/custom_plugin

Defines a custom 'alert' attribute plugin. This plugin listens for click events on an element, and when clicked, it alerts the value returned from an expression. It requires a value and returns a value.

```javascript
attribute({
    name: 'alert',
    requirement: {
        key: 'denied',
        value: 'must',
    },
    returnsValue: true,
    apply({ el, rx }) {
        const callback = () => alert(rx())
        el.addEventListener('click', callback)
        return () => el.removeEventListener('click', callback)
    }
})
```

----

### Request Cancellation Behavior

Source: https://data-star.dev/docs

Explains the default request cancellation behavior and how to control it using the `requestCancellation` option.

```APIDOC
## Request Cancellation Behavior

By default, initiating a new fetch request on an element automatically cancels any existing request on that same element. This prevents concurrent requests from interfering.

### Default Behavior Example

```html
<!-- Clicking this button multiple times will cancel previous requests (default behavior) -->
<button data-on:click="@get('/slow-endpoint')">Load Data</button>
```

This cancellation is element-level; requests on different elements can run concurrently.

### Controlling Cancellation

Use the `requestCancellation` option to modify this behavior:

- **`'disabled'`**: Allows concurrent requests on the same element.
  ```html
  <!-- Allow concurrent requests (no automatic cancellation) -->
  <button data-on:click="@get('/endpoint', {requestCancellation: 'disabled'})">Allow Multiple</button>
  ```

- **`AbortController` instance**: Provides fine-grained control over cancellation.
  ```html
  <div data-signals:controller="new AbortController()">
      <button data-on:click="@get('/endpoint', {requestCancellation: $controller})">Start Request</button>
      <button data-on:click="$controller.abort()">Cancel Request</button>
  </div>
  ```
```

----

### Efficient Datastar Event Handling with Event Bubbling

Source: https://data-star.dev/how_tos/keep_datastar_code_dry

Demonstrates how to use event bubbling by attaching a single event listener to a parent element. This listener checks the target element's tag name before executing the Datastar action, reducing the number of event listeners.

```html
<div data-on:click="evt.target.tagName == 'BUTTON' 
    && @get('/endpoint')">
    <button>Click me</button>
    <button>No, click me!</button>
    <button>Click us all!</button>
</div>
```

----

### Repeating Datastar Actions with HTML

Source: https://data-star.dev/how_tos/keep_datastar_code_dry

Demonstrates a common scenario where the same Datastar action is repeated across multiple HTML elements. This repetition can lead to verbose and hard-to-maintain code.

```html
<button data-on:click="@get('/endpoint')">Click me</button>
<button data-on:click="@get('/endpoint')">No, click me!</button>
<button data-on:click="@get('/endpoint')">Click us all!</button>
```

----

### Display Loading Indicator During Requests

Source: https://data-star.dev/guide/backend_requests

The `data-indicator` attribute manages a boolean signal that is true while a request is in flight and false otherwise. This signal can be used to conditionally display loading indicators, improving user experience for slower operations.

```html
<div id="question"></div>
<button
    data-on:click="@get('/actions/quiz')"
    data-indicator:fetching
>
    Fetch a question
</button>
<div data-class:loading="$fetching" class="indicator"></div>
```

----

### Datastar Expression: Conditional Request (Logical AND)

Source: https://data-star.dev/docs

Uses the logical AND operator `&&` within a `data-on:click` attribute to conditionally trigger an HTTP POST request. The `@post('/launch')` action will only execute if the `$landingGearRetracted` signal is truthy.

```html
<button data-on:click="$landingGearRetracted && @post('/launch')">
    Launch
</button>
```

----

### File Uploads with data-bind

Source: https://data-star.dev/reference/attributes

Input fields of type `file` with `data-bind` automatically encode file contents in base64, enabling signal updates without a form. The resulting signal is an array of objects containing file name, contents, and MIME type.

```html
<input type="file" data-bind:files multiple />
```

----

### Request Cancellation Behavior

Source: https://data-star.dev/reference/actions

Explains the default request cancellation behavior where new requests on an element cancel existing ones, and how to control this using the `requestCancellation` option.

```APIDOC
## Request Cancellation

By default, when a new fetch request is initiated on an element, any existing request on that same element is automatically cancelled. This prevents multiple concurrent requests from conflicting with each other and ensures clean state management.

For example, if a user rapidly clicks a button that triggers a backend action, only the most recent request will be processed:

```html
<!-- Clicking this button multiple times will cancel previous requests (default behavior) -->
<button data-on:click="@get('/slow-endpoint')">Load Data</button>
```

This automatic cancellation happens at the element level, meaning requests on different elements can run concurrently without interfering with each other.

You can control this behavior using the `requestCancellation` option:

```html
<!-- Allow concurrent requests (no automatic cancellation) -->
<button data-on:click="@get('/endpoint', {requestCancellation: 'disabled'})">Allow Multiple</button>

<!-- Custom abort controller for fine-grained control -->
<div data-signals:controller="new AbortController()">
    <button data-on:click="@get('/endpoint', {requestCancellation: $controller})">Start Request</button>
    <button data-on:click="$controller.abort()">Cancel Request</button>
</div>
```
```

----

### DRY Datastar Actions with Templating (Set Variable)

Source: https://data-star.dev/how_tos/keep_datastar_code_dry

Shows how to use a templating language's 'set' directive to define a reusable action string. This avoids repeating the same Datastar call for multiple elements, improving maintainability.

```html
{% set action = "@get('/endpoint')" %}
<button data-on:click="{{ action }}">Click me</button>
<button data-on:click="{{ action }}">No, click me!</button>
<button data-on:click="{{ action }}">Click us all!</button>
```

----

### DataStar HTML for Web Component Integration

Source: https://data-star.dev/guide/datastar_expressions

Demonstrates how to use DataStar attributes to bind data to a custom web component and handle its events. It shows attribute binding (`data-attr`), event handling (`data-on`), and text content binding (`data-text`).

```html
<div data-signals:result="''">
    <input data-bind:foo />
    <my-component
        data-attr:src="$foo"
        data-on:mycustomevent="$result = evt.detail.value"
    ></my-component>
    <span data-text="$result"></span>
</div>
```

----

### data-on-interval

Source: https://data-star.dev/reference

Runs an expression at a regular interval, with customizable duration and modifiers.

```APIDOC
## data-on-interval

### Description
Runs an expression at a regular interval. The interval duration defaults to one second and can be modified using the `__duration` modifier.

### Modifiers
- `__duration` (e.g., `.500ms`, `.1s`): Sets the interval duration. Defaults to 1 second.
- `.leading`: Execute the first interval immediately.
- `__viewtransition`: Wraps the expression in `document.startViewTransition()` if available.

### Example
```html
<div data-on-interval="$count++"></div>
<div data-on-interval__duration.500ms="$count++"></div>
```
```

----

### Create Loading Indicators with data-indicator

Source: https://data-star.dev/reference/attributes

The `data-indicator` attribute creates a boolean signal that is `true` while a fetch request is active and `false` otherwise. This signal can be used to display loading states, disable buttons, or show other UI feedback. Modifiers like `__case` can alter signal name casing.

```html
<button data-on:click="@get('/endpoint')"
        data-indicator:fetching
></button>
<button data-on:click="@get('/endpoint')"
        data-indicator:fetching
        data-attr:disabled="$fetching"
></button>
<div data-show="$fetching">Loading...</div>
<button data-indicator="fetching"></button>
<div data-indicator:fetching data-init="@get('/endpoint')"></div>
```

----

### Datastar Manual Loading Indicator

Source: https://data-star.dev/guide/the_tao_of_datastar

Shows a manual approach to implementing loading indicators in Datastar. The loading class is added to the button element before the backend request is made, and removed when the DOM is updated.

```html
<div>
    <button data-on:click="el.classList.add('loading'); @post('/do_something')">
        Do something
        <span>Loading...</span>
    </button>
</div>
```

----

### Attach Event Listener with data-on

Source: https://data-star.dev/guide/getting_started

Use the `data-on` attribute to attach an event listener to an HTML element. It executes a Datastar expression when the event is triggered.

```html
<button data-on:click="alert('I’m sorry, Dave. I’m afraid I can’t do that.')">
    Open the pod bay doors, HAL.
</button>
```

----

### HTML: Custom Event Dispatch and Handling

Source: https://data-star.dev/examples/custom_event

This snippet demonstrates dispatching a custom event named 'myevent' every second using JavaScript and listening to it with the `data-on` attribute in HTML. The `data-text` attribute dynamically updates to show the latest event details.

```html
<p
    id="foo"
    data-signals:_event-details
    data-on:myevent="$_eventDetails = evt.detail"
    data-text="`Last Event Details: ${$_eventDetails}`"
></p>
<script>
    const foo = document.getElementById("foo");
    setInterval(() => {
        foo.dispatchEvent(
            new CustomEvent("myevent", {
                detail: JSON.stringify({
                    eventTime: new Date().toLocaleTimeString(),
                }),
            })
        );
    }, 1000);
</script>
```

----

### Default Request Cancellation Behavior

Source: https://data-star.dev/reference/actions

Illustrates the default behavior of DataStar where multiple clicks on a button triggering a backend action will cancel previous requests. This ensures only the latest request is processed, preventing conflicts.

```html
<!-- Clicking this button multiple times will cancel previous requests (default behavior) -->
<button data-on:click="@get('/slow-endpoint')" data-datastar-element="true">Load Data</button>
```

----

### Conditional Rendering and Computed Properties in HTML

Source: https://data-star.dev/guide/reactive_signals

This HTML snippet showcases Datastar's capabilities for managing application state and conditional rendering. It defines signals for user input and a correct answer, uses a computed property to check correctness, and conditionally displays feedback based on the answer.

```html
<div
    data-signals="{response: '', answer: 'bread'}"
    data-computed:correct="$response.toLowerCase() == $answer"
>
    <div id="question">What do you put in a toaster?</div>
    <button data-on:click="$response = prompt('Answer:') ?? ''">BUZZ</button>
    <div data-show="$response != ''">
        You answered “<span data-text="$response"></span>”.
        <span data-show="$correct">That is correct ✅</span>
        <span data-show="!$correct">
        The correct answer is “
        <span data-text="$answer"></span>
        ” 🤷
        </span>
    </div>
</div>
```

----

### Predefined Signal Types with data-bind

Source: https://data-star.dev/reference/attributes

When a signal is predefined, its type is preserved during binding. `data-bind` automatically converts the element's value to match the original signal type, supporting numbers, strings, and arrays for multiple selections.

```html
<div data-signals:foo-bar="0">
    <select data-bind:foo-bar>
        <option value="10">10</option>
    </select>
</div>
<div data-signals:foo-bar="[]">
    <input data-bind:foo-bar type="checkbox" value="fizz" />
    <input data-bind:foo-bar type="checkbox" value="baz" />
</div>
```

----

### Call Asynchronous External Script Function in Data Star

Source: https://data-star.dev/docs

Shows how to handle asynchronous JavaScript functions within Data Star. The function dispatches a custom event with the result, which is then captured by Data Star. Data Star does not await asynchronous calls directly within expressions. Dependencies: Data Star library.

```html
<div data-signals:result>
    <input data-bind:foo 
           data-on:input="myfunction(el, $foo)"
           data-on:mycustomevent__window="$result = evt.detail.value"
    >
    <span data-text="$result"></span>
</div>
```

```javascript
async function myfunction(element, data) {
    const value = await new Promise((resolve) => {
        setTimeout(() => resolve(`You entered: ${data}`), 1000);
    });
    element.dispatchEvent(
        new CustomEvent('mycustomevent', {detail: {value}})
    );
}
```

----

### Datastar Expression: Conditional Output (Ternary Operator)

Source: https://data-star.dev/docs

Utilizes the ternary operator `?:` within a Datastar expression to conditionally display one of two values based on the truthiness of a signal. If `$landingGearRetracted` is true, 'Ready' is shown; otherwise, 'Waiting' is shown.

```html
<div data-text="$landingGearRetracted ? 'Ready' : 'Waiting'"></div>
```

----

### data-on-interval

Source: https://data-star.dev/reference/attributes

Executes an expression at a specified interval. The default interval is one second and can be adjusted using the `__duration` modifier. Supports view transitions.

```APIDOC
## data-on-interval

### Description
Runs an expression at a regular interval. The interval duration defaults to one second and can be modified using the `__duration` modifier.

### Modifiers

*   `__duration` – Sets the interval duration.
    *   `.500ms` – Interval duration of 500 milliseconds (accepts any integer).
    *   `.1s` – Interval duration of 1 second (default).
    *   `.leading` – Execute the first interval immediately.
*   `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.

### Example
```html
<div data-on-interval="$count++"></div>
<div data-on-interval__duration.500ms="$count++"></div>
```
```

----

### Signal Value Inherited from Predefined Signal

Source: https://data-star.dev/docs

If a signal is predefined using `data-signals`, `data-bind` will use the predefined signal's value instead of the element's initial value. Here, `$fooBar` inherits the value `fizz`.

```html
<div data-signals:foo-bar="'fizz'">
    <input data-bind:foo-bar value="baz" />
</div>
```

----

### Datastar Expression: Display Signal Value

Source: https://data-star.dev/docs

Illustrates how to display the value of a Datastar signal using the `data-text` attribute. The expression `$foo` directly references the signal named 'foo', which is initialized with the value '1'.

```html
<div data-signals:foo="1">
    <div data-text="$foo"></div>
</div>
```

----

### Datastar Expression: Multi-line Statements

Source: https://data-star.dev/guide/datastar_expressions

Illustrates multi-line Datastar expressions where statements must be separated by semicolons. Line breaks alone are not sufficient for statement separation in Datastar.

```html
1<div data-signals:foo="1">
2    <button data-on:click="
3        $landingGearRetracted = true; 
4        @post('/launch')
5    ">
6        Force launch
7    </button>
8</div>
```

----

### Datastar Expression: String Length

Source: https://data-star.dev/docs

Demonstrates accessing properties of a signal within a Datastar expression. Here, `$foo.length` calculates and displays the length of the string value of the 'foo' signal.

```html
<div data-text="$foo.length"></div>
```

----

### Throttle Event Listeners with data-on-signal-patch

Source: https://data-star.dev/docs

The data-on-signal-patch attribute can also be used with the __throttle modifier to control event listener execution frequency. It accepts time values and options like noleading and trailing for fine-grained control.

```html
<div data-on-signal-patch__throttle.500ms="doSomething()"></div>
```

----

### Throttle Event Listener with Data Attributes

Source: https://data-star.dev/docs

Applies throttling to an event listener using the `data-on-*` attribute with `__throttle` modifier. It accepts timing values like `.500ms` or `.1s`, and edge options like `.noleading` or `.trailing`. This is useful for limiting the rate at which an event handler is called.

```html
<!-- Example for throttle (no specific code provided in text, conceptual) -->
<div data-on-scroll__throttle.200ms="$handleScroll"></div>
```

----

### data-attr Attribute

Source: https://data-star.dev/docs

The `data-attr` attribute allows setting HTML attribute values dynamically using expressions, ensuring they stay in sync with the application state. It supports setting individual attributes or multiple attributes using key-value pairs.

```APIDOC
## `data-attr` Attribute

### Description
Sets the value of any HTML attribute to an expression, and keeps it in sync.

### Method
N/A (Attribute)

### Endpoint
N/A (Attribute)

### Parameters
#### Path Parameters
N/A

#### Query Parameters
N/A

#### Request Body
N/A

### Request Example
```html
<div data-attr:aria-label="$foo"></div>
<div data-attr="{'aria-label': $foo, disabled: $bar}"></div>
```

### Response
#### Success Response (N/A)
N/A

#### Response Example
N/A
```

----

### Use Computed Signals in Other Expressions

Source: https://data-star.dev/docs

Computed signals created with `data-computed` can be used as dependencies in other expressions, including those used by other Datastar attributes like `data-text`. This demonstrates using the computed signal 'foo' in a `data-text` binding.

```html
<div data-computed:foo="$bar + $baz"></div>
<div data-text="$foo"></div>
```

----

### Datastar Expression: Accessing Signal Value

Source: https://data-star.dev/guide/datastar_expressions

Demonstrates accessing a signal's value using the '$' prefix in a `data-text` attribute. The signal 'foo' is initialized with '1', and its value is displayed.

```html
1<div data-signals:foo="1">
2    <div data-text="$foo"></div>
3</div>
```

----

### Manage Signals with data-signals

Source: https://data-star.dev/guide/reactive_signals

The `data-signals` attribute is used to create or update globally accessible signals. It supports nested signals using dot notation and automatically converts hyphenated signal names to camel case. Multiple signals can be patched using key-value pairs.

```html
<div data-signals:foo-bar="1"></div>
```

```html
<div data-signals:form.baz="2"></div>
```

```html
<div data-signals:foo-bar="1"
     data-text="$fooBar"
></div>
```

```html
<div data-signals="{fooBar: 1, form: {baz: 2}}"></div>
```

----

### Create Element Reference with data-ref

Source: https://data-star.dev/reference/attributes

The `data-ref` attribute creates a new signal that is a reference to the element on which the data attribute is placed. The signal name can be specified in the key or the value. This is useful for referencing elements within your templates.

```html
<div data-ref:foo></div>
<div data-ref="foo"></div>
$foo is a reference to a <span data-text="$foo.tagName"></span> element
```

----

### Signal Casing Modifiers with data-bind

Source: https://data-star.dev/reference/attributes

Modifiers can be used with `data-bind` to alter the casing of the signal name. Supported cases include camel (default), kebab, snake, and pascal.

```html
<input data-bind:my-signal__case.kebab />
```

----

### Listen for 'Ctrl + L' Keydown Event Globally

Source: https://data-star.dev/how_tos/bind_keydown_events_to_specific_keys

Attaches a keydown event listener to the window that triggers an alert only when the 'Ctrl' and 'L' keys are pressed simultaneously. It checks `evt.ctrlKey` and `evt.key === 'l'`.

```html
<div data-on:keydown__window="evt.ctrlKey && evt.key === 'l' && alert('Key pressed')"></div>
```

----

### Include Aliased Datastar Bundle

Source: https://data-star.dev/reference/attributes

This script tag includes an aliased version of the Datastar JavaScript bundle. This is typically used to avoid conflicts with legacy libraries when `data-ignore` cannot be used, providing a `data-star-*` prefixed alternative for Datastar attributes.

```javascript
<script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0-RC.8/bundles/datastar-aliased.js"></script>
```

----

### Datastar Event Bubbling with Target ID

Source: https://data-star.dev/how_tos/keep_datastar_code_dry

Expands on event bubbling by showing how to capture and use a data attribute (e.g., 'data-id') from the clicked element. This allows the backend action to receive specific context for each button clicked.

```html
<div data-on:click="evt.target.tagName == 'BUTTON' 
    && ($id = evt.target.dataset.id)
    && @get('/endpoint')">
    <button data-id="1">Click me</button>
    <button data-id="2">No, click me!</button>
    <button data-id="3">Click us all!</button>
</div>
```

----

### data-on-interval: Execute expression at regular intervals

Source: https://data-star.dev/reference

The `data-on-interval` attribute executes a given expression at a regular interval, defaulting to one second. The `__duration` modifier can be used to customize this interval, accepting values in milliseconds or seconds.

```html
<div data-on-interval="$count++"></div>
```

----

### Conditional Visibility with data-show

Source: https://data-star.dev/guide/reactive_signals

The data-show attribute controls an element's visibility based on the evaluation of a Datastar expression. It's recommended to set initial display: none to prevent content flash before Datastar processes the attribute.

```html
<input data-bind:foo-bar />
<button data-show="$fooBar != ''">
    Save
</button>
<input data-bind:foo-bar />
<button data-show="$fooBar != ''" style="display: none">
    Save
</button>
```

----

### Prevent Default on 'Enter' Keydown Event

Source: https://data-star.dev/how_tos/bind_keydown_events_to_specific_keys

Attaches a keydown event listener to the window that prevents the default action (e.g., form submission) and shows an alert when the 'Enter' key is pressed. It uses `evt.preventDefault()` within the expression.

```html
<div data-on:keydown__window="evt.key === 'Enter' && (evt.preventDefault(), alert('Key pressed'))"></div>
```

----

### Keep SSE Connection Open When Page is Hidden

Source: https://data-star.dev/how_tos/prevent_sse_connections_closing

This snippet demonstrates how to configure an SSE connection to remain open even when the browser tab is in the background. It utilizes a custom data attribute `data-on:click` to trigger an event with the `openWhenHidden` option set to `true`. This is useful for applications that require continuous real-time updates.

```html
<button data-on:click="@get('/endpoint', {openWhenHidden: true})"></button>
```

----

### data-on-signal-patch

Source: https://data-star.dev/reference

Runs an expression whenever signals are patched, with options for filtering and timing modifiers.

```APIDOC
## data-on-signal-patch

### Description
Runs an expression whenever any signals are patched. This is useful for tracking changes, updating computed values, or triggering side effects when data updates. The `patch` variable is available in the expression.

### Modifiers
- `__delay` (e.g., `.500ms`, `.1s`): Delays the event listener.
- `__debounce` (e.g., `.500ms`, `.1s`, `.leading`, `.notrailing`): Debounces the event listener.
- `__throttle` (e.g., `.500ms`, `.1s`, `.noleading`, `.trailing`): Throttles the event listener.

### Example
```html
<div data-on-signal-patch="console.log('A signal changed!')"></div>
<div data-on-signal-patch="console.log('Signal patch:', patch)"></div>
<div data-on-signal-patch__debounce.500ms="doSomething()"></div>
```
```

----

### data-on-interval for Regular Expressions

Source: https://data-star.dev/reference/attributes

The data-on-interval attribute executes an expression at a specified interval, defaulting to one second. The __duration modifier can change this interval, and __viewtransition can wrap the expression in document.startViewTransition().

```html
<div data-on-interval="$count++"></div>
```

```html
<div data-on-interval__duration.500ms="$count++"></div>
```

----

### Datastar Expression: Conditional Visibility (Logical OR)

Source: https://data-star.dev/docs

Employs the logical OR operator `||` in a Datastar expression to control element visibility using `data-show`. The element will be shown if `$landingGearRetracted` is true OR if `$timeRemaining` is less than 10.

```html
<div data-show="$landingGearRetracted || $timeRemaining < 10">
    Countdown
</div>
```

----

### Datastar Expression: Accessing Signal Properties

Source: https://data-star.dev/guide/datastar_expressions

Illustrates accessing properties of a signal's value within a Datastar expression. The expression `$foo.length` assumes `$foo` holds a value with a 'length' property.

```html
1<div data-text="$foo.length"></div>
```

----

### data-on-signal-patch for Signal Change Events

Source: https://data-star.dev/reference/attributes

The data-on-signal-patch attribute runs an expression when any signals are patched. The 'patch' variable is available for signal patch details. Modifiers like __delay, __debounce, and __throttle can control the event listener timing.

```html
<div data-on-signal-patch="console.log('A signal changed!')"></div>
```

```html
<div data-on-signal-patch="console.log('Signal patch:', patch)"></div>
```

```html
<div data-on-signal-patch__debounce.500ms="doSomething()"></div>
```

----

### data-bind Attribute

Source: https://data-star.dev/docs

The `data-bind` attribute establishes a two-way data binding between an element's value and a Datastar signal. Changes to the element update the signal, and changes to the signal update the element. This is applicable to input, select, and textarea elements.

```APIDOC
## `data-bind` Attribute

### Description
Creates a signal (if one doesn’t already exist) and sets up two-way data binding between it and an element’s value. This means that the value of the element is updated when the signal changes, and the signal value is updated when the value of the element changes.

### Method
N/A (Attribute)

### Endpoint
N/A (Attribute)

### Parameters
#### Path Parameters
N/A

#### Query Parameters
N/A

#### Request Body
N/A

### Request Example
```html
<input data-bind:foo />
<input data-bind="foo" />
<input data-bind:foo-bar value="baz" />
<div data-signals:foo-bar="'fizz'">
    <input data-bind:foo-bar value="baz" />
</div>
<div data-signals:foo-bar="0">
    <select data-bind:foo-bar>
        <option value="10">10</option>
    </select>
</div>
<div data-signals:foo-bar="[]">
    <input data-bind:foo-bar type="checkbox" value="fizz" />
    <input data-bind:foo-bar type="checkbox" value="baz" />
</div>
<input type="file" data-bind:files multiple />
```

### Response
#### Success Response (N/A)
N/A

#### Response Example
N/A
```

----

### data-on-signal-patch-filter

Source: https://data-star.dev/reference

Filters which signals to watch when using the data-on-signal-patch attribute.

```APIDOC
## data-on-signal-patch-filter

### Description
Filters which signals to watch when using the `data-on-signal-patch` attribute. Accepts an object with `include` and/or `exclude` properties that are regular expressions.

### Example
```html
<!-- Only react to counter signal changes -->
<div data-on-signal-patch-filter="{include: /^counter$/}"></div>

<!-- React to all changes except those ending with "changes" -->
<div data-on-signal-patch-filter="{exclude: /changes$/}"></div>

<!-- Combine include and exclude filters -->
<div data-on-signal-patch-filter="{include: /user/, exclude: /password/}"></div>
```
```

----

### Datastar Expression: Accessing Element Properties

Source: https://data-star.dev/guide/datastar_expressions

Shows how to access the element's properties directly within a Datastar expression using the 'el' variable. Here, `el.offsetHeight` is used in a `data-text` attribute.

```html
1<div data-text="el.offsetHeight"></div>
```

----

### Attach Event Listeners with data-on

Source: https://data-star.dev/reference/attributes

The `data-on` attribute attaches event listeners to elements, executing expressions when events are triggered. An `evt` object is available. It supports various modifiers like `__once`, `__passive`, `__capture`, `__case`, `__delay`, `__debounce`, `__throttle`, `__viewtransition`, `__window`, `__outside`, `__prevent`, and `__stop` to customize event handling.

```html
<button data-on:click="$foo = ''">Reset</button>
<div data-on:my-event="$foo = evt.detail"></div>
<button data-on:click__window__debounce.500ms.leading="$foo = ''"></button>
<div data-on:my-event__case.camel="$foo = ''"></div>
```

----

### Bind HTML Attribute Values with data-attr

Source: https://data-star.dev/guide/reactive_signals

The `data-attr` attribute binds the value of any HTML attribute to a JavaScript expression. It supports setting single attributes and multiple attributes using key-value pairs. Attribute names with hyphens are converted to kebab case.

```html
<input data-bind:foo />
<button data-attr:disabled="$foo == ''">
    Save
</button>
```

```html
<button data-attr:aria-hidden="$foo">Save</button>
```

```html
<button data-attr="{disabled: $foo == '', 'aria-hidden': $foo}">Save</button>
```

----

### HTML Form for Inline Email Validation

Source: https://data-star.dev/examples/inline_validation

This HTML snippet defines a form with an email input field that triggers a server-side validation POST request on keydown. It includes labels, required attributes, ARIA live regions for accessibility, and a paragraph for displaying validation information.

```html
<div id="demo">
    <label>
        Email Address
        <input
            type="email"
            required
            aria-live="polite"
            aria-describedby="email-info"
            data-bind:email
            data-on:keydown__debounce.500ms="@post('/examples/inline_validation/validate')"
        />
    </label>
    <p id="email-info" class="info">The only valid email address is "test@test.com".</p>
    <label>
        First Name
        <input
            type="text"
            required
            aria-live="polite"
            data-bind:first-name
            data-on:keydown__debounce.500ms="@post('/examples/inline_validation/validate')"
        />
    </label>
    <label>
        Last Name
        <input
            type="text"
            required
            aria-live="polite"
            data-bind:last-name
            data-on:keydown__debounce.500ms="@post('/examples/inline_validation/validate')"
        />
    </label>
    <button
        class="success"
        data-on:click="@post('/examples/inline_validation')"
    >
        <i class="material-symbols:person-add"></i>
        Sign Up
    </button>
</div>
```

----

### Execute Side Effects with data-effect

Source: https://data-star.dev/reference/attributes

The `data-effect` attribute executes a JavaScript expression on initial page load and whenever any signals within the expression change. This is primarily used for performing side effects like updating other signals or making API calls.

```html
<div data-effect="$foo = $bar + $baz"></div>
```

----

### data-on-intersect

Source: https://data-star.dev/reference/attributes

Controls the behavior of event listeners based on element intersection with the viewport. Supports modifiers for once, exit, visibility thresholds, delays, debouncing, throttling, and view transitions.

```APIDOC
## data-on-intersect

### Description
Allows you to modify the element intersection behavior and the timing of the event listener.

### Modifiers

*   `__once` – Only triggers the event once.
*   `__exit` – Only triggers the event when the element exits the viewport.
*   `__half` – Triggers when half of the element is visible.
*   `__full` – Triggers when the full element is visible.
*   `__threshold` – Triggers when the element is visible by a certain percentage.
    *   `.25` – Triggers when 25% of the element is visible.
    *   `.75` – Triggers when 75% of the element is visible.
*   `__delay` – Delay the event listener.
    *   `.500ms` – Delay for 500 milliseconds (accepts any integer).
    *   `.1s` – Delay for 1 second (accepts any integer).
*   `__debounce` – Debounce the event listener.
    *   `.500ms` – Debounce for 500 milliseconds (accepts any integer).
    *   `.1s` – Debounce for 1 second (accepts any integer).
    *   `.leading` – Debounce with leading edge (must come after timing).
    *   `.notrailing` – Debounce without trailing edge (must come after timing).
*   `__throttle` – Throttle the event listener.
    *   `.500ms` – Throttle for 500 milliseconds (accepts any integer).
    *   `.1s` – Throttle for 1 second (accepts any integer).
    *   `.noleading` – Throttle without leading edge (must come after timing).
    *   `.trailing` – Throttle with trailing edge (must come after timing).
*   `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.

### Example
```html
<div data-on-intersect__once__full="$fullyIntersected = true"></div>
```
```

----

### Two-Way Data Binding with data-bind

Source: https://data-star.dev/guide/reactive_signals

The data-bind attribute establishes two-way data binding for input elements, synchronizing the element's value with a Datastar signal. It supports direct signal names or values, and automatically handles camel-casing for hyphenated attribute names.

```html
<input data-bind:foo />
<input data-bind="foo" />
<!-- Both of these create the signal `$fooBar` -->
<input data-bind:foo-bar />
<input data-bind="fooBar" />
```

----

### Infinite Scroll Trigger with data-on-intersect

Source: https://data-star.dev/examples/infinite_scroll

This HTML snippet demonstrates the core of the infinite scroll pattern. The `data-on-intersect` attribute on the last element triggers a request to a specified URL when that element becomes visible in the viewport. The response is then appended to the DOM.

```html
<div data-on-intersect="@get('/examples/infinite_scroll/more')">
    Loading...
</div>
```

----

### FetchInvalidContentType Error

Source: https://data-star.dev/errors/fetch_invalid_content_type

This error occurs when an unsupported content type is specified for a request. Datastar expects either 'json' (default) or 'form'.

```APIDOC
## FetchInvalidContentType

### Description
An invalid content type was provided. The content type must be either `json` (the default) or `form`.

### Method
POST

### Endpoint
/endpoint

### Parameters
#### Query Parameters
- **contentType** (string) - Optional - Specifies the content type for the request. Must be 'json' or 'form'. Defaults to 'json'.

### Request Example
```html
<button data-on:click="@post('/endpoint', {contentType: 'form'})></button>
```

### Response
#### Error Response (400)
- **error** (string) - Description of the error, e.g., "Invalid content type provided. Must be 'json' or 'form'."

#### Response Example
```json
{
  "error": "Invalid content type provided. Must be 'json' or 'form'."
}
```
```

----

### data-on-signal-patch-filter for Signal Filtering

Source: https://data-star.dev/reference/attributes

The data-on-signal-patch-filter attribute filters which signals are watched by data-on-signal-patch. It accepts an object with 'include' and/or 'exclude' properties that are regular expressions.

```html
<!-- Only react to counter signal changes -->
<div data-on-signal-patch-filter="{include: /^counter$/}"></div>
```

```html
<!-- React to all changes except those ending with "changes" -->
<div data-on-signal-patch-filter="{exclude: /changes$/}"></div>
```

```html
<!-- Combine include and exclude filters -->
<div data-on-signal-patch-filter="{include: /user/, exclude: /password/}"></div>
```

----

### data-signals Modifiers for Casing and Conditional Patching

Source: https://data-star.dev/reference/attributes

Modifiers for `data-signals` allow for signal name casing conversion (`__case`) and conditional patching if a signal is missing (`__ifmissing`). Keys in `data-signals:*` are converted to camel case.

```html
<div data-signals:my-signal__case.kebab="1"
     data-signals:foo__ifmissing="1"
></div>
```

----

### data-ref Modifiers for Casing

Source: https://data-star.dev/reference/attributes

Modifiers for `data-ref` allow you to modify the casing of the signal name. Supported cases include camel (default), kebab, snake, and pascal.

```html
<div data-ref:my-signal__case.kebab></div>
```

----

### Add/Remove Classes with data-class

Source: https://data-star.dev/guide/reactive_signals

The `data-class` attribute dynamically adds or removes CSS classes from an element based on a JavaScript expression. It supports single class toggling and multiple class management using key-value pairs. Class names with hyphens are converted to kebab case.

```html
<input data-bind:foo-bar />
<button data-class:success="$fooBar != ''">
    Save
</button>
```

```html
<button data-class:font-bold="$fooBar == 'strong'">
    Save
</button>
```

```html
<button data-class="{success: $fooBar != '', 'font-bold': $fooBar == 'strong'}">
    Save
</button>
```

----

### Setting Inline CSS Styles with data-style

Source: https://data-star.dev/reference/attributes

The `data-style` attribute sets and synchronizes inline CSS styles on an element based on an expression. It supports individual properties or multiple properties using an object notation. Falsy values restore original styles or remove the property.

```html
<div data-style:display="$hiding && 'none'"></div>
<div data-style:background-color="$red ? 'red' : 'blue'"></div>
<div data-style="{
   display: $hiding ? 'none' : 'flex',
   'background-color': $red ? 'red' : 'green'
}"></div>
<!-- When $x is false, color remains red from inline style -->
<div style="color: red;" data-style:color="$x && 'green'"></div>

<!-- When $hiding is true, display becomes none; when false, reverts to flex from inline style -->
<div style="display: flex;" data-style:display="$hiding && 'none'"></div>
```

----

### Patching Signals with data-signals

Source: https://data-star.dev/reference/attributes

The `data-signals` attribute patches (adds, updates, or removes) one or more signals. Values defined later in the DOM override earlier ones. Signals can be nested using dot-notation or patched using JavaScript object notation.

```html
<div data-signals:foo="1"></div>
<div data-signals:foo.bar="1"></div>
<div data-signals="{foo: {bar: 1, baz: 2}}"></div>
<div data-signals="{foo: null}"></div>
```

----

### Setting Text Content with data-text

Source: https://data-star.dev/guide/reactive_signals

The data-text attribute sets an element's text content based on the value of a Datastar signal. It supports Datastar expressions, allowing for dynamic text manipulation like function calls.

```html
<input data-bind:foo-bar />
<div data-text="$fooBar"></div>
<input data-bind:foo-bar />
<div data-text="$fooBar.toUpperCase()"></div>
```

----

### data-on-intersect

Source: https://data-star.dev/docs

Runs an expression when the element intersects with the viewport. Modifiers allow you to modify the element intersection behavior and the timing of the event listener.

```APIDOC
### `data-on-intersect` 

Runs an expression when the element intersects with the viewport.

```html
<div data-on-intersect="$intersected = true"></div>
```

#### Modifiers

Modifiers allow you to modify the element intersection behavior and the timing of the event listener.

- `__once` – Only triggers the event once.
- `__exit` – Only triggers the event when the element exits the viewport.
- `__half` – Triggers when half of the element is visible.
- `__full` – Triggers when the full element is visible.
- `__threshold` – Triggers when the element is visible by a certain percentage.
  
  - `.25` – Triggers when 25% of the element is visible.
  - `.75` – Triggers when 75% of the element is visible.
- `__delay` – Delay the event listener.
  
  - `.500ms` – Delay for 500 milliseconds (accepts any integer).
  - `.1s` – Delay for 1 second (accepts any integer).
- `__debounce` – Debounce the event listener.
  
  - `.500ms` – Debounce for 500 milliseconds (accepts any integer).
  - `.1s` – Debounce for 1 second (accepts any integer).
  - `.leading` – Debounce with leading edge (must come after timing).
  - `.notrailing` – Debounce without trailing edge (must come after timing).
- `__throttle` – Throttle the event listener.
  
  - `.500ms` – Throttle for 500 milliseconds (accepts any integer).
  - `.1s` – Throttle for 1 second (accepts any integer).
  - `.noleading` – Throttle without leading edge (must come after timing).
  - `.trailing` – Throttle with trailing edge (must come after timing).
- `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.

```html
<div data-on-intersect__once__full="$fullyIntersected = true"></div>
```
```

----

### Set HTML Attribute with data-attr

Source: https://data-star.dev/reference/attributes

The `data-attr` attribute sets the value of any HTML attribute to an expression and keeps it in sync. It can be used for a single attribute or multiple attributes using a key-value pair object.

```html
<div data-attr:aria-label="$foo"></div>
<div data-attr="{'aria-label': $foo, disabled: $bar}"></div>
```

----

### Detect Viewport Intersection with data-on-intersect

Source: https://data-star.dev/reference/attributes

The `data-on-intersect` attribute triggers an expression when an element intersects with the viewport. This is useful for lazy loading or triggering animations when content becomes visible.

```html
<div data-on-intersect="$intersected = true"></div>
```

----

### Event Modifiers

Source: https://data-star.dev/docs

Modifiers allow you to modify behavior when events are triggered. Some modifiers have tags to further modify the behavior.

```APIDOC
## Event Modifiers

Modifiers allow you to modify behavior when events are triggered. Some modifiers have tags to further modify the behavior.

- `__once` * – Only trigger the event listener once.
- `__passive` * – Do not call `preventDefault` on the event listener.
- `__capture` * – Use a capture event listener.
- `__case` – Converts the casing of the event.
  
  - `.camel` – Camel case: `myEvent`
  - `.kebab` – Kebab case: `my-event` (default)
  - `.snake` – Snake case: `my_event`
  - `.pascal` – Pascal case: `MyEvent`
- `__delay` – Delay the event listener.
  
  - `.500ms` – Delay for 500 milliseconds (accepts any integer).
  - `.1s` – Delay for 1 second (accepts any integer).
- `__debounce` – Debounce the event listener.
  
  - `.500ms` – Debounce for 500 milliseconds (accepts any integer).
  - `.1s` – Debounce for 1 second (accepts any integer).
  - `.leading` – Debounce with leading edge (must come after timing).
  - `.notrailing` – Debounce without trailing edge (must come after timing).
- `__throttle` – Throttle the event listener.
  
  - `.500ms` – Throttle for 500 milliseconds (accepts any integer).
  - `.1s` – Throttle for 1 second (accepts any integer).
  - `.noleading` – Throttle without leading edge (must come after timing).
  - `.trailing` – Throttle with trailing edge (must come after timing).
- `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.
- `__window` – Attaches the event listener to the `window` element.
- `__outside` – Triggers when the event is outside the element.
- `__prevent` – Calls `preventDefault` on the event listener.
- `__stop` – Calls `stopPropagation` on the event listener.

** Only works with built-in events.*

```html
<button data-on:click__window__debounce.500ms.leading="$foo = ''"></button>
<div data-on:my-event__case.camel="$foo = ''"></div>
```
```

----

### Add/Remove Classes with data-class

Source: https://data-star.dev/reference/attributes

The `data-class` attribute dynamically adds or removes CSS classes from an element based on a JavaScript expression. It supports single class toggling and multiple class management using key-value pairs. Modifiers like `__case` can alter class name casing.

```html
<div data-class:font-bold="$foo == 'strong'"></div>
<div data-class="{success: $foo != '', 'font-bold': $foo == 'strong'}"></div>
<div data-class:my-class__case.camel="$foo"></div>
```

----

### Create Computed Signals with data-computed

Source: https://data-star.dev/reference/attributes

The `data-computed` attribute creates a read-only reactive signal whose value is derived from an expression involving other signals. It's ideal for memoizing values and can be used in other expressions. Modifiers like `__case` can alter signal name casing.

```html
<div data-computed:foo="$bar + $baz"></div>
<div data-computed:foo="$bar + $baz"></div>
<div data-text="$foo"></div>
<div data-computed="{foo: () => $bar + $baz}"></div>
<div data-computed:my-signal__case.kebab="$bar + $baz"></div>
```

----

### Binding Text Content with data-text

Source: https://data-star.dev/reference/attributes

The `data-text` attribute binds the text content of an element to an expression, ensuring the element's text is always synchronized with the expression's evaluated value.

```html
<div data-text="$foo"></div>
```

----

### data-on-intersect Modifiers for Event Triggering

Source: https://data-star.dev/reference/attributes

The data-on-intersect attribute triggers an event based on element visibility. Modifiers like __once, __exit, __half, __full, __threshold, __delay, __debounce, __throttle, and __viewtransition allow fine-grained control over when and how the event is triggered.

```html
<div data-on-intersect__once__full="$fullyIntersected = true"></div>
```

----

### Derived Signals with data-computed

Source: https://data-star.dev/guide/reactive_signals

The data-computed attribute creates a read-only signal derived from a Datastar expression. Its value automatically updates when any signals within its expression change, useful for memoizing complex expressions.

```html
<input data-bind:foo-bar />
<div data-computed:repeated="$fooBar.repeat(2)" data-text="$repeated"></div>
```

=== COMPLETE CONTENT === This response contains all available snippets from this library. No additional content exists. Do not make further requests.