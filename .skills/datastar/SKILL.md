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
For the full Datastar API reference, fetch from: https://data-star.dev/reference/plugins_core

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

----------------

# Datastar Docs

Read the full-page docs at [data-star.dev/docs](https://data-star.dev/docs) for the best experience.

## Guide

### Getting Started

Datastar simplifies frontend development, allowing you to build backend-driven, interactive UIs using a [hypermedia-first](https://hypermedia.systems/hypermedia-a-reintroduction/) approach that extends and enhances HTML.

Datastar provides backend reactivity like [htmx](https://htmx.org/) and frontend reactivity like [Alpine.js](https://alpinejs.dev/) in a lightweight frontend framework that doesn’t require any npm packages or other dependencies. It provides two primary functions:
. Modify the DOM and state by sending events from your backend.
. Build reactivity into your frontend using standard `data-*` HTML attributes.

> Other useful resources include an AI-generated [deep wiki](https://deepwiki.com/starfederation/datastar), LLM-ingestible [code samples](https://context7.com/websites/data-star_dev), and [single-page docs](https://data-star.dev/docs).

## `data-*` 

At the core of Datastar are `data-*` HTML attributes (hence the name). They allow you to add reactivity to your frontend and interact with your backend in a declarative way.

> The Datastar [VSCode extension](https://marketplace.visualstudio.com/items?itemName=starfederation.datastar-vscode) and [IntelliJ plugin](https://plugins.jetbrains.com/plugin/26072-datastar-support) provide autocompletion for all available `data-*` attributes.

The [`data-on`](https://data-star.dev/reference/attributes#data-on) attribute can be used to attach an event listener to an element and execute an expression whenever the event is triggered. The value of the attribute is a [Datastar expression](https://data-star.dev/guide/datastar_expressions) in which JavaScript can be used.

```
<button data-on:click="alert('I’m sorry, Dave. I’m afraid I can’t do that.')">
    Open the pod bay doors, HAL.
</button>
```

We’ll explore more data attributes in the [next section of the guide](https://data-star.dev/guide/reactive_signals).

## Patching Elements 

With Datastar, the backend *drives* the frontend by **patching** (adding, updating and removing) HTML elements in the DOM.

Datastar receives elements from the backend and manipulates the DOM using a morphing strategy (by default). Morphing ensures that only modified parts of the DOM are updated, and that only data attributes that have changed are [reapplied](https://data-star.dev/reference/attributes#attribute-evaluation-order), preserving state and improving performance.

Datastar provides [actions](https://data-star.dev/reference/actions#backend-actions) for sending requests to the backend. The [`@get()`](https://data-star.dev/reference/actions#get) action sends a `GET` request to the provided URL using a [fetch](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API) request.

```
<button data-on:click="@get('/endpoint')">
    Open the pod bay doors, HAL.
</button>
<div id="hal"></div>
```

> Actions in Datastar are helper functions that have the syntax `@actionName()`. Read more about actions in the [reference](https://data-star.dev/reference/actions).

If the response has a `content-type` of `text/html`, the top-level HTML elements will be morphed into the existing DOM based on the element IDs.

```
<div id="hal">
    I’m sorry, Dave. I’m afraid I can’t do that.
</div>
```

We call this a “Patch Elements” event because multiple elements can be patched into the DOM at once.

In the example above, the DOM must contain an element with a `hal` ID in order for morphing to work. Other [patching strategies](https://data-star.dev/reference/sse_events#datastar-patch-elements) are available, but morph is the best and simplest choice in most scenarios.

```
import (
    "github.com/starfederation/datastar-go/datastar"
    time
)

// Creates a new `ServerSentEventGenerator` instance.
sse := datastar.NewSSE(w,r)

// Patches elements into the DOM.
sse.PatchElements(
    `<div id="hal">I’m sorry, Dave. I’m afraid I can’t do that.</div>`
)

time.Sleep(1 * time.Second)

sse.PatchElements(
    `<div id="hal">Waiting for an order...</div>`
)
```

We’ll cover event streams and [SSE events](https://data-star.dev/reference/sse_events) in more detail [later in the guide](https://data-star.dev/guide/backend_requests), but as you can see, they are just plain text events with a special syntax, made simpler by the [SDKs](https://data-star.dev/reference/sdks).

### Reactive Signals

In a hypermedia approach, the backend drives state to the frontend and acts as the primary source of truth. It’s up to the backend to determine what actions the user can take next by patching appropriate elements in the DOM.

Sometimes, however, you may need access to frontend state that’s driven by user interactions. Click, input and keydown events are some of the more common user events that you’ll want your frontend to be able to react to.

Datastar uses *signals* to manage frontend state. You can think of signals as reactive variables that automatically track and propagate changes in and to [Datastar expressions](https://data-star.dev/guide/datastar_expressions). Signals are denoted using the `$` prefix.

## Data Attributes 

Datastar allows you to add reactivity to your frontend and interact with your backend in a declarative way using [custom `data-*` attributes](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Global_attributes/data-*).

> The Datastar [VSCode extension](https://marketplace.visualstudio.com/items?itemName=starfederation.datastar-vscode) and [IntelliJ plugin](https://plugins.jetbrains.com/plugin/26072-datastar-support) provide autocompletion for all available `data-*` attributes.

### `data-bind` 

The [`data-bind`](https://data-star.dev/reference/attributes#data-bind) attribute sets up two-way data binding on any HTML element that receives user input or selections. These include `input`, `textarea`, `select`, `checkbox` and `radio` elements, as well as web components whose value can be made reactive.

```
<input data-bind:foo />
```

This creates a new signal that can be called using `$foo`, and binds it to the element’s value. If either is changed, the other automatically updates.

You can accomplish the same thing passing the signal name as a *value*. This syntax can be more convenient to use with some templating languages.

```
<input data-bind="foo" />
```

According to the [HTML spec](https://developer.mozilla.org/en-US/docs/Web/HTML/Global_attributes/data-*), all [`data-*`](https://developer.mozilla.org/en-US/docs/Web/HTML/How_to/Use_data_attributes) attributes are case-insensitive. When Datastar processes these attributes, hyphenated names are automatically converted to camel case by removing hyphens and uppercasing the letter following each hyphen. For example, `data-bind:foo-bar` creates a signal named `$fooBar`.

```
<!-- Both of these create the signal `$fooBar` -->
<input data-bind:foo-bar />
<input data-bind="fooBar" />
```

Read more about [attribute casing](https://data-star.dev/reference/attributes#attribute-casing) in the reference.

### `data-text` 

The [`data-text`](https://data-star.dev/reference/attributes#data-text) attribute sets the text content of an element to the value of a signal. The `$` prefix is required to denote a signal.

```
<input data-bind:foo-bar />
<div data-text="$fooBar"></div>
```

The value of the `data-text` attribute is a [Datastar expression](https://data-star.dev/guide/datastar_expressions) that is evaluated, meaning that we can use JavaScript in it.

```
<input data-bind:foo-bar />
<div data-text="$fooBar.toUpperCase()"></div>
```

### `data-computed` 

The [`data-computed`](https://data-star.dev/reference/attributes#data-computed) attribute creates a new signal that is derived from a reactive expression. The computed signal is read-only, and its value is automatically updated when any signals in the expression are updated.

```
<input data-bind:foo-bar />
<div data-computed:repeated="$fooBar.repeat(2)" data-text="$repeated"></div>
```

This results in the `$repeated` signal’s value always being equal to the value of the `$fooBar` signal repeated twice. Computed signals are useful for memoizing expressions containing other signals.

### `data-show` 

The [`data-show`](https://data-star.dev/reference/attributes#data-show) attribute can be used to show or hide an element based on whether an expression evaluates to `true` or `false`.

```
<input data-bind:foo-bar />
<button data-show="$fooBar != ''">
    Save
</button>
```

This results in the button being visible only when the input value is *not* an empty string. This could also be shortened to `data-show="$fooBar"`.

Since the button is visible until Datastar processes the `data-show` attribute, it’s a good idea to set its initial style to `display: none` to prevent a flash of unwanted content.

```
<input data-bind:foo-bar />
<button data-show="$fooBar != ''" style="display: none">
    Save
</button>
```

### `data-class` 

The [`data-class`](https://data-star.dev/reference/attributes#data-class) attribute allows us to add or remove an element’s class based on an expression.

```
<input data-bind:foo-bar />
<button data-class:success="$fooBar != ''">
    Save
</button>
```

If the expression evaluates to `true`, the `success` class is added to the element, otherwise it is removed.

Unlike the `data-bind` attribute, in which hyphenated names are converted to camel case, the `data-class` attribute converts the class name to kebab case. For example, `data-class:font-bold` adds or removes the `font-bold` class.

```
<button data-class:font-bold="$fooBar == 'strong'">
    Save
</button>
```

The `data-class` attribute can also be used to add or remove multiple classes from an element using a set of key-value pairs, where the keys represent class names and the values represent expressions.

```
<button data-class="{success: $fooBar != '', 'font-bold': $fooBar == 'strong'}">
    Save
</button>
```

Note how the `font-bold` key must be wrapped in quotes because it contains a hyphen.

### `data-attr` 

The [`data-attr`](https://data-star.dev/reference/attributes#data-attr) attribute can be used to bind the value of any HTML attribute to an expression.

```
<input data-bind:foo />
<button data-attr:disabled="$foo == ''">
    Save
</button>
```

This results in a `disabled` attribute being given the value `true` whenever the input is an empty string.

The `data-attr` attribute also converts the attribute name to kebab case, since HTML attributes are typically written in kebab case. For example, `data-attr:aria-hidden` sets the value of the `aria-hidden` attribute.

```
<button data-attr:aria-hidden="$foo">Save</button>
```

The `data-attr` attribute can also be used to set the values of multiple attributes on an element using a set of key-value pairs, where the keys represent attribute names and the values represent expressions.

```
<button data-attr="{disabled: $foo == '', 'aria-hidden': $foo}">Save</button>
```

Note how the `aria-hidden` key must be wrapped in quotes because it contains a hyphen.

### `data-signals` 

Signals are globally accessible from anywhere in the DOM. So far, we’ve created signals on the fly using `data-bind` and `data-computed`. If a signal is used without having been created, it will be created automatically and its value set to an empty string.

Another way to create signals is using the [`data-signals`](https://data-star.dev/reference/attributes#data-signals) attribute, which patches (adds, updates or removes) one or more signals into the existing signals.

```
<div data-signals:foo-bar="1"></div>
```

Signals can be nested using dot-notation.

```
<div data-signals:form.baz="2"></div>
```

Like the `data-bind` attribute, hyphenated names used with `data-signals` are automatically converted to camel case by removing hyphens and uppercasing the letter following each hyphen.

```
<div data-signals:foo-bar="1"
     data-text="$fooBar"
></div>
```

The `data-signals` attribute can also be used to patch multiple signals using a set of key-value pairs, where the keys represent signal names and the values represent expressions. Nested signals can be created using nested objects.

```
<div data-signals="{fooBar: 1, form: {baz: 2}}"></div>
```

### `data-on` 

The [`data-on`](https://data-star.dev/reference/attributes#data-on) attribute can be used to attach an event listener to an element and run an expression whenever the event is triggered.

```
<input data-bind:foo />
<button data-on:click="$foo = ''">
    Reset
</button>
```

This results in the `$foo` signal’s value being set to an empty string whenever the button element is clicked. This can be used with any valid event name such as `data-on:keydown`, `data-on:mouseover`, etc.

Custom events can also be used. Like the `data-class` attribute, the `data-on` attribute converts the event name to kebab case. For example, `data-on:custom-event` listens for the `custom-event` event.

```
<div data-on:my-event="$foo = ''">
    <input data-bind:foo />
</div>
```

These are just *some* of the attributes available in Datastar. For a complete list, see the [attribute reference](https://data-star.dev/reference/attributes).

## Frontend Reactivity 

Datastar’s data attributes enable declarative signals and expressions, providing a simple yet powerful way to add reactivity to the frontend.

Datastar expressions are strings that are evaluated by Datastar [attributes](https://data-star.dev/reference/attributes) and [actions](https://data-star.dev/reference/actions). While they are similar to JavaScript, there are some important differences that are explained in the [next section of the guide](https://data-star.dev/guide/datastar_expressions).

```
<div data-signals:hal="'...'">
    <button data-on:click="$hal = 'Affirmative, Dave. I read you.'">
        HAL, do you read me?
    </button>
    <div data-text="$hal"></div>
</div>
```

See if you can figure out what the code below does based on what you’ve learned so far, *before* trying the demo below it.

```
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

You answered “”. That is correct ✅ The correct answer is “bread” 🤷

## Patching Signals 

Remember that in a hypermedia approach, the backend drives state to the frontend. Just like with elements, frontend signals can be **patched** (added, updated and removed) from the backend using [backend actions](https://data-star.dev/reference/actions#backend-actions).

```
<div data-signals:hal="'...'">
    <button data-on:click="@get('/endpoint')">
        HAL, do you read me?
    </button>
    <div data-text="$hal"></div>
</div>
```

If a response has a `content-type` of `application/json`, the signal values are patched into the frontend signals.

We call this a “Patch Signals” event because multiple signals can be patched (using [JSON Merge Patch RFC 7396](https://datatracker.ietf.org/doc/rfc7396/)) into the existing signals.

```
{"hal": "Affirmative, Dave. I read you."}
```

```
import (
    "github.com/starfederation/datastar-go/datastar"
)

// Creates a new `ServerSentEventGenerator` instance.
sse := datastar.NewSSE(w, r)

// Patches signals
sse.PatchSignals([]byte(`{hal: 'Affirmative, Dave. I read you.'}`))

time.Sleep(1 * time.Second)

sse.PatchSignals([]byte(`{hal: '...'}`))
```

We’ll cover event streams and [SSE events](https://data-star.dev/reference/sse_events) in more detail [later in the guide](https://data-star.dev/guide/backend_requests), but as you can see, they are just plain text events with a special syntax, made simpler by the [SDKs](https://data-star.dev/reference/sdks).

### Datastar Expressions

Datastar expressions are strings that are evaluated by `data-*` attributes. While they are similar to JavaScript, there are some important differences that make them more powerful for declarative hypermedia applications.

## Datastar Expressions 

The following example outputs `1` because we’ve defined `foo` as a signal with the initial value `1`, and are using `$foo` in a `data-*` attribute.

```
<div data-signals:foo="1">
    <div data-text="$foo"></div>
</div>
```

A variable `el` is available in every Datastar expression, representing the element that the attribute is attached to.

```
<div data-text="el.offsetHeight"></div>
```

When Datastar evaluates the expression `$foo`, it first converts it to the signal value, and then evaluates that expression in a sandboxed context. This means that JavaScript can be used in Datastar expressions.

```
<div data-text="$foo.length"></div>
```

JavaScript operators are also available in Datastar expressions. This includes (but is not limited to) the ternary operator `?:`, the logical OR operator `||`, and the logical AND operator `&&`. These operators are helpful in keeping Datastar expressions terse.

```
// Output one of two values, depending on the truthiness of a signal
<div data-text="$landingGearRetracted ? 'Ready' : 'Waiting'"></div>

// Show a countdown if the signal is truthy or the time remaining is less than 10 seconds
<div data-show="$landingGearRetracted || $timeRemaining < 10">
    Countdown
</div>

// Only send a request if the signal is truthy
<button data-on:click="$landingGearRetracted && @post('/launch')">
    Launch
</button>
```

Multiple statements can be used in a single expression by separating them with a semicolon.

```
<div data-signals:foo="1">
    <button data-on:click="$landingGearRetracted = true; @post('/launch')">
        Force launch
    </button>
</div>
```

Expressions may span multiple lines, but a semicolon must be used to separate statements. Unlike JavaScript, line breaks alone are not sufficient to separate statements.

```
<div data-signals:foo="1">
    <button data-on:click="
        $landingGearRetracted = true; 
        @post('/launch')
    ">
        Force launch
    </button>
</div>
```

## Using JavaScript 

Most of your JavaScript logic should go in `data-*` attributes, since reactive signals and actions only work in [Datastar expressions](https://data-star.dev/guide/datastar_expressions).

> Caution: if you find yourself trying to do too much in Datastar expressions, **you are probably overcomplicating it™**.

Any JavaScript functionality you require that cannot belong in `data-*` attributes should be extracted out into [external scripts](#external-scripts) or, better yet, [web components](#web-components).

> Always encapsulate state and send **props down, events up**.

### External Scripts 

When using external scripts, you should pass data into functions via arguments and return a result. Alternatively, listen for custom events dispatched from them (props down, events up).

In this way, the function is encapsulated – all it knows is that it receives input via an argument, acts on it, and optionally returns a result or dispatches a custom event – and `data-*` attributes can be used to drive reactivity.

```
<div data-signals:result>
    <input data-bind:foo 
        data-on:input="$result = myfunction($foo)"
    >
    <span data-text="$result"></span>
</div>
```

```
function myfunction(data) {
    return `You entered: ${data}`;
}
```

If your function call is asynchronous then it will need to dispatch a custom event containing the result. While asynchronous code *can* be placed within Datastar expressions, Datastar will *not* await it.

```
<div data-signals:result>
    <input data-bind:foo 
           data-on:input="myfunction(el, $foo)"
           data-on:mycustomevent__window="$result = evt.detail.value"
    >
    <span data-text="$result"></span>
</div>
```

```
async function myfunction(element, data) {
    const value = await new Promise((resolve) => {
        setTimeout(() => resolve(`You entered: ${data}`), 1000);
    });
    element.dispatchEvent(
        new CustomEvent('mycustomevent', {detail: {value}})
    );
}
```

See the [sortable example](https://data-star.dev/examples/sortable).

### Web Components 

[Web components](https://developer.mozilla.org/en-US/docs/Web/API/Web_components) allow you to create reusable, encapsulated, custom elements. They are native to the web and require no external libraries or frameworks. Web components unlock [custom elements](https://developer.mozilla.org/en-US/docs/Web/API/Web_components/Using_custom_elements) – HTML tags with custom behavior and styling.

When using web components, pass data into them via attributes and listen for custom events dispatched from them (*props down, events up*).

In this way, the web component is encapsulated – all it knows is that it receives input via an attribute, acts on it, and optionally dispatches a custom event containing the result – and `data-*` attributes can be used to drive reactivity.

```
<div data-signals:result="''">
    <input data-bind:foo />
    <my-component
        data-attr:src="$foo"
        data-on:mycustomevent="$result = evt.detail.value"
    ></my-component>
    <span data-text="$result"></span>
</div>
```

```
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

Since the `value` attribute is allowed on web components, it is also possible to use `data-bind` to bind a signal to the web component’s value. Note that a `change` event must be dispatched so that the event listener used by `data-bind` is triggered by the value change.

See the [web component example](https://data-star.dev/examples/web_component).

## Executing Scripts 

Just like elements and signals, the backend can also send JavaScript to be executed on the frontend using [backend actions](https://data-star.dev/reference/actions#backend-actions).

```
<button data-on:click="@get('/endpoint')">
    What are you talking about, HAL?
</button>
```

If a response has a `content-type` of `text/javascript`, the value will be executed as JavaScript in the browser.

```
alert('This mission is too important for me to allow you to jeopardize it.')
```

We’ll cover event streams and [SSE events](https://data-star.dev/reference/sse_events) in more detail [later in the guide](https://data-star.dev/guide/backend_requests), but as you can see, they are just plain text events with a special syntax, made simpler by the [SDKs](https://data-star.dev/reference/sdks).

### Backend Requests

Between [attributes](https://data-star.dev/reference/attributes) and [actions](https://data-star.dev/reference/actions), Datastar provides you with everything you need to build hypermedia-driven applications. Using this approach, the backend drives state to the frontend and acts as the single source of truth, determining what actions the user can take next.

## Sending Signals 

By default, all signals (except for local signals whose keys begin with an underscore) are sent in an object with every backend request. When using a `GET` request, the signals are sent as a `datastar` query parameter, otherwise they are sent as a JSON body.

By sending **all** signals in every request, the backend has full access to the frontend state. This is by design. It is **not** recommended to send partial signals, but if you must, you can use the [`filterSignals`](https://data-star.dev/reference/actions#filterSignals) option to filter the signals sent to the backend.

### Nesting Signals 

Signals can be nested, making it easier to target signals in a more granular way on the backend.

Using dot-notation:

```
<div data-signals:foo.bar="1"></div>
```

Using object syntax:

```
<div data-signals="{foo: {bar: 1}}"></div>
```

Using two-way binding:

```
<input data-bind:foo.bar />
```

A practical use-case of nested signals is when you have repetition of state on a page. The following example tracks the open/closed state of a menu on both desktop and mobile devices, and the [toggleAll()](https://data-star.dev/reference/actions#toggleAll) action to toggle the state of all menus at once.

```
<div data-signals="{menu: {isOpen: {desktop: false, mobile: false}}}">
    <button data-on:click="@toggleAll({include: /^menu\.isOpen\./})">
        Open/close menu
    </button>
</div>
```

## Backend Actions 

We’re not limited to sending just `GET` requests. Datastar provides [backend actions](https://data-star.dev/reference/actions#backend-actions) for each of the methods available: `@get()`, `@post()`, `@put()` and `@delete()`.

Here’s how we can send an answer to the server for processing, using a `POST` request.

```
<button data-on:click="@post('/actions/quiz')">
    Submit answer
</button>
```

One of the benefits of using SSE is that we can send multiple events (patch elements and patch signals) in a single response.

```
sse.PatchElements(`<div id="question">...</div>`)
sse.PatchElements(`<div id="instructions">...</div>`)
sse.PatchSignals([]byte(`{answer: '...', prize: '...'}`))
```

Read more about SSE events in the [reference](https://data-star.dev/reference/sse_events).

### The Tao of Datastar

## State in the Right Place 

Most state should live in the backend. Since the frontend is exposed to the user, the backend should be the source of truth for your application state.

## Start with the Defaults 

The default configuration options are the recommended settings for the majority of applications. Start with the defaults, and before you ever get tempted to change them, stop and ask yourself, [well... how did I get here?](https://youtu.be/5IsSpAOD6K8)

## Patch Elements & Signals 

Since the backend is the source of truth, it should *drive* the frontend by **patching** (adding, updating and removing) HTML elements and signals.

## Use Signals Sparingly 

Overusing signals typically indicates trying to manage state on the frontend. Favor fetching current state from the backend rather than pre-loading and assuming frontend state is current. A good rule of thumb is to *only* use signals for user interactions (e.g. toggling element visibility) and for sending new state to the backend (e.g. by binding signals to form input elements).

## In Morph We Trust 

Morphing ensures that only modified parts of the DOM are updated, preserving state and improving performance. This allows you to send down large chunks of the DOM tree (all the way up to the `html` tag), sometimes known as “fat morph”, rather than trying to manage fine-grained updates yourself. If you want to explicitly ignore morphing an element, place the [`data-ignore-morph`](https://data-star.dev/reference/attributes#data-ignore-morph) attribute on it.

## SSE Responses 

[SSE](https://html.spec.whatwg.org/multipage/server-sent-events.html) responses allow you to send `0` to `n` events, in which you can [patch elements](https://data-star.dev/guide/getting_started/#patching-elements), [patch signals](https://data-star.dev/guide/reactive_signals#patching-signals), and [execute scripts](https://data-star.dev/guide/datastar_expressions#executing-scripts). Since event streams are just HTTP responses with some special formatting that [SDKs](https://data-star.dev/reference/sdks) can handle for you, there’s no real benefit to using a content type other than [`text/event-stream`](https://data-star.dev/reference/actions#response-handling).

## Compression 

Since SSE responses stream events from the backend and morphing allows sending large chunks of DOM, compressing the response is a natural choice. Compression ratios of 200:1 are not uncommon when compressing streams using Brotli. Read more about compressing streams in [this article](https://andersmurphy.com/2025/04/15/why-you-should-use-brotli-sse.html).

## Backend Templating 

Since your backend generates your HTML, you can and should use your templating language to [keep things DRY](https://data-star.dev/how_tos/keep_datastar_code_dry) (Don’t Repeat Yourself).

## Page Navigation 

Page navigation hasn't changed in 30 years. Use the [anchor element](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/a) (`<a>`) to navigate to a new page, or a [redirect](https://data-star.dev/how_tos/redirect_the_page_from_the_backend) if redirecting from the backend. For smooth page transitions, use the [View Transition API](https://developer.mozilla.org/en-US/docs/Web/API/View_Transition_API).

## Browser History 

Browsers automatically keep a history of pages visited. As soon as you start trying to manage browser history yourself, you are adding complexity. Each page is a resource. Use anchor tags and let the browser do what it is good at.

## CQRS 

[CQRS](https://martinfowler.com/bliki/CQRS.html), in which commands (writes) and requests (reads) are segregated, makes it possible to have a single long-lived request to receive updates from the backend (reads), while making multiple short-lived requests to the backend (writes). It is a powerful pattern that makes real-time collaboration simple using Datastar. Here’s a basic example.

```
<div id="main" data-init="@get('/cqrs_endpoint')">
    <button data-on:click="@post('/do_something')">
        Do something
    </button>
</div>
```

## Loading Indicators 

Loading indicators inform the user that an action is in progress. Use the [`data-indicator`](https://data-star.dev/reference/attributes#data-indicator) attribute to show loading indicators on elements that trigger backend requests. Here’s an example of a button that shows a loading element while waiting for a response from the backend.

```
<div>
    <button data-indicator:_loading
            data-on:click="@post('/do_something')"
    >
        Do something
        <span data-show="$_loading">Loading...</span>
    </button>
</div>
```

When using [CQRS](#cqrs), it is generally better to manually show a loading indicator when backend requests are made, and allow it to be hidden when the DOM is updated from the backend. Here’s an example.

```
<div>
    <button data-on:click="el.classList.add('loading'); @post('/do_something')">
        Do something
        <span>Loading...</span>
    </button>
</div>
```

## Optimistic Updates 

Optimistic updates (also known as optimistic UI) are when the UI updates immediately as if an operation succeeded, before the backend actually confirms it. It is a strategy used to makes web apps feel snappier, when it in fact deceives the user. Imagine seeing a confirmation message that an action succeeded, only to be shown a second later that it actually failed. Rather than deceive the user, use [loading indicators](#loading-indicators) to show the user that the action is in progress, and only confirm success from the backend.

## Accessibility 

The web should be accessible to everyone. Datastar stays out of your way and leaves [accessibility](https://developer.mozilla.org/en-US/docs/Web/Accessibility) to you. Use semantic HTML, apply ARIA where it makes sense, and ensure your app works well with keyboards and screen readers. Here’s an example of using [`data-attr`](https://data-star.dev/reference/attributes#data-attr) to apply ARIA attributes to a button that toggles the visibility of a menu.

```
<button data-on:click="$_menuOpen = !$_menuOpen"
        data-attr:aria-expanded="$_menuOpen ? 'true' : 'false'"
>
    Open/Close Menu
</button>
<div data-attr:aria-hidden="$_menuOpen ? 'false' : 'true'"></div>
```

## Reference

### Attributes

Data attributes are [evaluated in the order](#attribute-evaluation-order) they appear in the DOM, have special [casing](#attribute-casing) rules, can be [aliased](#aliasing-attributes) to avoid conflicts with other libraries, can contain [Datastar expressions](#datastar-expressions), and have [runtime error handling](#error-handling).

> The Datastar [VSCode extension](https://marketplace.visualstudio.com/items?itemName=starfederation.datastar-vscode) and [IntelliJ plugin](https://plugins.jetbrains.com/plugin/26072-datastar-support) provide autocompletion for all available `data-*` attributes.

### `data-attr` 

Sets the value of any HTML attribute to an expression, and keeps it in sync.

```
<div data-attr:aria-label="$foo"></div>
```

The `data-attr` attribute can also be used to set the values of multiple attributes on an element using a set of key-value pairs, where the keys represent attribute names and the values represent expressions.

```
<div data-attr="{'aria-label': $foo, disabled: $bar}"></div>
```

### `data-bind` 

Creates a signal (if one doesn’t already exist) and sets up two-way data binding between it and an element’s value. This means that the value of the element is updated when the signal changes, and the signal value is updated when the value of the element changes.

The `data-bind` attribute can be placed on any HTML element on which data can be input or choices selected (`input`, `select`, `textarea` elements, and web components). Event listeners are added for `change` and `input` events.

```
<input data-bind:foo />
```

The signal name can be specified in the key (as above), or in the value (as below). This can be useful depending on the templating language you are using.

```
<input data-bind="foo" />
```

[Attribute casing](#attribute-casing) rules apply to the signal name.

```
<!-- Both of these create the signal `$fooBar` -->
<input data-bind:foo-bar />
<input data-bind="fooBar" />
```

The initial value of the signal is set to the value of the element, unless a signal has already been defined. So in the example below, `$fooBar` is set to `baz`.

```
<input data-bind:foo-bar value="baz" />
```

Whereas in the example below, `$fooBar` inherits the value `fizz` of the predefined signal.

```
<div data-signals:foo-bar="'fizz'">
    <input data-bind:foo-bar value="baz" />
</div>
```

#### Predefined Signal Types

When you predefine a signal, its **type** is preserved during binding. Whenever the element’s value changes, the signal value is automatically converted to match the original type.

For example, in the code below, `$fooBar` is set to the **number** `10` (not the string `"10"`) when the option is selected.

```
<div data-signals:foo-bar="0">
    <select data-bind:foo-bar>
        <option value="10">10</option>
    </select>
</div>
```

In the same way, you can assign multiple input values to a single signal by predefining it as an **array**. In the example below, `$fooBar` becomes `["fizz", "baz"]` when both checkboxes are checked, and `["", ""]` when neither is checked.

```
<div data-signals:foo-bar="[]">
    <input data-bind:foo-bar type="checkbox" value="fizz" />
    <input data-bind:foo-bar type="checkbox" value="baz" />
</div>
```

#### File Uploads

Input fields of type `file` will automatically encode file contents in base64. This means that a form is not required.

```
<input type="file" data-bind:files multiple />
```

The resulting signal is in the format `{ name: string, contents: string, mime: string }[]`. See the [file upload example](https://data-star.dev/examples/file_upload).

> If you want files to be uploaded to the server, rather than be converted to signals, use a form and with `multipart/form-data` in the [`enctype`](https://developer.mozilla.org/en-US/docs/Web/API/HTMLFormElement/enctype) attribute. See the [backend actions](https://data-star.dev/reference/actions#backend-actions) reference.

#### Modifiers

Modifiers allow you to modify behavior when binding signals using a key.

- `__case` – Converts the casing of the signal name.
  
  - `.camel` – Camel case: `mySignal` (default)
  - `.kebab` – Kebab case: `my-signal`
  - `.snake` – Snake case: `my_signal`
  - `.pascal` – Pascal case: `MySignal`

```
<input data-bind:my-signal__case.kebab />
```

### `data-class` 

Adds or removes a class to or from an element based on an expression.

```
<div data-class:font-bold="$foo == 'strong'"></div>
```

If the expression evaluates to `true`, the `hidden` class is added to the element; otherwise, it is removed.

The `data-class` attribute can also be used to add or remove multiple classes from an element using a set of key-value pairs, where the keys represent class names and the values represent expressions.

```
<div data-class="{success: $foo != '', 'font-bold': $foo == 'strong'}"></div>
```

#### Modifiers

Modifiers allow you to modify behavior when defining a class name using a key.

- `__case` – Converts the casing of the class.
  
  - `.camel` – Camel case: `myClass`
  - `.kebab` – Kebab case: `my-class` (default)
  - `.snake` – Snake case: `my_class`
  - `.pascal` – Pascal case: `MyClass`

```
<div data-class:my-class__case.camel="$foo"></div>
```

### `data-computed` 

Creates a signal that is computed based on an expression. The computed signal is read-only, and its value is automatically updated when any signals in the expression are updated.

```
<div data-computed:foo="$bar + $baz"></div>
```

Computed signals are useful for memoizing expressions containing other signals. Their values can be used in other expressions.

```
<div data-computed:foo="$bar + $baz"></div>
<div data-text="$foo"></div>
```

> Computed signal expressions must not be used for performing actions (changing other signals, actions, JavaScript functions, etc.). If you need to perform an action in response to a signal change, use the [`data-effect`](#data-effect) attribute.

The `data-computed` attribute can also be used to create computed signals using a set of key-value pairs, where the keys represent signal names and the values are callables (usually arrow functions) that return a reactive value.

```
<div data-computed="{foo: () => $bar + $baz}"></div>
```

#### Modifiers

Modifiers allow you to modify behavior when defining computed signals using a key.

- `__case` – Converts the casing of the signal name.
  
  - `.camel` – Camel case: `mySignal` (default)
  - `.kebab` – Kebab case: `my-signal`
  - `.snake` – Snake case: `my_signal`
  - `.pascal` – Pascal case: `MySignal`

```
<div data-computed:my-signal__case.kebab="$bar + $baz"></div>
```

### `data-effect` 

Executes an expression on page load and whenever any signals in the expression change. This is useful for performing side effects, such as updating other signals, making requests to the backend, or manipulating the DOM.

```
<div data-effect="$foo = $bar + $baz"></div>
```

### `data-ignore` 

Datastar walks the entire DOM and applies plugins to each element it encounters. It’s possible to tell Datastar to ignore an element and its descendants by placing a `data-ignore` attribute on it. This can be useful for preventing naming conflicts with third-party libraries, or when you are unable to [escape user input](https://data-star.dev/reference/security#escape-user-input).

```
<div data-ignore data-show-thirdpartylib="">
    <div>
        Datastar will not process this element.
    </div>
</div>
```

#### Modifiers

- `__self` – Only ignore the element itself, not its descendants.

### `data-ignore-morph` 

Similar to the `data-ignore` attribute, the `data-ignore-morph` attribute tells the `PatchElements` watcher to skip processing an element and its children when morphing elements.

```
<div data-ignore-morph>
    This element will not be morphed.
</div>
```

> To remove the `data-ignore-morph` attribute from an element, simply patch the element with the `data-ignore-morph` attribute removed.

### `data-indicator` 

Creates a signal and sets its value to `true` while a fetch request is in flight, otherwise `false`. The signal can be used to show a loading indicator.

```
<button data-on:click="@get('/endpoint')"
        data-indicator:fetching
></button>
```

This can be useful for showing a loading spinner, disabling a button, etc.

```
<button data-on:click="@get('/endpoint')"
        data-indicator:fetching
        data-attr:disabled="$fetching"
></button>
<div data-show="$fetching">Loading...</div>
```

The signal name can be specified in the key (as above), or in the value (as below). This can be useful depending on the templating language you are using.

```
<button data-indicator="fetching"></button>
```

When using `data-indicator` with a fetch request initiated in a `data-init` attribute, you should ensure that the indicator signal is created before the fetch request is initialized.

```
<div data-indicator:fetching data-init="@get('/endpoint')"></div>
```

#### Modifiers

Modifiers allow you to modify behavior when defining indicator signals using a key.

- `__case` – Converts the casing of the signal name.
  
  - `.camel` – Camel case: `mySignal` (default)
  - `.kebab` – Kebab case: `my-signal`
  - `.snake` – Snake case: `my_signal`
  - `.pascal` – Pascal case: `MySignal`

### `data-init` 

Runs an expression when the attribute is initialized. This can happen on page load, when an element is patched into the DOM, and any time the attribute is modified (via a backend action or otherwise).

> The expression contained in the [`data-init`](#data-init) attribute is executed when the element attribute is loaded into the DOM. This can happen on page load, when an element is patched into the DOM, and any time the attribute is modified (via a backend action or otherwise).

```
<div data-init="$count = 1"></div>
```

#### Modifiers

Modifiers allow you to add a delay to the event listener.

- `__delay` – Delay the event listener.
  
  - `.500ms` – Delay for 500 milliseconds (accepts any integer).
  - `.1s` – Delay for 1 second (accepts any integer).
- `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.

```
<div data-init__delay.500ms="$count = 1"></div>
```

### `data-json-signals` 

Sets the text content of an element to a reactive JSON stringified version of signals. Useful when troubleshooting an issue.

```
<!-- Display all signals -->
<pre data-json-signals></pre>
```

You can optionally provide a filter object to include or exclude specific signals using regular expressions.

```
<!-- Only show signals that include "user" in their path -->
<pre data-json-signals="{include: /user/}"></pre>

<!-- Show all signals except those ending in "temp" -->
<pre data-json-signals="{exclude: /temp$/}"></pre>

<!-- Combine include and exclude filters -->
<pre data-json-signals="{include: /^app/, exclude: /password/}"></pre>
```

#### Modifiers

Modifiers allow you to modify the output format.

- `__terse` – Outputs a more compact JSON format without extra whitespace. Useful for displaying filtered data inline.

```
<!-- Display filtered signals in a compact format -->
<pre data-json-signals__terse="{include: /counter/}"></pre>
```

### `data-on` 

Attaches an event listener to an element, executing an expression whenever the event is triggered.

```
<button data-on:click="$foo = ''">Reset</button>
```

An `evt` variable that represents the event object is available in the expression.

```
<div data-on:my-event="$foo = evt.detail"></div>
```

The `data-on` attribute works with [events](https://developer.mozilla.org/en-US/docs/Web/Events) and [custom events](https://developer.mozilla.org/en-US/docs/Web/Events/Creating_and_triggering_events). The `data-on:submit` event listener prevents the default submission behavior of forms.

#### Modifiers

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

```
<button data-on:click__window__debounce.500ms.leading="$foo = ''"></button>
<div data-on:my-event__case.camel="$foo = ''"></div>
```

### `data-on-intersect` 

Runs an expression when the element intersects with the viewport.

```
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

```
<div data-on-intersect__once__full="$fullyIntersected = true"></div>
```

### `data-on-interval` 

Runs an expression at a regular interval. The interval duration defaults to one second and can be modified using the `__duration` modifier.

```
<div data-on-interval="$count++"></div>
```

#### Modifiers

Modifiers allow you to modify the interval duration.

- `__duration` – Sets the interval duration.
  
  - `.500ms` – Interval duration of 500 milliseconds (accepts any integer).
  - `.1s` – Interval duration of 1 second (default).
  - `.leading` – Execute the first interval immediately.
- `__viewtransition` – Wraps the expression in `document.startViewTransition()` when the View Transition API is available.

```
<div data-on-interval__duration.500ms="$count++"></div>
```

### `data-on-signal-patch` 

Runs an expression whenever any signals are patched. This is useful for tracking changes, updating computed values, or triggering side effects when data updates.

```
<div data-on-signal-patch="console.log('A signal changed!')"></div>
```

The `patch` variable is available in the expression and contains the signal patch details.

```
<div data-on-signal-patch="console.log('Signal patch:', patch)"></div>
```

You can filter which signals to watch using the [`data-on-signal-patch-filter`](#data-on-signal-patch-filter) attribute.

#### Modifiers

Modifiers allow you to modify the timing of the event listener.

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

```
<div data-on-signal-patch__debounce.500ms="doSomething()"></div>
```

### `data-on-signal-patch-filter` 

Filters which signals to watch when using the [`data-on-signal-patch`](#data-on-signal-patch) attribute.

The `data-on-signal-patch-filter` attribute accepts an object with `include` and/or `exclude` properties that are regular expressions.

```
<!-- Only react to counter signal changes -->
<div data-on-signal-patch-filter="{include: /^counter$/}"></div>

<!-- React to all changes except those ending with "changes" -->
<div data-on-signal-patch-filter="{exclude: /changes$/}"></div>

<!-- Combine include and exclude filters -->
<div data-on-signal-patch-filter="{include: /user/, exclude: /password/}"></div>
```

### `data-preserve-attr` 

Preserves the value of an attribute when morphing DOM elements.

```
<details open data-preserve-attr="open">
    <summary>Title</summary>
    Content
</details>
```

You can preserve multiple attributes by separating them with a space.

```
<details open class="foo" data-preserve-attr="open class">
    <summary>Title</summary>
    Content
</details>
```

### `data-ref` 

Creates a new signal that is a reference to the element on which the data attribute is placed.

```
<div data-ref:foo></div>
```

The signal name can be specified in the key (as above), or in the value (as below). This can be useful depending on the templating language you are using.

```
<div data-ref="foo"></div>
```

The signal value can then be used to reference the element.

```
$foo is a reference to a <span data-text="$foo.tagName"></span> element
```

#### Modifiers

Modifiers allow you to modify behavior when defining references using a key.

- `__case` – Converts the casing of the signal name.
  
  - `.camel` – Camel case: `mySignal` (default)
  - `.kebab` – Kebab case: `my-signal`
  - `.snake` – Snake case: `my_signal`
  - `.pascal` – Pascal case: `MySignal`

```
<div data-ref:my-signal__case.kebab></div>
```

### `data-show` 

Shows or hides an element based on whether an expression evaluates to `true` or `false`. For anything with custom requirements, use [`data-class`](#data-class) instead.

```
<div data-show="$foo"></div>
```

To prevent flickering of the element before Datastar has processed the DOM, you can add a `display: none` style to the element to hide it initially.

```
<div data-show="$foo" style="display: none"></div>
```

### `data-signals` 

Patches (adds, updates or removes) one or more signals into the existing signals. Values defined later in the DOM tree override those defined earlier.

```
<div data-signals:foo="1"></div>
```

Signals can be nested using dot-notation.

```
<div data-signals:foo.bar="1"></div>
```

The `data-signals` attribute can also be used to patch multiple signals using a set of key-value pairs, where the keys represent signal names and the values represent expressions.

```
<div data-signals="{foo: {bar: 1, baz: 2}}"></div>
```

The value above is written in JavaScript object notation, but JSON, which is a subset and which most templating languages have built-in support for, is also allowed.

Setting a signal’s value to `null` or `undefined` removes the signal.

```
<div data-signals="{foo: null}"></div>
```

Keys used in `data-signals:*` are converted to camel case, so the signal name `mySignal` must be written as `data-signals:my-signal` or `data-signals="{mySignal: 1}"`.

Signals beginning with an underscore are *not* included in requests to the backend by default. You can opt to include them by modifying the value of the [`filterSignals`](https://data-star.dev/reference/actions#filterSignals) option.

> Signal names cannot begin with nor contain a double underscore (`__`), due to its use as a modifier delimiter.

#### Modifiers

Modifiers allow you to modify behavior when patching signals using a key.

- `__case` – Converts the casing of the signal name.
  
  - `.camel` – Camel case: `mySignal` (default)
  - `.kebab` – Kebab case: `my-signal`
  - `.snake` – Snake case: `my_signal`
  - `.pascal` – Pascal case: `MySignal`
- `__ifmissing` – Only patches signals if their keys do not already exist. This is useful for setting defaults without overwriting existing values.

```
<div data-signals:my-signal__case.kebab="1"
     data-signals:foo__ifmissing="1"
></div>
```

### `data-style` 

Sets the value of inline CSS styles on an element based on an expression, and keeps them in sync.

```
<div data-style:display="$hiding && 'none'"></div>
<div data-style:background-color="$red ? 'red' : 'blue'"></div>
```

The `data-style` attribute can also be used to set multiple style properties on an element using a set of key-value pairs, where the keys represent CSS property names and the values represent expressions.

```
<div data-style="{
    display: $hiding ? 'none' : 'flex',
    'background-color': $red ? 'red' : 'green'
}"></div>
```

Empty string, `null`, `undefined`, or `false` values will restore the original inline style value if one existed, or remove the style property if there was no initial value. This allows you to use the logical AND operator (`&&`) for conditional styles: `$condition && 'value'` will apply the style when the condition is true and restore the original value when false.

```
<!-- When $x is false, color remains red from inline style -->
<div style="color: red;" data-style:color="$x && 'green'"></div>

<!-- When $hiding is true, display becomes none; when false, reverts to flex from inline style -->
<div style="display: flex;" data-style:display="$hiding && 'none'"></div>
```

The plugin tracks initial inline style values and restores them when data-style expressions become falsy or during cleanup. This ensures existing inline styles are preserved and only the dynamic changes are managed by Datastar.

### `data-text` 

Binds the text content of an element to an expression.

```
<div data-text="$foo"></div>
```

## Attribute Evaluation Order 

Elements are evaluated by walking the DOM in a depth-first manner, and attributes are applied in the order they appear in the element. This is important in some cases, such as when using `data-indicator` with a fetch request initiated in a `data-init` attribute, in which the indicator signal must be created before the fetch request is initialized.

```
<div data-indicator:fetching data-init="@get('/endpoint')"></div>
```

Data attributes are evaluated and applied on page load (after Datastar has initialized), and are reapplied after any DOM patches that add, remove, or change them. Note that [morphing elements](https://data-star.dev/reference/sse_events#datastar-patch-elements) preserves existing attributes unless they are explicitly changed in the DOM, meaning they will only be reapplied if the attribute itself is changed.

## Attribute Casing 

[According to the HTML spec](https://developer.mozilla.org/en-US/docs/Web/HTML/Global_attributes/data-*), all `data-*` attributes (not Datastar the framework, but any time a data attribute appears in the DOM) are case-insensitive. When Datastar processes these attributes, hyphenated names are automatically converted to [camel case](https://developer.mozilla.org/en-US/docs/Glossary/Camel_case) by removing hyphens and uppercasing the letter following each hyphen.

Datastar handles casing of data attribute key suffixes containing hyphens in two ways:
. The keys used in attributes that define signals (`data-bind:*`, `data-signals:*`, `data-computed:*`, etc.), are converted to camel case (the recommended casing for signals) by removing hyphens and uppercasing the letter following each hyphen. For example, `data-signals:my-signal` defines a signal named `mySignal`, and you would use the signal in a [Datastar expression](https://data-star.dev/guide/datastar_expressions) as `$mySignal`.
. The keys suffixes used by all other attributes are, by default, converted to [kebab case](https://developer.mozilla.org/en-US/docs/Glossary/Kebab_case). For example, `data-class:text-blue-700` adds or removes the class `text-blue-700`, and `data-on:rocket-launched` would react to the event named `rocket-launched`.

You can use the `__case` modifier to convert between `camelCase`, `kebab-case`, `snake_case`, and `PascalCase`, or alternatively use object syntax when available.

For example, if listening for an event called `widgetLoaded`, you would use `data-on:widget-loaded__case.camel`.

## Datastar Expressions 

Datastar expressions used in `data-*` attributes parse signals, converting all dollar signs followed by valid signal name characters into their corresponding signal values. Expressions support standard JavaScript syntax, including operators, function calls, ternary expressions, and object and array literals.

A variable `el` is available in every Datastar expression, representing the element that the attribute exists on.

```
<div id="bar" data-text="$foo + el.id"></div>
```

Read more about [Datastar expressions](https://data-star.dev/guide/datastar_expressions) in the guide.

## Error Handling 

Datastar has built-in error handling and reporting for runtime errors. When a data attribute is used incorrectly, for example `data-text-foo`, the following error message is logged to the browser console.

```
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

The “More info” link takes you directly to a context-aware error page that explains the error and provides correct sample usage. See [the error page for the example above](https://data-star.dev/errors/key_not_allowed?metadata=%7B%22plugin%22%3A%7B%22name%22%3A%22text%22%2C%22type%22%3A%22attribute%22%7D%2C%22element%22%3A%7B%22id%22%3A%22%22%2C%22tag%22%3A%22DIV%22%7D%2C%22expression%22%3A%7B%22rawKey%22%3A%22textFoo%22%2C%22key%22%3A%22foo%22%2C%22value%22%3A%22%22%2C%22fnContent%22%3A%22%22%7D%7D), and all available error messages in the sidebar menu.

### Actions

Datastar provides actions (helper functions) that can be used in Datastar expressions.

> The `@` prefix designates actions that are safe to use in expressions. This is a security feature that prevents arbitrary JavaScript from being executed in the browser. Datastar uses [`Function()` constructors](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Function/Function) to create and execute these actions in a secure and controlled sandboxed environment.

### `@peek()` 

> `@peek(callable: () => any)`

Allows accessing signals without subscribing to their changes in expressions.

```
<div data-text="$foo + @peek(() => $bar)"></div>
```

In the example above, the expression in the `data-text` attribute will be re-evaluated whenever `$foo` changes, but it will *not* be re-evaluated when `$bar` changes, since it is evaluated inside the `@peek()` action.

### `@setAll()` 

> `@setAll(value: any, filter?: {include: RegExp, exclude?: RegExp})`

Sets the value of all matching signals (or all signals if no filter is used) to the expression provided in the first argument. The second argument is an optional filter object with an `include` property that accepts a regular expression to match signal paths. You can optionally provide an `exclude` property to exclude specific patterns.

```
<!-- Sets the `foo` signal only -->
<div data-signals:foo="false">
    <button data-on:click="@setAll(true, {include: /^foo$/})"></button>
</div>

<!-- Sets all signals starting with `user.` -->
<div data-signals="{user: {name: '', nickname: ''}}">
    <button data-on:click="@setAll('johnny', {include: /^user\./})"></button>
</div>

<!-- Sets all signals except those ending with `_temp` -->
<div data-signals="{data: '', data_temp: '', info: '', info_temp: ''}">
    <button data-on:click="@setAll('reset', {include: /.*/, exclude: /_temp$/})"></button>
</div>
```

### `@toggleAll()` 

> `@toggleAll(filter?: {include: RegExp, exclude?: RegExp})`

Toggles the boolean value of all matching signals (or all signals if no filter is used). The argument is an optional filter object with an `include` property that accepts a regular expression to match signal paths. You can optionally provide an `exclude` property to exclude specific patterns.

```
<!-- Toggles the `foo` signal only -->
<div data-signals:foo="false">
    <button data-on:click="@toggleAll({include: /^foo$/})"></button>
</div>

<!-- Toggles all signals starting with `is` -->
<div data-signals="{isOpen: false, isActive: true, isEnabled: false}">
    <button data-on:click="@toggleAll({include: /^is/})"></button>
</div>

<!-- Toggles signals starting with `settings.` -->
<div data-signals="{settings: {darkMode: false, autoSave: true}}">
    <button data-on:click="@toggleAll({include: /^settings\./})"></button>
</div>
```

## Backend Actions 

### `@get()` 

> `@get(uri: string, options={ })`

Sends a `GET` request to the backend using the [Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API). The URI can be any valid endpoint and the response must contain zero or more [Datastar SSE events](https://data-star.dev/reference/sse_events).

```
<button data-on:click="@get('/endpoint')"></button>
```

By default, requests are sent with a `Datastar-Request: true` header, and a `{datastar: *}` object containing all existing signals, except those beginning with an underscore. This behavior can be changed using the [`filterSignals`](#filterSignals) option, which allows you to include or exclude specific signals using regular expressions.

> When using a `get` request, the signals are sent as a query parameter, otherwise they are sent as a JSON body.

When a page is hidden (in a background tab, for example), the default behavior for `get` requests is for the SSE connection to be closed, and reopened when the page becomes visible again. To keep the connection open when the page is hidden, set the [`openWhenHidden`](#openWhenHidden) option to `true`.

```
<button data-on:click="@get('/endpoint', {openWhenHidden: true})"></button>
```

It’s possible to send form encoded requests by setting the `contentType` option to `form`. This sends requests using `application/x-www-form-urlencoded` encoding.

```
<button data-on:click="@get('/endpoint', {contentType: 'form'})"></button>
```

It’s also possible to send requests using `multipart/form-data` encoding by specifying it in the `form` element’s [`enctype`](https://developer.mozilla.org/en-US/docs/Web/API/HTMLFormElement/enctype) attribute. This should be used when uploading files. See the [form data example](https://data-star.dev/examples/form_data).

```
<form enctype="multipart/form-data">
    <input type="file" name="file" />
    <button data-on:click="@get('/endpoint', {contentType: 'form'})"></button>
</form>
```

### `@post()` 

> `@post(uri: string, options={ })`

Works the same as [`@get()`](#get) but sends a `POST` request to the backend.

```
<button data-on:click="@post('/endpoint')"></button>
```

### `@put()` 

> `@put(uri: string, options={ })`

Works the same as [`@get()`](#get) but sends a `PUT` request to the backend.

```
<button data-on:click="@put('/endpoint')"></button>
```

### `@patch()` 

> `@patch(uri: string, options={ })`

Works the same as [`@get()`](#get) but sends a `PATCH` request to the backend.

```
<button data-on:click="@patch('/endpoint')"></button>
```

### `@delete()` 

> `@delete(uri: string, options={ })`

Works the same as [`@get()`](#get) but sends a `DELETE` request to the backend.

```
<button data-on:click="@delete('/endpoint')"></button>
```

### Options 

All of the actions above take a second argument of options.

- `contentType` – The type of content to send. A value of `json` sends all signals in a JSON request. A value of `form` tells the action to look for the closest form to the element on which it is placed (unless a `selector` option is provided), perform validation on the form elements, and send them to the backend using a form request (no signals are sent). Defaults to `json`.
- `filterSignals` – A filter object with an `include` property that accepts a regular expression to match signal paths (defaults to all signals: `/.*/`), and an optional `exclude` property to exclude specific signal paths (defaults to all signals that do not have a `_` prefix: `/(^_|\._).*/`).
  
- `selector` – Optionally specifies a form to send when the `contentType` option is set to `form`. If the value is `null`, the closest form is used. Defaults to `null`.
- `headers` – An object containing headers to send with the request.
- `openWhenHidden` – Whether to keep the connection open when the page is hidden. Useful for dashboards but can cause a drain on battery life and other resources when enabled. Defaults to `false` for `get` requests, and `true` for all other HTTP methods.
- `payload` – Allows the fetch payload to be overridden with a custom object.
- `retry` – Determines when to retry requests. Can be `'auto'` (default, retries on network errors only), `'error'` (retries on `4xx` and `5xx` responses), `'always'` (retries on all non-`204` responses except redirects), or `'never'` (disables retries). Defaults to `'auto'`.
- `retryInterval` – The retry interval in milliseconds. Defaults to `1000` (one second).
- `retryScaler` – A numeric multiplier applied to scale retry wait times. Defaults to `2`.
- `retryMaxWaitMs` – The maximum allowable wait time in milliseconds between retries. Defaults to `30000` (30 seconds).
- `retryMaxCount` – The maximum number of retry attempts. Defaults to `10`.
- `requestCancellation` – Controls request cancellation behavior. Can be `'auto'` (default, cancels existing requests on the same element), `'cleanup'` (cancels existing requests on the same element and on element or attribute cleanup), `'disabled'` (allows concurrent requests), or an `AbortController` instance for custom control. Defaults to `'auto'`.

```
<button data-on:click="@get('/endpoint', {
    filterSignals: {include: /^foo\./},
    openWhenHidden: true,
    requestCancellation: 'disabled',
})"></button>
```

### Request Cancellation 

By default, when a new fetch request is initiated on an element, any existing request on that same element is automatically cancelled. This prevents multiple concurrent requests from conflicting with each other and ensures clean state management.

For example, if a user rapidly clicks a button that triggers a backend action, only the most recent request will be processed:

```
<!-- Clicking this button multiple times will cancel previous requests (default behavior) -->
<button data-on:click="@get('/slow-endpoint')">Load Data</button>
```

This automatic cancellation happens at the element level, meaning requests on different elements can run concurrently without interfering with each other.

You can control this behavior using the [`requestCancellation`](#requestCancellation) option:

```
<!-- Allow concurrent requests (no automatic cancellation) -->
<button data-on:click="@get('/endpoint', {requestCancellation: 'disabled'})">Allow Multiple</button>

<!-- Custom abort controller for fine-grained control -->
<div data-signals:controller="new AbortController()">
    <button data-on:click="@get('/endpoint', {requestCancellation: $controller})">Start Request</button>
    <button data-on:click="$controller.abort()">Cancel Request</button>
</div>
```

### Events 

All of the actions above trigger `datastar-fetch` events during the fetch request lifecycle. The event type determines the stage of the request.

- `started` – Triggered when the fetch request is started.
- `finished` – Triggered when the fetch request is finished.
- `error` – Triggered when the fetch request encounters an error.
- `retrying` – Triggered when the fetch request is retrying.
- `retries-failed` – Triggered when all fetch retries have failed.

```
<div data-on:datastar-fetch="
    evt.detail.type === 'error' && console.log('Fetch error encountered')
"></div>
```

### Security

## Escape User Input 

The golden rule of security is to never trust user input. This is especially true when using Datastar expressions, which can execute arbitrary JavaScript. When using Datastar expressions, you should always escape user input. This helps prevent, among other issues, Cross-Site Scripting (XSS) attacks.

## Avoid Sensitive Data 

Keep in mind that signal values are visible in the source code in plain text, and can be modified by the user before being sent in requests. For this reason, you should avoid leaking sensitive data in signals and always implement backend validation.

## Ignore Unsafe Input 

If, for some reason, you cannot escape unsafe user input, you should ignore it using the [`data-ignore`](https://data-star.dev/reference/attributes#data-ignore) attribute. This tells Datastar to ignore an element and its descendants when processing DOM nodes.

## Content Security Policy 

When using a [Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP) (CSP), `unsafe-eval` must be allowed for scripts, since Datastar evaluates expressions using a [`Function()` constructor](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Function/Function).

```
<meta http-equiv="Content-Security-Policy" 
    content="script-src 'self' 'unsafe-eval';"
>
```
