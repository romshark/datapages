---
name: datapages-architecture
description: >-
  Plan a Datapages app or feature: choose where to store values, how to update
  the browser, where to define handlers, how to scope event subjects and how
  many server processes to run. Use before other Datapages skills when these
  choices are not set.
---

# Architecture

Read `datapages` first for the build loop, hard rules and naming conventions.

Use this skill to choose the Datapages constructs a feature needs. Ask the operator when a missing requirement would change that choice. Then read the referenced skill before implementing the selected construct.

Keep authoritative state on the server, separate reads from commands and send complete page updates instead of small DOM patches.

## New app

```sh
datapages init  # go.mod, app/app.go, cmd/server/main.go, compose.yaml, Makefile, CI
make up         # start the broker
make dev        # datapages watch
```

`init` creates `PageIndex`, `PageError404`, `PageError500`, `Head` and `RecoverError`. It writes only missing files and deletes nothing. You can run it again without overwriting existing files.

`init` asks which Web Component setup to scaffold with the app package: Datastar Rocket, TypeScript bundled by esbuild, JavaScript modules or none. `--web-components rocket|typescript|javascript|none` answers without the prompt, and `-n` alone picks none. Pick one only when the app needs JavaScript that would be impractical in Datastar attributes; see `datapages-templates`.

Before the first page, decide:

| question | decides |
| --- | --- |
| Are users authenticated? | Session data type and the `Session` alias shared by all handlers. See `datapages-sessions`. |
| What does a handler need besides the request? | `App` fields. A page has no named dependency fields, only `App *App`. |
| Can data change without the reader acting? | Whether events are needed. See `datapages-events`. |
| Will 1 or multiple server processes run? | Broker, session store and data placement. See `Server topology`. |

Add sessions, state and events when a feature needs them, not upfront.

## Before implementation

Do not guess requirements that change the application model. Ask the operator when they are unclear.

| unknown | determines |
| --- | --- |
| Authentication or per-user data | Whether data belongs in a session. See `datapages-sessions`. |
| Persistence | What must survive a process restart and where it is stored. |
| External changes | Whether data can change without the current tab acting and needs events. See `datapages-events`. |
| Per-tab server state | Whether a handler without a request needs tab-local values. |
| URL state | Whether a value must be shareable or survive reload. |
| Server topology | Whether 1 or multiple server processes will run. See `Server topology`. |
| Shared routes | Whether a handler belongs to a page, `*App` or an embedded shared type. |

Do not ask when the requirement is already explicit in the task or existing code.

Use the smallest model that satisfies the requirements. Add sessions, events, `State[T]`, persistence or shared handlers only when a current requirement needs them.

## Default architecture

For server-backed interactive or real-time data, prefer Datastar's CQRS model:

- the server stores authoritative state;
- separate reads from commands;
- long-lived SSE handlers provide reads;
- short-lived actions are commands;
- commands mutate authoritative state and notify subscribers;
- `On` handlers react to events and render from authoritative state;
- prefer complete page updates over small DOM patches;
- keep plain `GET` rendering correct.

A command should normally mutate state and dispatch an event. The event says that something changed; it is not the authoritative state. Subscribers read the current state, render the page and send the result over SSE.

Render 1 template for each page and let Datastar update the changed DOM. Split templates into smaller components when useful. Do not add small DOM patches only to reduce transferred HTML. SSE streams use Brotli compression.

Each stream update must contain enough current state to restore the UI after a missed event or interrupted connection. Send complete current state when a missed incremental update would make the client incorrect.

Handle local interactions on the client. Examples include opening a menu, changing a class and validating a local field.

## Where to store a value

Give each value 1 owner:

| value | owner | constraint |
| --- | --- | --- |
| Shared by all users and outlives a request | `App` field behind a repository | 1 `App` serves all requests in 1 process. Guard its fields. For multiple servers, see `Server topology`. |
| 1 user across tabs and restarts | Session data and its store | Every handler uses the same data type. |
| 1 browser tab | `datapages.State[T]` | Reconnect starts zeroed. `StreamOpen` rebuilds it. |
| Shareable URL state that survives reload | `Query` field with `reflectsignal` | Seeds the signal on load and rewrites the URL on change. |
| Interaction-only state such as an open menu or a selected tab | Local Datastar signal, named with a `_` prefix | Requests do not carry it. |
| Input an action sends, such as draft text | Datastar signal without the prefix | The server sees it when an action sends it. |
| Client state a library holds or that runs ahead of the server, such as a map viewport or unconfirmed editor text | Web Component | The server keeps what it confirmed. The element sends changes as events. See `datapages-templates`. |
| Derived value | Nowhere | Compute it. Storing the same fact 2 times can create inconsistent values. |

An `On` handler receives no request or signals. Put what it needs on the event or in `State[T]`.

For example, copy a user-selected filter into `State[T]` in `StreamOpen` or in the action that changes it. The event handler can then render from that state.

`state.Values` is valid only while the handler runs. Never retain its pointer, store it in `App` or use it from a goroutine. Copy the required values.

## How to update the browser

| change | use |
| --- | --- |
| Client-only interaction: class, menu, field validation | Datastar attributes and signals. No request. |
| Server decides, only the acting tab needs the result | Take `datapages.SSE` in the action and patch. |
| Authoritative state changed and open pages must update | Command mutates state and dispatches an event; `On` handlers read current state and render it. |
| Navigate to another page | `href`, or `datapages.Redirect` from the action. |
| The input of a Web Component changed | Render its attributes from current state, and the morph changes them. Use `data-attr` with a signal for an element that morphs skip, and `ExecuteScript` for a stream of changes. See `datapages-templates`. |

The acting tab also receives subscribed events. A command that dispatches an event rarely needs `SSE`. Using both usually sends the same state 2 times.

For server-rendered updates, render a complete DOM tree from current state. Avoid coordinating many small patches or client-side signal updates.

Shrink morph targets only for a measured reason. A complete HTML fragment is often simpler because Datastar preserves unchanged DOM while morphing the received tree.

A morph also sets the attributes of a Web Component and morphs its light DOM. The component observes the attributes that can change and builds its own DOM in a shadow root, which morphs do not enter; see `datapages-templates`.

Events are notifications, not durable state. Delivery is at most once and has no replay. A hidden tab, interrupted stream or full buffer can miss an event. Later renders must use authoritative state. They must not depend on every earlier event.

Every page must also render correctly from a plain `GET`.

## Who owns the handler

| route | owner |
| --- | --- |
| Serves 1 page | That page type, value receiver. |
| Called from several pages | `*App`. A stream embed cannot declare actions. |
| Shared `On` or stream hook | A stream embed, a non-`Page` type with `App *App` embedded in each page. |
| Same `GET` on several pages | Each page's own `GET`, calling a shared function. |

Keep an app action stateless unless every calling page uses the same `State[T]` type. A state type mismatch returns 409.

If pages render changed data differently, make the shared action dispatch an event. Each page's `On` handler reads current state and renders its own page.

## Event subject scope

| receivers | subject |
| --- | --- |
| Everyone on the page | No subject fields. |
| 1 authenticated user across tabs | `datapages.SubjectUser` field. |
| 1 browser tab | `datapages.SubjectStateID` field populated from `stateID`. |
| Watchers of 1 room or document | `datapages.Subject` field with a `signal` tag. |

1 dispatch targets 1 subject. To target several users, dispatch once per user. Each dispatch can fail independently.

An event with subject fields claims every subject below its base subject. Define the subject namespace before splitting an event into several types.

Use notification events when subscribers can read authoritative state. Put data on the event only when the `On` handler cannot read it from a repository or `State[T]`.

An empty event is enough when every subscriber re-reads the repository.

## Multiple applications

1 Go module may contain several Datapages applications. Each application has its own app package, `datapagesgen` package and entry point.

Applications that use the same broker share 1 event namespace. They must not claim the same subject. For events shared across apps, declare the event type once in a shared package. Use that type in each application.

Use:

```sh
datapages watch --app <name>
```

when the module contains multiple applications.

## Server topology

Determine whether the app will use 1 server process or several before you select infrastructure or store shared data.

| component | single server | multiple servers |
| --- | --- | --- |
| Broker | `modules/messaging/inmem` | `modules/messaging/natscore` |
| Session store | `modules/sessions/natskv`, or `inmem` in development | `modules/sessions/natskv` |
| `State[T]` | In memory | 1 process per tab. Route its stream and actions by `Datapages-Instance`. |
| Data in `App` fields | May stay in memory | Move it to a datastore, or each process serves its own copy. |

The application model otherwise stays the same.

`modules/sessions/inmem` loses every session on restart and signs every user out. Do not use it in production, including on a single server. `modules/messaging/inmem` sends events only within 1 process. A restart loses only events in flight, which is consistent with at-most-once delivery.

## After changing the application model

Return to `datapages` for the build and validation loop.

Run:

```sh
datapages gen
```

`gen` validates the model and writes generated code. `datapages lint` performs the same validation without writing files. Use it in CI and editors.

CLI errors identify invalid application declarations. Fix those declarations. Never edit `datapagesgen` directly.

## Defaults

Use the construct with the fewest parts.

For server-backed interactive or real-time state, use this default:

`command -> mutate authoritative state -> dispatch event -> subscriber reads current state -> complete morph`

A complete morph sends a DOM tree derived from current state, potentially the whole page, instead of coordinating fine-grained updates.

Deviate when the requirements make a simpler or different model more suitable.

A page that re-reads data on navigation is correct before events exist. Add events only when changes must reach already open pages. Add `State[T]` only when a handler without a request needs a per-tab value.

<!-- written by datapages sha256:2d01fbd0391fbaaa -->
