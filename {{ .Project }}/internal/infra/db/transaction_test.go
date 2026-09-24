package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestTransactions(t *testing.T) {
	state := &transactionState{}
	store := &Store{DB: openTransactionDB(t, state)}
	if store.SQL(t.Context()) != store.DB {
		t.Fatal("SQL() did not return the database")
	}
	if err := store.Tx(t.Context(), func(ctx context.Context) error {
		if _, ok := store.SQL(ctx).(*sql.Tx); !ok {
			t.Fatal("SQL() did not return the transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if state.commits != 1 {
		t.Fatalf("commits = %d", state.commits)
	}
	handlerErr := io.EOF
	if err := store.Tx(t.Context(), func(context.Context) error { return handlerErr }); !errors.Is(err, handlerErr) {
		t.Fatalf("handler error = %v", err)
	}
	if state.rollbacks != 1 {
		t.Fatalf("rollbacks = %d", state.rollbacks)
	}
	rollbackStore := &Store{DB: openTransactionDB(t, &transactionState{rollbackErr: errors.New("rollback failed")})}
	if err := rollbackStore.Tx(t.Context(), func(context.Context) error { return handlerErr }); err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("rollback error = %v", err)
	}
	beginStore := &Store{DB: openTransactionDB(t, &transactionState{beginErr: errors.New("begin failed")})}
	if err := beginStore.Tx(t.Context(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("begin error was ignored")
	}
	commitStore := &Store{DB: openTransactionDB(t, &transactionState{commitErr: errors.New("commit failed")})}
	if err := commitStore.Tx(t.Context(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("commit error was ignored")
	}
}

func TestTransactionPanicRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rollbackErr error
	}{
		{name: "successful rollback"},
		{name: "failed rollback", rollbackErr: errors.New("rollback failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &transactionState{rollbackErr: tc.rollbackErr}
			store := &Store{DB: openTransactionDB(t, state)}
			panicValue := errors.New("handler panicked")
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_ = store.Tx(context.Background(), func(context.Context) error {
					panic(panicValue)
				})
			}()
			if recovered != panicValue {
				t.Fatalf("panic = %v, want %v", recovered, panicValue)
			}
			if state.rollbacks != 1 || state.commits != 0 {
				t.Fatalf("rollbacks = %d, commits = %d; want 1, 0", state.rollbacks, state.commits)
			}
			if inUse := store.DB.Stats().InUse; inUse != 0 {
				t.Fatalf("connections still in use after panic = %d", inUse)
			}
		})
	}
}

type transactionDriver struct{ state *transactionState }
type transactionState struct {
	beginErr    error
	commitErr   error
	rollbackErr error
	commits     int
	rollbacks   int
}
type transactionConn struct{ state *transactionState }
type transactionTx struct{ state *transactionState }

func (d transactionDriver) Open(string) (driver.Conn, error) {
	return &transactionConn{state: d.state}, nil
}

func (*transactionConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }

func (*transactionConn) Close() error { return nil }

func (c *transactionConn) Begin() (driver.Tx, error) { return c.begin() }

func (c *transactionConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.begin()
}

func (c *transactionConn) begin() (driver.Tx, error) {
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	return &transactionTx{state: c.state}, nil
}

func (tx *transactionTx) Commit() error { tx.state.commits++; return tx.state.commitErr }

func (tx *transactionTx) Rollback() error { tx.state.rollbacks++; return tx.state.rollbackErr }

var transactionDriverSequence int

func openTransactionDB(t *testing.T, state *transactionState) *sql.DB {
	t.Helper()
	transactionDriverSequence++
	name := "chi-layout-transaction-" + strconv.Itoa(transactionDriverSequence)
	sql.Register(name, transactionDriver{state: state})
	database, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
