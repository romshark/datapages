// Package sessionstore is the main highlight of the sqlitesessions example:
// a SQLite-backed implementation of the framework's
// [sessions.Manager] interface for [app.Session].
//
// The sessions table stores the bare minimum: a token, the owning user id,
// and create/expire timestamps. Display fields like Name and Email stay in
// the users table and are joined in on read rather than copied into each session row.
package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"

	"github.com/romshark/datapages/example/sqlitesessions/app"
	"github.com/romshark/datapages/example/sqlitesessions/app/sqdb"
	"github.com/romshark/datapages/modules/sessions"
)

// Store implements [sessions.Manager] for [app.Session].
//
// sqdb.DB serializes the DB access, hence Store holds no DB lock.
// notifyLock guards the notifier map alone.
type Store struct {
	db       sqdb.DB
	tokenGen sessions.TokenGenerator
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

var _ sessions.Manager[app.SessionData] = (*Store)(nil)

// New creates the sessions table if missing and returns a Store to pass to
// [github.com/romshark/datapages.WithSessionManager].
//
// The schema declares `REFERENCES users(id) ON DELETE CASCADE`,
// which drops every session of a deleted user. It takes effect only on
// a connection with `PRAGMA foreign_keys = ON`.
// The users table must already exist, the userstore package creates it.
//
// lifetime is the maximum age of a session before [Store.ReadSessionFromCookie]
// treats it as expired. Pass 0 to disable expiry entirely. log defaults to
// [slog.Default] when nil.
func New(
	db sqdb.DB,
	tokenGen sessions.TokenGenerator,
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

// ReadSessionFromCookie resolves a session cookie in one round trip:
// the JOIN between sessions and users returns the persisted session fields and the
// current display fields together.
//
// Three returns say "no session":
//
//   - (zero, "", "", false, nil): the cookie is missing, the row is gone,
//     or the row was expired and just got cleaned up.
//     The request becomes a guest and the cookie is cleared.
//   - (zero, "", "", false, err): the DB errored. The request fails,
//     which keeps a transient DB outage from downgrading users to guests.
//   - (populated, token, userID, true, nil): the session is valid.
//
// A row past expires_at is dropped in band through [Store.CloseSession].
// A failing cleanup is logged and still reports ok=false, the next read retries it.
func (s *Store) ReadSessionFromCookie(cookieValue string) (
	rec sessions.Record[app.SessionData], token string, ok bool, err error,
) {
	if cookieValue == "" {
		return rec, "", false, nil
	}
	token = cookieValue

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
		// The cookie is stale, treat the request as a guest.
		return rec, "", false, nil
	}
	row := rows[0]
	userID := row[0].String
	createdAt := row[1].Int64
	expiresAt := row[2].Int64
	name := row[3].String
	email := row[4].String

	if expiresAt > 0 && time.Now().Unix() > expiresAt {
		if cerr := s.CloseSession(context.Background(), token); cerr != nil {
			s.log.Warn("sessionstore: lazy expiry cleanup failed",
				slog.Any("err", cerr))
		}
		return rec, "", false, nil
	}

	rec = sessions.Record[app.SessionData]{
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

// CreateSession generates a token and persists (token, user_id, created_at, expires_at).
// rec.Data is ignored: Name and Email live in the users table and are joined in on read.
func (s *Store) CreateSession(
	_ context.Context, rec sessions.Record[app.SessionData],
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

// CloseSession deletes the row and fires the notifiers registered for the
// token by [Store.NotifyClosed]. They run after the delete, which keeps an
// observer from seeing a closed session that is still in the DB.
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

// DeleteExpired deletes every session whose expires_at has passed and reports
// how many rows went. A session that never expires is stored with expires_at 0
// and is left alone.
//
// The tokens are read before the delete so the notifiers of each session can be fired,
// the way [Store.CloseSession] fires them for one.
func (s *Store) DeleteExpired(_ context.Context) (int, error) {
	now := time.Now().Unix()

	rows, err := s.db.QueryRows(
		`SELECT token FROM sessions WHERE expires_at != 0 AND expires_at <= ?`,
		sqinn.Bind([]any{now}),
		[]byte{sqinn.ValString},
	)
	if err != nil {
		return 0, fmt.Errorf("listing expired sessions: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	if err := s.db.ExecParams(
		`DELETE FROM sessions WHERE expires_at != 0 AND expires_at <= ?`,
		1, 1,
		sqinn.Bind([]any{now}),
	); err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}

	for _, row := range rows {
		s.fireNotifiers(row[0].String)
	}
	return len(rows), nil
}

// NotifyClosed registers fn to run when the session of token is closed.
// It is what tears down the SSE streams of a session: signing out on one tab
// ends every streaming response keyed to it.
//
// The contract of [sessions.CloseNotifier] resolves to three cases:
//
//   - The session no longer exists: fn runs immediately and nothing is registered,
//     which settles the race with an already-closed session.
//   - ctx is already canceled: nothing happens, the caller has lost interest.
//   - Otherwise fn is stored and fires in [Store.CloseSession] or [Store.DeleteExpired].
//     A goroutine drops it from the map once ctx is
//     canceled, which keeps the store from holding closures of abandoned subscribers.
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
		// Already gone: fire once and register nothing.
		fn()
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}

	s.notifyLock.Lock()
	s.notify[token] = append(s.notify[token], notifier{ctx: ctx, fn: fn})
	s.notifyLock.Unlock()

	// The map entry outlives the subscriber unless something drops it.
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

// fireNotifiers drains the notifier list of token and runs every fn whose
// context is still live. Callers delete the row first,
// which is what makes the session gone by the time a callback runs.
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
