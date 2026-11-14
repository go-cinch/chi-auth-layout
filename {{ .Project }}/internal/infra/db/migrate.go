package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
)

const migrationTable = "schema_migrations"

const SQLRoot = "migrations"

//go:embed migrations
var SQLFiles embed.FS

func MigrateUp(ctx context.Context, db *sql.DB) error {
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		return fmt.Errorf("list database migrations: %w", err)
	}
	if len(files) == 0 {
		return nil
	}

	if err := migrateRiver(ctx, db); err != nil {
		return err
	}

	source := migrate.EmbedFileSystemMigrationSource{
		FileSystem: SQLFiles,
		Root:       SQLRoot,
	}
	logger := slog.Default()
	migrate.SetTable(migrationTable)
	applied, err := migrate.ExecContext(ctx, db, postgresDriver, source, migrate.Up)
	if err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	logger.InfoContext(ctx, "database migrations applied: "+strconv.Itoa(applied), slog.String("operation", "database_migrate"), slog.String("outcome", "success"), slog.Int("applied", applied), slog.String("driver", postgresDriver))
	return nil
}
