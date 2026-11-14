package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestMissingDatabaseErrors(t *testing.T) {
	if !isMissingDatabase(&pq.Error{Code: "3D000"}) || isMissingDatabase(errors.New("x")) {
		t.Fatal("postgres missing database detection failed")
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
	if err := createDatabase(t.Context(), "invalid"); err == nil {
		t.Fatal("invalid PostgreSQL DSN was accepted")
	}
}
