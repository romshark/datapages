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

For Datastar API reference, fetch from: https://context7.com/websites/data-star_dev

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
