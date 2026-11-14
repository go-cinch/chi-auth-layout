package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// Serialize each River migration across replicas. PostgreSQL requires a commit
// between adding an enum value and using it in a subsequent migration.
func migrateRiver(ctx context.Context, database *sql.DB) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(database), nil)
	if err != nil {
		return err
	}
	store := Store{DB: database}
	for {
		var done bool
		err := store.Tx(ctx, func(ctx context.Context) error {
			tx := store.SQL(ctx).(*sql.Tx)
			if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1735290211, 5)"); err != nil {
				return err
			}
			result, err := migrator.MigrateTx(ctx, tx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{MaxSteps: 1})
			if err == nil {
				done = len(result.Versions) == 0
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate River: %w", err)
		}
		if done {
			return nil
		}
	}
}
