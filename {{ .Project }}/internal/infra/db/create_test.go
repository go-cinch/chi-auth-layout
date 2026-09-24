package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestMissingDatabaseErrors(t *testing.T) {
	if !isMissingDatabase(&pq.Error{Code: "3D000"}, "postgres") || isMissingDatabase(errors.New("x"), "postgres") {
		t.Fatal("postgres missing database detection failed")
	}
	if isMissingDatabase(errors.New("x"), "sqlite") {
		t.Fatal("unsupported driver reported missing database")
	}
}

func TestAdminDSNsAndIdentifiers(t *testing.T) {
	admin, name, err := postgresAdminDSN("postgresql://root:password@localhost:5432/my%20db?sslmode=disable")
	if err != nil || name != "my db" || !strings.Contains(admin, "/postgres?") {
		t.Fatalf("postgresAdminDSN() = %q, %q, %v", admin, name, err)
	}
	for _, dsn := range []string{"host=localhost dbname=app", "postgresql://localhost", "postgresql://localhost/a/b"} {
		if _, _, err := postgresAdminDSN(dsn); err == nil {
			t.Fatalf("postgresAdminDSN(%q) succeeded", dsn)
		}
	}
	if quotePostgresIdentifier(`a"b`) != `"a""b"` {
		t.Fatal("identifier quoting failed")
	}
	if err := createDatabase(t.Context(), "sqlite", "dsn"); err == nil {
		t.Fatal("unsupported driver was accepted")
	}
	if err := createDatabase(t.Context(), "postgres", "invalid"); err == nil {
		t.Fatal("invalid PostgreSQL DSN was accepted")
	}
}
