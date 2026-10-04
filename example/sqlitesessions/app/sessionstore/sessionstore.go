// Package sessionstore is the main highlight of the sqlitesessions example:
// a SQLite-backed implementation of the framework's
// [sessions.Manager] interface for [app.Session].
//
// The sessions table stores the bare minimum: the SHA-256 of the token, the owning
// user id, and create/expire timestamps. Display fields like Name and Email stay in
// the users table and are joined in on read rather than copied into each session row.
package sessionstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
// notifyLock guards the notifier map. [Store.NotifyClosed] also holds it
// across its probe of the session row.
type Store struct {
	db       sqdb.DB
	tokenGen sessions.TokenGenerator
	lifetime time.Duration
	log      *slog.Logger

	notifyLock sync.Mutex
	notify     map[string][]notifier // by token hash
}

// notifier is a registered NotifyClosed callback paired with the
// context that bounds its lifetime.
type notifier struct {
	ctx context.Context
	fn  func()
}

var _ sessions.Manager[app.SessionData] = (*Store)(nil)

// New creates the sessions table if missing and returns a Store to pass to
// [github.com/romshark/datapages.WithSessionManager]. It replaces a sessions
// table that has no token_hash column, which ends the sessions in it.
//
// The schema declares `REFERENCES users(id) ON DELETE CASCADE`,
// which drops every session of a deleted user. It takes effect only on
// a connection with `PRAGMA foreign_keys = ON`.
// The users table must already exist, the userstore package creates it.
//
// lifetime is the maximum age of a session. A session whose record asks for
// an earlier ExpiresAt expires at that time instead. With lifetime 0 the record
// alone decides, and a zero ExpiresAt never expires. log defaults to
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

	// A table created before the store hashed tokens has no token_hash column
	// and holds raw tokens. CREATE TABLE IF NOT EXISTS would keep it, and every
	// session query would fail on it. Dropping it signs its users out.
	rows, err := db.QueryRows(
		`SELECT 1 FROM pragma_table_info('sessions') WHERE name = 'token_hash'`,
		nil, []byte{sqinn.ValInt32},
	)
	if err != nil {
		return nil, fmt.Errorf("reading sessions schema: %w", err)
	}
	if len(rows) == 0 {
		if err := db.ExecSql(`DROP TABLE IF EXISTS sessions`); err != nil {
			return nil, fmt.Errorf("dropping sessions table: %w", err)
		}
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash  TEXT PRIMARY KEY,
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
// The result has three forms:
//
//   - (zero, "", false, nil): the cookie is missing, the row is gone,
//     or the row was expired and just got cleaned up.
//     The request becomes a guest and the cookie is cleared.
//   - (zero, "", false, err): the DB errored. The request fails,
//     which keeps a transient DB outage from downgrading users to guests.
//   - (populated, token, true, nil): the session is valid. rec.UserID identifies
//     the user.
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
		 WHERE s.token_hash = ?`,
		sqinn.Bind([]any{hashToken(token)}),
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

// CreateSession generates a token and persists its hash with the user id and
// the create and expire times. The session expires at the sooner of
// rec.ExpiresAt and the lifetime of the store.
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
	if err := s.db.ExecParams(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		1, 4,
		sqinn.Bind([]any{
			hashToken(token), rec.UserID, now, s.expiresAt(now, rec.ExpiresAt),
		}),
	); err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return token, nil
}

// expiresAt returns the expires_at of a session created at now, in Unix seconds:
// the sooner of asked and now plus the lifetime. A zero asked and
// a zero lifetime set no limit, and 0 is a session that never expires.
func (s *Store) expiresAt(now int64, asked time.Time) int64 {
	var exp int64
	if s.lifetime > 0 {
		exp = now + int64(s.lifetime.Seconds())
	}
	if !asked.IsZero() {
		// 0 stands for no expiry, and an ExpiresAt at or before the epoch has passed.
		a := max(asked.Unix(), 1)
		if exp == 0 || a < exp {
			exp = a
		}
	}
	return exp
}

// hashToken returns the key a session is stored and notified under:
// the hex SHA-256 of its token. The table never holds the token itself,
// which keeps read access to the database file from yielding a working cookie.
//
// A salt or a slow hash, as for a password, would add nothing: both raise the
// cost of guessing a secret of low entropy, and a [sessions.TokenGenerator]
// token is random.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CloseSession deletes the row and fires the notifiers registered for the
// token by [Store.NotifyClosed]. They run after the delete, which keeps an
// observer from seeing a closed session that is still in the DB.
func (s *Store) CloseSession(_ context.Context, token string) error {
	key := hashToken(token)
	if err := s.db.ExecParams(
		`DELETE FROM sessions WHERE token_hash = ?`,
		1, 1,
		sqinn.Bind([]any{key}),
	); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	s.fireNotifiers(key)
	return nil
}

// DeleteExpired deletes every session whose expires_at has passed and reports
// how many rows went. A session that never expires is stored with expires_at 0
// and is left alone.
//
// The token hashes are read before the delete so the notifiers of each session
// can be fired, the way [Store.CloseSession] fires them for one.
func (s *Store) DeleteExpired(_ context.Context) (int, error) {
	now := time.Now().Unix()

	rows, err := s.db.QueryRows(
		`SELECT token_hash FROM sessions WHERE expires_at != 0 AND expires_at <= ?`,
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
	gone, err := s.addNotifier(ctx, hashToken(token), fn)
	if err != nil {
		return err
	}
	if gone {
		fn()
	}
	return nil
}

// addNotifier stores fn under key unless the session row is gone or ctx is canceled,
// and reports whether the row is gone. It leaves running fn for a gone row to
// the caller: fn may call back into the store, which deadlocks under notifyLock.
//
// It holds notifyLock across the probe and the store of fn. A close deletes the row
// before it takes the lock to fire the notifiers: either the probe misses the row,
// or the close finds fn. A close that fired between the probe and the store would
// leave fn registered for a session that no longer exists, and nothing would run it.
func (s *Store) addNotifier(
	ctx context.Context, key string, fn func(),
) (gone bool, err error) {
	s.notifyLock.Lock()
	defer s.notifyLock.Unlock()

	rows, err := s.db.QueryRows(
		`SELECT 1 FROM sessions WHERE token_hash = ?`,
		sqinn.Bind([]any{key}),
		[]byte{sqinn.ValInt32},
	)
	if err != nil {
		return false, fmt.Errorf("probing session: %w", err)
	}
	if len(rows) == 0 {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, nil
	}
	s.notify[key] = append(s.notify[key], notifier{ctx: ctx, fn: fn})

	// The map entry outlives the subscriber unless something drops it.
	go func() {
		<-ctx.Done()
		s.notifyLock.Lock()
		defer s.notifyLock.Unlock()
		kept := s.notify[key][:0]
		for _, n := range s.notify[key] {
			if n.ctx.Err() == nil {
				kept = append(kept, n)
			}
		}
		if len(kept) == 0 {
			delete(s.notify, key)
		} else {
			s.notify[key] = kept
		}
	}()
	return false, nil
}

// fireNotifiers drains the notifier list of key and runs every fn whose
// context is still live. Callers delete the row first,
// which is what makes the session gone by the time a callback runs.
func (s *Store) fireNotifiers(key string) {
	s.notifyLock.Lock()
	ns := s.notify[key]
	delete(s.notify, key)
	s.notifyLock.Unlock()
	for _, n := range ns {
		if n.ctx.Err() == nil {
			n.fn()
		}
	}
}
