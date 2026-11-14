package tests

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func TestTOTPBaselineSchemaConstraints(t *testing.T) {
	store := permissionDatabase(t)
	var retiredColumns int
	if err := store.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND ((table_name = 't_user_totp' AND column_name = 'enabled_at') OR (table_name = 't_user_recovery_code' AND column_name = 'used_at'))`).Scan(&retiredColumns); err != nil || retiredColumns != 0 {
		t.Fatal("retired Verification state remains in the baseline", retiredColumns, err)
	}
	assertRejected := func(code, query string, args ...any) {
		t.Helper()
		_, err := store.DB.ExecContext(t.Context(), query, args...)
		var databaseError *pq.Error
		if !errors.As(err, &databaseError) || string(databaseError.Code) != code {
			t.Fatalf("expected SQLSTATE %s, got %v", code, err)
		}
	}
	// A persisted factor has already consumed its enrollment code. There is no
	// unverified/default counter state in PostgreSQL.
	assertRejected("23502", `INSERT INTO t_user_totp (user_id, secret_ciphertext, period_seconds) VALUES (2, 'fixture', 30)`)
	assertRejected("23502", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step) VALUES (2, 'fixture', 0)`)
	for _, period := range []int64{0, -1, 9223372037} {
		assertRejected("23514", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (2, 'fixture', 0, $1)`, period)
	}
	assertRejected("23514", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (2, 'fixture', -1, 30)`)
	assertRejected("23514", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (2, '', 0, 30)`)
	assertRejected("23503", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (999999, 'fixture', 0, 30)`)
	if _, err := store.DB.ExecContext(t.Context(), `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (2, 'fixture', 0, 30)`); err != nil {
		t.Fatal(err)
	}
	assertRejected("23505", `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES (2, 'another-fixture', 1, 30)`)
	assertRejected("23514", `INSERT INTO t_user_recovery_code (user_id, code_hash) VALUES (2, 'plaintext-recovery-code')`)
	hash := strings.Repeat("a", 64)
	if _, err := store.DB.ExecContext(t.Context(), `INSERT INTO t_user_recovery_code (user_id, code_hash) VALUES (2, $1)`, hash); err != nil {
		t.Fatal(err)
	}
	assertRejected("23505", `INSERT INTO t_user_recovery_code (user_id, code_hash) VALUES (2, $1)`, hash)
	assertRejected("23503", `INSERT INTO t_user_recovery_code (user_id, code_hash) VALUES (999999, $1)`, hash)
	if _, err := store.DB.ExecContext(t.Context(), `DELETE FROM t_user WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := store.DB.QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM t_user_totp WHERE user_id = 2) + (SELECT COUNT(*) FROM t_user_recovery_code WHERE user_id = 2)`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("deleting a user left Verification credentials", remaining, err)
	}
}

func TestTOTPBaselineMigrationRoundTrip(t *testing.T) {
	store := permissionDatabase(t)
	files, err := fs.Glob(db.SQLFiles, db.SQLRoot+"/*.sql")
	if err != nil || len(files) != 5 {
		t.Fatal("expected the auth baseline migrations", files, err)
	}
	for i := len(files) - 1; i >= 0; i-- {
		content, err := db.SQLFiles.ReadFile(files[i])
		if err != nil {
			t.Fatal(err)
		}
		_, down, found := strings.Cut(string(content), "-- +migrate Down")
		if !found {
			t.Fatal("migration missing Down", files[i])
		}
		if _, err := store.DB.ExecContext(t.Context(), down); err != nil {
			t.Fatalf("roll back %s: %v", files[i], err)
		}
	}
	var remaining int
	if err := store.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name LIKE 't_%'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("baseline rollback left business tables", remaining, err)
	}
	if _, err := store.DB.ExecContext(t.Context(), `DROP TABLE schema_migrations`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.MigrateUp(t.Context(), store.DB); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM schema_migrations`).Scan(&remaining); err != nil || remaining != len(files) {
		t.Fatal("baseline migration history is not stable", remaining, err)
	}
}
