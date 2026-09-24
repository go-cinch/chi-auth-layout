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

func isMissingDatabase(err error, driver string) bool {
	switch driver {
	case "postgres":
		var target *pq.Error
		return errors.As(err, &target) && target.Code == "3D000"
	default:
		return false
	}
}

func createDatabase(ctx context.Context, driver, dsn string) error {
	var adminDSN string
	var name string
	var statement string
	var err error
	switch driver {
	case "postgres":
		adminDSN, name, err = postgresAdminDSN(dsn)
		statement = "CREATE DATABASE " + quotePostgresIdentifier(name)
	default:
		return fmt.Errorf("unsupported database driver %q", driver)
	}
	if err != nil {
		return err
	}

	admin, err := sql.Open(driver, adminDSN)
	if err != nil {
		return fmt.Errorf("open database server: %w", err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, statement); err != nil {
		var target *pq.Error
		if driver != "postgres" || !errors.As(err, &target) || target.Code != "42P04" {
			return fmt.Errorf("create database: %w", err)
		}
	}
	slog.InfoContext(ctx, "database created: "+name)
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
