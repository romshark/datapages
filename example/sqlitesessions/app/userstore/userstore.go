// Package userstore is a SQLite-backed user store using bcrypt for password hashing.
package userstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/romshark/datapages/example/sqlitesessions/app/sqdb"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailAlreadyInUse  = errors.New("email already in use")
)

type User struct {
	ID    string
	Name  string
	Email string
}

type Store struct {
	db sqdb.DB
}

// New creates the users table if missing.
func New(db sqdb.DB) (*Store, error) {
	if err := db.ExecSql(`CREATE TABLE IF NOT EXISTS users (
		id             TEXT PRIMARY KEY,
		email          TEXT UNIQUE NOT NULL,
		name           TEXT NOT NULL,
		password_hash  TEXT NOT NULL,
		created_at     INTEGER NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("initializing users schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Register returns ErrEmailAlreadyInUse if the email is taken.
func (s *Store) Register(_ context.Context, name, email, password string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	if email == "" || password == "" || name == "" {
		return User{}, ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hashing password: %w", err)
	}
	id := newUserID()

	// Wrap the pre-check and insert in a tx so another goroutine can't
	// slip between them.
	err = s.db.WithTx(func(tx sqdb.Tx) error {
		rows, err := tx.QueryRows(
			`SELECT 1 FROM users WHERE email = ?`,
			sqinn.Bind([]any{email}),
			[]byte{sqinn.ValInt32},
		)
		if err != nil {
			return fmt.Errorf("checking email uniqueness: %w", err)
		}
		if len(rows) > 0 {
			return ErrEmailAlreadyInUse
		}
		if err := tx.ExecParams(
			`INSERT INTO users (id, email, name, password_hash, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			1, 5,
			sqinn.Bind([]any{id, email, name, string(hash), time.Now().Unix()}),
		); err != nil {
			return fmt.Errorf("inserting user: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Name: name, Email: email}, nil
}

// ListUsers is for the demo home page only; a real app would never
// expose this.
func (s *Store) ListUsers(_ context.Context) ([]User, error) {
	rows, err := s.db.QueryRows(
		`SELECT id, name, email FROM users ORDER BY created_at ASC`,
		nil,
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValString},
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	users := make([]User, 0, len(rows))
	for _, row := range rows {
		users = append(users, User{
			ID:    row[0].String,
			Name:  row[1].String,
			Email: row[2].String,
		})
	}
	return users, nil
}

func (s *Store) Authenticate(_ context.Context, email, password string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	rows, err := s.db.QueryRows(
		`SELECT id, name, password_hash FROM users WHERE email = ?`,
		sqinn.Bind([]any{email}),
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValString},
	)
	if err != nil {
		return User{}, fmt.Errorf("loading user: %w", err)
	}
	if len(rows) == 0 {
		return User{}, ErrInvalidCredentials
	}
	id := rows[0][0].String
	name := rows[0][1].String
	hash := rows[0][2].String
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return User{ID: id, Name: name, Email: email}, nil
}

func newUserID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
