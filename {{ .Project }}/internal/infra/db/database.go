package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/redact"
	"github.com/lib/pq"

{{- if .Computed.enable_trace_final }}
	"github.com/XSAM/otelsql"
{{- end }}
)

type Store struct {
	DB *sql.DB
}

type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func New(ctx context.Context, cfg *config.Config) (*Store, func(), error) {
	logger := slog.Default()
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	if driver != "postgres" {
		return nil, nil, fmt.Errorf("unsupported database driver %q", driver)
	}
	dsn := strings.TrimSpace(cfg.Database.DSN)
	if dsn == "" {
		return nil, nil, errors.New("database dsn is required")
	}

{{- if .Computed.enable_trace_final }}
	var db *sql.DB
	var err error
	if cfg.Tracer.Enabled {
		db, err = otelsql.Open(driver, dsn)
	} else {
		db, err = sql.Open(driver, dsn)
	}
{{- else }}
	db, err := sql.Open(driver, dsn)
{{- end }}
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	if cfg.Database.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pingErr := db.PingContext(pingCtx)
	if pingErr != nil && cfg.Database.AutoCreate && isMissingDatabase(pingErr, driver) {
		if err := createDatabase(pingCtx, driver, dsn); err != nil {
			_ = db.Close()
			return nil, nil, err
		}
		pingErr = db.PingContext(pingCtx)
	}
	if pingErr != nil {
		_ = db.Close()
		return nil, nil, databaseConnectionError(pingErr, driver, dsn)
	}

	if cfg.Database.Migrate {
		if err := MigrateUp(ctx, db, driver); err != nil {
			_ = db.Close()
			return nil, nil, err
		}
	}

	cleanup := func() {
		if err := db.Close(); err != nil {
			logger.Error("close database failed: " + err.Error())
		}
	}
	logger.InfoContext(ctx, "database initialized: "+redact.DSN(dsn))
	return &Store{DB: db}, cleanup, nil
}

func databaseConnectionError(err error, driver, dsn string) error {
	var authenticationFailed bool
	switch driver {
	case "postgres":
		var target *pq.Error
		authenticationFailed = errors.As(err, &target) && target.Code == "28P01"
	}
	if authenticationFailed {
		return fmt.Errorf("%v; dsn: %s", err, redact.DSN(dsn))
	}
	return fmt.Errorf("ping database: %w", err)
}
