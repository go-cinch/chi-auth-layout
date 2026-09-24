package db

import (
	"context"
	"database/sql"
	"fmt"
)

type contextTxKey struct{}

func (s *Store) SQL(ctx context.Context) SQLExecutor {
	if tx, ok := ctx.Value(contextTxKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.DB
}

func (s *Store) Tx(ctx context.Context, handler func(context.Context) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Ensure panic paths release the transaction; after completion this is a no-op.
	defer func() { _ = tx.Rollback() }()

	txCtx := context.WithValue(ctx, contextTxKey{}, tx)
	if err := handler(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("rollback transaction after %w: %v", err, rollbackErr)
		}
		return err
	}
	return tx.Commit()
}
