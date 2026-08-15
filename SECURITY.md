# Security Policy

## Supported Versions

Only the latest release is supported with security updates.

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly
via [GitHub's private vulnerability reporting](https://github.com/romshark/datapages/security/advisories/new).

**⚠️ Please do not open public issues for security vulnerabilities.**

You can expect an initial response within 72 hours. Once the issue is confirmed,
a fix will be developed and released as soon as possible, typically within 30 days
depending on complexity.

## Scope

What the generated server provides:

- **Request limits.** A read timeout, a header read timeout,
  a header size limit and a 1 MiB request body limit.
- **Sessions and CSRF.** Modules the application wires in.
- **Per-tab state isolation.** State is reached only with an HMAC-signed
  identifier that the server minted, and that identifier stays out of message
  broker subjects.

What is the responsibility of the application or of the deployment instead.
A report about one of these is a feature request rather than a vulnerability.

- **Volumetric abuse.** Nothing is metered per IP or per client.
  `MaxConcurrentInstances` bounds per-tab state for the whole process and
  nothing else is counted. An instance lives no longer than its stream,
  which leaves a per-client connection limit bounding how many one client can hold.
  Put one in front of the server. `SPECIFICATION.md` explains how middleware
  caps instances per session.
- **Authorization.** The framework routes a request and hands the handler its session.
  What that session may see or change is the application's decision.
- **Cross-site scripting.** `templ` escapes the values a template interpolates
  into text and into attributes. Two places need more than that. `templ.Raw`
  writes markup verbatim. A value interpolated into a Datastar attribute lands
  inside a JavaScript expression, where the browser decodes the HTML escaping
  before the expression is parsed. Both are the application's to get right.
- **The identity of per-tab state.** An instance is not bound to a session or a
  user and survives a sign-out. Do not keep in it what the next session on that
  tab may not see.
- **Key management.** The application supplies the HMAC and encryption keys.
  Storage and rotation are its own, and a key given to one subsystem should not
  be shared with another.
- **Transport.** TLS versions, ciphers, HSTS and the certificate lifecycle
  belong to whatever terminates TLS, including when that is `ListenAndServeTLS`.
- **Slow readers.** The write timeout is disabled by default because SSE requires it.
