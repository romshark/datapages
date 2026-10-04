# Sessions

Package `sessions` declares the interfaces a session store implements and the `Record` it stores. Pass a store to `datapages.WithSessionManager`: `inmem` for development and tests, `natskv` for production, or any other `sessions.Manager`.

```mermaid
sequenceDiagram
    participant B as Browser
    participant D as Datapages server
    participant H as App handler
    participant S as Session store

    Note over B,S: Sign in
    B->>D: POST sign-in action
    D->>H: call the action
    H-->>D: NewSession{UserID, ExpiresAt, Data}
    D->>S: CloseSession(old token), if the request has one
    D->>S: CreateSession(record)
    S-->>D: token
    D-->>B: Set-Cookie: token

    Note over B,S: Request to a handler with a session parameter
    B->>D: request with the cookie
    D->>S: ReadSessionFromCookie(cookie)
    S-->>D: record
    D->>H: call with the session

    Note over B,S: Stream and sign out
    B->>D: open SSE stream
    D->>S: NotifyClosed(token)
    B->>D: POST sign-out action
    D->>H: call the action
    H-->>D: CloseSession(true)
    D->>S: CloseSession(token)
    D-->>B: clear the cookie
    S-->>D: NotifyClosed callback
    D-->>B: reload the page over the stream
```

- A cookie that names no session is cleared, and the request continues as a guest. A store error answers 503 and keeps the cookie.
- A session past its `ExpiresAt` is closed when it is next read, and its streams stop at `ExpiresAt`. `DeleteExpired` removes the sessions nobody reads again. The server never calls it: the application schedules it.
- `UserSessions` and `CloseAllUserSessions` are optional. The application calls them on the store, for a settings page or a sign-out everywhere. Their tokens are the ones the cookies carry, which `Session.Token()` returns, except where the store documents otherwise.
- The `inmem` cookie is the token itself. The `natskv` cookie is the session's KV key encrypted with AES-128-GCM, which keeps the user ID from the client.
