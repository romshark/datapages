package sessionstore_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/sqlitesessions/app"
	"github.com/romshark/datapages/example/sqlitesessions/app/sessionstore"
	"github.com/romshark/datapages/example/sqlitesessions/app/sqdb"
	"github.com/romshark/datapages/example/sqlitesessions/app/userstore"
	"github.com/romshark/datapages/modules/sessions"
)

// TestCreateSessionExpiresAt tests that a session expires at the sooner of
// the ExpiresAt of its record and the lifetime of the store.
func TestCreateSessionExpiresAt(t *testing.T) {
	const (
		quarterHour = 15 * time.Minute
		week        = 7 * 24 * time.Hour
	)
	db, _ := newDB(t)
	userID := addUser(t, db)

	for name, tt := range map[string]struct {
		lifetime time.Duration
		asked    time.Duration // ExpiresAt of the record after creation, 0 for none
		want     time.Duration // expiry after creation, 0 for none
	}{
		"record sooner":   {lifetime: week, asked: quarterHour, want: quarterHour},
		"lifetime sooner": {lifetime: time.Hour, asked: week, want: time.Hour},
		"record only":     {asked: quarterHour, want: quarterHour},
		"lifetime only":   {lifetime: time.Hour, want: time.Hour},
		"neither":         {},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t, db, tt.lifetime)
			before := time.Now()
			var asked time.Time
			if tt.asked != 0 {
				asked = before.Add(tt.asked)
			}
			token := createSession(t, s, userID, asked)
			after := time.Now()

			rec, _, ok, err := s.ReadSessionFromCookie(token)
			require.NoError(t, err)
			require.True(t, ok)
			if tt.want == 0 {
				require.Zero(t, rec.ExpiresAt)
				return
			}
			// The store keeps whole seconds and reads the clock between before and after.
			got := rec.ExpiresAt.Unix()
			require.GreaterOrEqual(t, got, before.Add(tt.want).Unix(),
				"ExpiresAt %s", rec.ExpiresAt)
			require.LessOrEqual(t, got, after.Add(tt.want).Unix(),
				"ExpiresAt %s", rec.ExpiresAt)
		})
	}
}

// TestReadSessionPassedExpiresAt tests that a session is refused once the
// ExpiresAt of its record has passed, though the lifetime of the store has not.
func TestReadSessionPassedExpiresAt(t *testing.T) {
	db, _ := newDB(t)
	userID := addUser(t, db)
	s := newStore(t, db, 7*24*time.Hour)

	for name, tt := range map[string]struct {
		asked  time.Time
		wantOK bool
	}{
		"future": {asked: time.Now().Add(time.Hour), wantOK: true},
		"passed": {asked: time.Now().Add(-time.Minute)},
		"epoch":  {asked: time.Unix(0, 0)},
	} {
		t.Run(name, func(t *testing.T) {
			token := createSession(t, s, userID, tt.asked)
			_, _, ok, err := s.ReadSessionFromCookie(token)
			require.NoError(t, err)
			require.Equal(t, tt.wantOK, ok, "ExpiresAt %s", tt.asked)
		})
	}
}

// TestDatabaseHoldsNoToken tests that no database file holds a session token,
// and that the token still reads and closes its session.
func TestDatabaseHoldsNoToken(t *testing.T) {
	db, dir := newDB(t)
	userID := addUser(t, db)
	s := newStore(t, db, time.Hour)
	token := createSession(t, s, userID, time.Time{})

	rec, got, ok, err := s.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, token, got)
	require.Equal(t, userID, rec.UserID)

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	holdsUserID := false
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		require.NoError(t, err)
		require.False(t, bytes.Contains(b, []byte(token)),
			"%s holds the session token %q", f.Name(), token)
		holdsUserID = holdsUserID || bytes.Contains(b, []byte(userID))
	}
	// The session row holds the user ID too. Finding it shows that the scan
	// reads the files the rows are written to.
	require.True(t, holdsUserID, "no file in %s holds the user ID %q", dir, userID)

	require.NoError(t, s.CloseSession(t.Context(), token))
	_, _, ok, err = s.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.False(t, ok, "the closed session still reads")
}

// TestNewReplacesTableWithoutTokenHash tests that New replaces a sessions
// table without a token_hash column, which ends the sessions in it,
// and keeps a table that has one.
func TestNewReplacesTableWithoutTokenHash(t *testing.T) {
	db, _ := newDB(t)
	userID := addUser(t, db)
	// The schema of the store before it hashed tokens.
	require.NoError(t, db.ExecSql(`CREATE TABLE sessions (
		token       TEXT PRIMARY KEY,
		user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at  INTEGER NOT NULL,
		expires_at  INTEGER NOT NULL
	)`))
	require.NoError(t, db.ExecSql(
		`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,
	))
	const oldToken = "old-token"
	require.NoError(t, db.ExecParams(
		`INSERT INTO sessions (token, user_id, created_at, expires_at)
		 VALUES (?, ?, ?, 0)`,
		1, 3,
		sqinn.Bind([]any{oldToken, userID, time.Now().Unix()}),
	))

	s := newStore(t, db, time.Hour)
	_, _, ok, err := s.ReadSessionFromCookie(oldToken)
	require.NoError(t, err)
	require.False(t, ok, "a session of the old table still reads")

	token := createSession(t, s, userID, time.Time{})
	_, _, ok, err = s.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok)

	// A restart keeps the table and the sessions in it.
	s = newStore(t, db, time.Hour)
	_, _, ok, err = s.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok, "New replaced a table that has token_hash")
}

// TestNotifyClosed tests that NotifyClosed runs fn once when the session of
// the token is closed or deleted as expired, and right away when it is gone.
func TestNotifyClosed(t *testing.T) {
	closeSession := func(t *testing.T, s *sessionstore.Store, token string) {
		require.NoError(t, s.CloseSession(t.Context(), token))
	}
	deleteExpired := func(t *testing.T, s *sessionstore.Store, _ string) {
		n, err := s.DeleteExpired(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, n, "deleted sessions")
	}
	for name, tt := range map[string]struct {
		// before closes the session before NotifyClosed, after closes it after.
		before, after func(*testing.T, *sessionstore.Store, string)
	}{
		"close":  {after: closeSession},
		"expire": {after: deleteExpired},
		"gone":   {before: closeSession},
	} {
		t.Run(name, func(t *testing.T) {
			db, _ := newDB(t)
			userID := addUser(t, db)
			s := newStore(t, db, time.Hour)
			// Expired for DeleteExpired to collect. NotifyClosed and
			// CloseSession act on a row regardless of its expiry.
			token := createSession(t, s, userID, time.Now().Add(-time.Minute))
			if tt.before != nil {
				tt.before(t, s, token)
			}

			var runs atomic.Int32
			require.NoError(t, s.NotifyClosed(
				t.Context(), token, func() { runs.Add(1) },
			))
			if tt.after != nil {
				require.Zero(t, runs.Load(), "runs of fn before the session closed")
				tt.after(t, s, token)
			}
			require.Equal(t, int32(1), runs.Load(), "runs of fn")
		})
	}
}

// TestNotifyClosedCloseDuringProbe tests that fn runs when CloseSession deletes
// the session after NotifyClosed has found it and before fn is registered.
func TestNotifyClosedCloseDuringProbe(t *testing.T) {
	conn, _ := newDB(t)
	db := &interceptDB{DB: conn}
	userID := addUser(t, db)
	s := newStore(t, db, time.Hour)
	token := createSession(t, s, userID, time.Time{})

	var closeErr error
	closeDone := make(chan struct{})
	deleted := make(chan struct{})
	db.afterExec = sync.OnceFunc(func() { close(deleted) })
	// The probe of NotifyClosed is the next query.
	db.afterQuery = func() {
		db.afterQuery = nil
		go func() {
			closeErr = s.CloseSession(t.Context(), token)
			close(closeDone)
		}()
		<-deleted
		// The row is gone and CloseSession goes on to fire the notifiers, which
		// takes microseconds unless the store holds it back until fn is registered.
		select {
		case <-closeDone:
		case <-time.After(100 * time.Millisecond):
		}
	}

	var ran atomic.Bool
	require.NoError(t, s.NotifyClosed(t.Context(), token, func() { ran.Store(true) }))
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "CloseSession did not return")
	}
	require.NoError(t, closeErr)
	require.True(t, ran.Load(),
		"fn did not run for a session closed during the probe of NotifyClosed")
}

// interceptDB passes every call to the DB it embeds and runs afterQuery and
// afterExec, when set, once the query or the exec has returned.
type interceptDB struct {
	sqdb.DB
	afterQuery func()
	afterExec  func()
}

func (d *interceptDB) QueryRows(
	sql string, params []sqinn.Value, coltypes []byte,
) ([][]sqinn.Value, error) {
	rows, err := d.DB.QueryRows(sql, params, coltypes)
	if d.afterQuery != nil {
		d.afterQuery()
	}
	return rows, err
}

func (d *interceptDB) ExecParams(
	sql string, niterations, nparams int, params []sqinn.Value,
) error {
	err := d.DB.ExecParams(sql, niterations, nparams, params)
	if d.afterExec != nil {
		d.afterExec()
	}
	return err
}

// newDB launches sqinn on a database file in a temporary directory and returns
// the connection and the directory. An empty SQINN_PATH selects the binary
// embedded in sqinn-go, as in cmd/server.
func newDB(t *testing.T) (*sqdb.Conn, string) {
	t.Helper()
	dir := t.TempDir()
	sq, err := sqinn.Launch(sqinn.Options{
		Sqinn: os.Getenv("SQINN_PATH"),
		Db:    filepath.Join(dir, "test.db"),
	})
	require.NoError(t, err, "launching sqinn")
	t.Cleanup(func() { assert.NoError(t, sq.Close(), "closing sqinn") })
	return sqdb.New(sq, nil), dir
}

// addUser creates the users table the sessions table refers to and
// registers a user in it. It returns the ID of the user.
func addUser(t *testing.T, db sqdb.DB) string {
	t.Helper()
	users, err := userstore.New(db)
	require.NoError(t, err)
	u, err := users.Register(t.Context(), "Ada", "ada@example.com", "password123")
	require.NoError(t, err)
	return u.ID
}

func newStore(t *testing.T, db sqdb.DB, lifetime time.Duration) *sessionstore.Store {
	t.Helper()
	s, err := sessionstore.New(db, sessions.DefaultTokenGenerator{}, lifetime, nil)
	require.NoError(t, err)
	return s
}

func createSession(
	t *testing.T, s *sessionstore.Store, userID string, expiresAt time.Time,
) string {
	t.Helper()
	token, err := s.CreateSession(t.Context(), sessions.Record[app.SessionData]{
		UserID:    userID,
		IssuedAt:  time.Now(),
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	return token
}
