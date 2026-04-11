// Package sqdb is a thin synchronized wrapper over a single sqinn-go v2 connection.
package sqdb

import (
	"fmt"
	"log/slog"
	"sync"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"
)

// DB is an abstraction over sqinn. *Conn satisfies it.
type DB interface {
	ExecSql(sql string) error
	ExecParams(sql string, niterations, nparams int, params []sqinn.Value) error
	QueryRows(sql string, params []sqinn.Value, coltypes []byte) ([][]sqinn.Value, error)
	WithTx(fn func(Tx) error) error
}

// Tx is the handle passed to WithTx callbacks. Transactions do not nest.
type Tx interface {
	ExecSql(sql string) error
	ExecParams(sql string, niterations, nparams int, params []sqinn.Value) error
	QueryRows(sql string, params []sqinn.Value, coltypes []byte) ([][]sqinn.Value, error)
}

// Conn serializes every call to the wrapped *sqinn.Sqinn so
// multi-statement transactions don't interleave with other callers.
type Conn struct {
	lock sync.Mutex
	sq   *sqinn.Sqinn
	log  *slog.Logger
}

var _ DB = (*Conn)(nil)

// New wraps sq with a Conn. If log is nil, slog.Default() is used.
func New(sq *sqinn.Sqinn, log *slog.Logger) *Conn {
	if log == nil {
		log = slog.Default()
	}
	return &Conn{sq: sq, log: log}
}

func (c *Conn) ExecSql(sql string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sq.ExecSql(sql)
}

func (c *Conn) ExecParams(
	sql string, niterations, nparams int, params []sqinn.Value,
) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sq.ExecParams(sql, niterations, nparams, params)
}

func (c *Conn) QueryRows(
	sql string, params []sqinn.Value, coltypes []byte,
) ([][]sqinn.Value, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sq.QueryRows(sql, params, coltypes)
}

// WithTx runs fn inside a SQLite transaction, holding the Conn mutex
// for the whole span. fn must use the passed Tx, not the outer Conn,
// or it will deadlock.
func (c *Conn) WithTx(fn func(Tx) error) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := c.sq.ExecSql("BEGIN"); err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rerr := c.sq.ExecSql("ROLLBACK"); rerr != nil {
				c.log.Warn("sqdb: rollback failed",
					slog.Any("err", rerr),
					slog.Any("original_err", err))
			}
			return
		}
		if cerr := c.sq.ExecSql("COMMIT"); cerr != nil {
			err = fmt.Errorf("commit: %w", cerr)
		}
	}()
	return fn(&tx{sq: c.sq})
}

// tx forwards directly to *sqinn.Sqinn without locking — WithTx
// already holds the mutex.
type tx struct{ sq *sqinn.Sqinn }

var _ Tx = (*tx)(nil)

func (t *tx) ExecSql(sql string) error { return t.sq.ExecSql(sql) }

func (t *tx) ExecParams(
	sql string, niterations, nparams int, params []sqinn.Value,
) error {
	return t.sq.ExecParams(sql, niterations, nparams, params)
}

func (t *tx) QueryRows(
	sql string, params []sqinn.Value, coltypes []byte,
) ([][]sqinn.Value, error) {
	return t.sq.QueryRows(sql, params, coltypes)
}
