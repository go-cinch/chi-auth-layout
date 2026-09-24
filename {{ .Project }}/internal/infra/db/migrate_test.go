package db

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestEmbeddedMigrations(t *testing.T) {
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("required auth migrations missing: %v", files)
	}
	for _, name := range files {
		contents, err := SQLFiles.ReadFile(name)
		if err != nil {
			t.Fatalf("read embedded migration %q: %v", name, err)
		}
		if len(contents) == 0 {
			t.Fatalf("embedded migration %q is empty", name)
		}
	}
}

func TestMigrateUpPreservesDatabaseError(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	failure := errors.New("migration table unavailable")
	mock.ExpectExec(`(?i)create table if not exists "schema_migrations"`).WillReturnError(failure)
	if err := MigrateUp(t.Context(), database, "postgres"); !errors.Is(err, failure) {
		t.Fatalf("migration database failure = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateUpDoesNotReplayAppliedMigrations(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"id", "applied_at"})
	for _, name := range files {
		rows.AddRow(path.Base(name), time.Unix(1, 0))
	}
	mock.ExpectExec(`(?i)create table if not exists "schema_migrations"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "schema_migrations"`).WillReturnRows(rows)
	if err := MigrateUp(t.Context(), database, "postgres"); err != nil {
		t.Fatalf("already migrated database: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultUserPasswordsMatchUsernames(t *testing.T) {
	names, err := fs.Glob(SQLFiles, SQLRoot+"/*-02-auth-default-data.sql")
	if err != nil || len(names) != 1 {
		t.Fatalf("default data migration = %#v, %v", names, err)
	}
	contents, err := SQLFiles.ReadFile(names[0])
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^\s*\(\d+,\s*(?:\d+|NULL),\s*'([^']+)',\s*'[^']+',\s*'([^']+)',\s*1\)[,;]$`)
	matches := pattern.FindAllSubmatch(contents, -1)
	if len(matches) != 5 {
		t.Fatalf("seed users = %d, want 5", len(matches))
	}
	for _, match := range matches {
		username, passwordHash := string(match[1]), match[2]
		if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(username)); err != nil {
			t.Fatalf("seed password for %q does not match its username: %v", username, err)
		}
	}
}
