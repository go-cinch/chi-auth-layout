package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/lib/pq"
)

func isMissingDatabase(err error) bool {
	var target *pq.Error
	return errors.As(err, &target) && target.Code == "3D000"
}

func createDatabase(ctx context.Context, dsn string) error {
	adminDSN, name, err := postgresAdminDSN(dsn)
	if err != nil {
		return err
	}

	admin, err := sql.Open(postgresDriver, adminDSN)
	if err != nil {
		return fmt.Errorf("open database server: %w", err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quotePostgresIdentifier(name)); err != nil {
		var target *pq.Error
		if !errors.As(err, &target) || target.Code != "42P04" {
			return fmt.Errorf("create database: %w", err)
		}
	}
	slog.InfoContext(ctx, "database created: "+name, slog.String("operation", "database_create"), slog.String("outcome", "success"), slog.String("database", name), slog.String("driver", postgresDriver))
	return nil
}

func postgresAdminDSN(dsn string) (string, string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", "", fmt.Errorf("parse postgresql dsn: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", "", errors.New("automatic postgresql database creation requires a postgres url dsn")
	}
	escapedName := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if escapedName == "" || strings.Contains(escapedName, "/") {
		return "", "", errors.New("postgresql dsn must contain one database name")
	}
	name, err := url.PathUnescape(escapedName)
	if err != nil {
		return "", "", fmt.Errorf("decode postgresql database name: %w", err)
	}
	parsed.Path = "/postgres"
	parsed.RawPath = ""
	return parsed.String(), name, nil
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
