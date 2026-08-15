// Package sessionstore is the main highlight of the sqlitesessions
// example: a SQLite-backed implementation of the framework's
// [sessmanager.SessionManager] interface for [app.Session].
//
// # Schema design
//
// The sessions table stores the bare minimum: a token, the owning
// user id, and create/expire timestamps. Display fields like Name
// and Email stay in the users table and are joined in on read rather
// than copied into each session row.
//
// # Framework methods
//
// The framework requires exactly four methods
// ([sessmanager.SessionManager]):
//
//   - ReadSessionFromCookie — called on every authenticated request
//     to materialize the session from the cookie value.
//   - CreateSession — called by the framework when a POST handler
//     returns a non-zero Session in its `newSession` return value.
//   - CloseSession — called when a handler returns `closeSession=true`,
//     or when something else wants the session gone.
//   - NotifyClosed — lets the framework subscribe to a session's
//     closure so it can tear down any associated SSE streams.
package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"

	"github.com/romshark/datapages/example/sqlitesessions/app"
	"github.com/romshark/datapages/example/sqlitesessions/app/sqdb"
	"github.com/romshark/datapages/modules/sessmanager"
)

// Store implements [sessmanager.SessionManager] for [app.Session].
//
// DB access goes through sqdb.DB, which handles every mutex concern —
// Store itself holds no DB lock. The only mutex it owns is notifyLock,
// which guards the in-memory notifier map used by NotifyClosed.
type Store struct {
	db       sqdb.DB
	tokenGen sessmanager.TokenGenerator
	lifetime time.Duration
	log      *slog.Logger

	notifyLock sync.Mutex
	notify     map[string][]notifier
}

// notifier is a registered NotifyClosed callback paired with the
// context that bounds its lifetime.
type notifier struct {
	ctx context.Context
	fn  func()
}

// Compile-time proof that *Store satisfies the framework interface.
// If you add or remove a method this line will break first, pointing
// at the contract, instead of at some random call site elsewhere.
var _ sessmanager.SessionManager[app.SessionData] = (*Store)(nil)

// New creates the sessions table if missing and returns a Store ready
// to plug into [datapagesgen.NewServer] as the session manager
// positional argument.
//
// The schema uses a foreign-key `REFERENCES users(id) ON DELETE
// CASCADE` so deleting a user automatically invalidates every session
// of that user — but only if the caller has enabled
// `PRAGMA foreign_keys = ON` on the sqinn connection. The users table
// must already exist; it is the userstore package's responsibility.
//
// lifetime is the maximum age of a session before ReadSessionFromCookie
// treats it as expired. Pass 0 to disable expiry entirely. log
// defaults to [slog.Default]() when nil.
func New(
	db sqdb.DB,
	tokenGen sessmanager.TokenGenerator,
	lifetime time.Duration,
	log *slog.Logger,
) (*Store, error) {
	if log == nil {
		log = slog.Default()
	}
	schema := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			token       TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at  INTEGER NOT NULL,
			expires_at  INTEGER NOT NULL
		)`,
		// Speeds up (future) "all sessions for this user" lookups and
		// the ON DELETE CASCADE check above.
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
	}
	for _, stmt := range schema {
		if err := db.ExecSql(stmt); err != nil {
			return nil, fmt.Errorf("initializing sessions schema: %w", err)
		}
	}
	return &Store{
		db:       db,
		tokenGen: tokenGen,
		lifetime: lifetime,
		log:      log,
		notify:   make(map[string][]notifier),
	}, nil
}

// ReadSessionFromCookie resolves a session cookie to a fully-populated
// [app.Session] in a single round trip — a JOIN between sessions and
// users gives us both the persisted session fields and the current
// display fields.
//
// The return contract has three distinct "no session" shapes:
//
//   - (zero, "", "", false, nil) — the cookie is missing, the row is
//     gone, or the row was expired and just got cleaned up. The
//     framework treats the request as a guest and clears the cookie.
//   - (zero, "", "", false, err) — the DB itself errored. The
//     framework fails the request so transient DB outages don't
//     silently downgrade users to guests.
//   - (populated, token, userID, true, nil) — the happy path.
//
// Lazy expiry: when the row is found but past expires_at, we call
// [Store.CloseSession] synchronously to drop it, then report ok=false.
// If the cleanup itself errors, we log it and still report ok=false —
// the next read will try again.
func (s *Store) ReadSessionFromCookie(c *http.Cookie) (
	rec sessmanager.Record[app.SessionData], token string, ok bool, err error,
) {
	if c == nil || c.Value == "" {
		return rec, "", false, nil
	}
	token = c.Value

	rows, qerr := s.db.QueryRows(
		`SELECT s.user_id, s.created_at, s.expires_at, u.name, u.email
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		sqinn.Bind([]any{token}),
		[]byte{
			sqinn.ValString, // user_id
			sqinn.ValInt64,  // created_at
			sqinn.ValInt64,  // expires_at
			sqinn.ValString, // name
			sqinn.ValString, // email
		},
	)
	if qerr != nil {
		return rec, "", false, fmt.Errorf("loading session: %w", qerr)
	}
	if len(rows) == 0 {
		// Absent row — cookie is stale; treat as guest.
		return rec, "", false, nil
	}
	row := rows[0]
	userID := row[0].String
	createdAt := row[1].Int64
	expiresAt := row[2].Int64
	name := row[3].String
	email := row[4].String

	if expiresAt > 0 && time.Now().Unix() > expiresAt {
		// Lazy expiry: drop the stale row in-band. A failing delete
		// is logged and ignored; the request still becomes a guest
		// and the next read will try the delete again.
		if cerr := s.CloseSession(context.Background(), token); cerr != nil {
			s.log.Warn("sessionstore: lazy expiry cleanup failed",
				slog.Any("err", cerr))
		}
		return rec, "", false, nil
	}

	rec = sessmanager.Record[app.SessionData]{
		UserID:   userID,
		IssuedAt: time.Unix(createdAt, 0),
		Data:     app.SessionData{Name: name, Email: email},
	}
	if expiresAt > 0 {
		// Surfaced to handlers, and enforced by datapages on every request.
		rec.ExpiresAt = time.Unix(expiresAt, 0)
	}
	return rec, token, true, nil
}

// CreateSession generates a new token and persists a row of
// (token, user_id, created_at, expires_at). The [app.Session]
// argument is ignored: Name and Email already live in the users
// table and are joined in on read.
func (s *Store) CreateSession(
	_ context.Context, rec sessmanager.Record[app.SessionData],
) (string, error) {
	if rec.UserID == "" {
		return "", errors.New("empty user id")
	}
	token, err := s.tokenGen.Generate()
	if err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	now := time.Now().Unix()
	var expiresAt int64
	if s.lifetime > 0 {
		expiresAt = now + int64(s.lifetime.Seconds())
	}
	if err := s.db.ExecParams(
		`INSERT INTO sessions (token, user_id, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		1, 4,
		sqinn.Bind([]any{token, rec.UserID, now, expiresAt}),
	); err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return token, nil
}

// CloseSession deletes the row and then fires any notifier callbacks
// that were registered against this token via NotifyClosed. Notifiers
// run *after* the DELETE so observers never see a "closed" session
// that still exists in the DB.
func (s *Store) CloseSession(_ context.Context, token string) error {
	if err := s.db.ExecParams(
		`DELETE FROM sessions WHERE token = ?`,
		1, 1,
		sqinn.Bind([]any{token}),
	); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	s.fireNotifiers(token)
	return nil
}

// NotifyClosed registers fn to be invoked when the session with the
// given token is closed (via [Store.CloseSession]). It is the hook the
// framework uses to wire per-session SSE teardown — when a user signs
// out on one tab, NotifyClosed lets the framework shut down every
// streaming response keyed to that session.
//
// Semantics follow the interface contract in [sessmanager]:
//
//   - If the session does not exist at call time, fn is invoked
//     immediately and the registration is skipped. This handles the
//     "subscribe to an already-closed session" race cleanly.
//   - If ctx is already canceled when we get here, we do nothing at
//     all — the caller has lost interest.
//   - Otherwise fn is stored and will fire inside CloseSession. A
//     background goroutine watches ctx and, on cancellation, garbage-
//     collects the notifier from the map so the store does not leak
//     references to abandoned subscribers.
func (s *Store) NotifyClosed(
	ctx context.Context, token string, fn func(),
) error {
	rows, err := s.db.QueryRows(
		`SELECT 1 FROM sessions WHERE token = ?`,
		sqinn.Bind([]any{token}),
		[]byte{sqinn.ValInt32},
	)
	if err != nil {
		return fmt.Errorf("probing session: %w", err)
	}
	if len(rows) == 0 {
		// Already gone — fire once and do not register.
		fn()
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}

	s.notifyLock.Lock()
	s.notify[token] = append(s.notify[token], notifier{ctx: ctx, fn: fn})
	s.notifyLock.Unlock()

	// Lifetime goroutine: when the subscriber's context is canceled,
	// scrub its notifier out of the map so the store does not hold a
	// stale closure forever. If no notifiers remain for the token we
	// also delete the map entry.
	go func() {
		<-ctx.Done()
		s.notifyLock.Lock()
		defer s.notifyLock.Unlock()
		kept := s.notify[token][:0]
		for _, n := range s.notify[token] {
			if n.ctx.Err() == nil {
				kept = append(kept, n)
			}
		}
		if len(kept) == 0 {
			delete(s.notify, token)
		} else {
			s.notify[token] = kept
		}
	}()
	return nil
}

// fireNotifiers drains the notifier list for token and invokes each
// fn whose context is still live. Called from CloseSession after the
// row has been deleted, so observers see the session as definitively
// gone by the time their callback runs.
func (s *Store) fireNotifiers(token string) {
	s.notifyLock.Lock()
	ns := s.notify[token]
	delete(s.notify, token)
	s.notifyLock.Unlock()
	for _, n := range ns {
		if n.ctx.Err() == nil {
			n.fn()
		}
	}
}
