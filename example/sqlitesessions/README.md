# sqlitesessions — Custom SQLite Session Manager Example

A minimal auth demo that implements [`sessmanager.SessionManager`](../../modules/sessmanager/sessmanager.go) against a local SQLite database via [sqinn-go](https://github.com/cvilsmeier/sqinn-go) (a non-cgo SQLite adapter that drives a small `sqinn` subprocess over stdin/stdout, so `go build` stays pure-Go). The point of the example is to show how a custom session store plugs into the Datapages framework — everything else (user table, bcrypt, templates) is just the smallest surrounding app that exercises it.

https://github.com/user-attachments/assets/d34b57f5-e24b-4274-8357-e4122b32d541

The interface implementation lives in [`app/sessionstore/sessionstore.go`](./app/sessionstore/sessionstore.go). It provides the four methods the framework calls (`ReadSessionFromCookie`, `CreateSession`, `CloseSession`, `NotifyClosed`).

## Run

```sh
datapages watch # or: make dev
```

Then open <http://localhost:7331>. Three demo users are seeded on first launch (`alice@example.com` / `bob@example.com` / `carol@example.com`, all with password `password123`).
