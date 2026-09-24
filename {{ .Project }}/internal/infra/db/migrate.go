package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
)

const migrationTable = "schema_migrations"

func MigrateUp(ctx context.Context, db *sql.DB, dialect string) error {
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		return fmt.Errorf("list database migrations: %w", err)
	}
	if len(files) == 0 {
		return nil
	}

	source := migrate.EmbedFileSystemMigrationSource{
		FileSystem: SQLFiles,
		Root:       SQLRoot,
	}
	logger := slog.Default()
	migrate.SetTable(migrationTable)
	applied, err := migrate.ExecContext(ctx, db, dialect, source, migrate.Up)
	if err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	logger.InfoContext(ctx, "database migrations applied: "+strconv.Itoa(applied))
	return nil
}
