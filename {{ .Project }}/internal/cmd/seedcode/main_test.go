package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteSeedCodesPreservesReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default-data.sql")
	original := `INSERT INTO t_action (code) VALUES ('OLDCODEA'), ('OLDCODEB');
INSERT INTO t_role (action) VALUES ('OLDCODEA,OLDCODEB');
INSERT INTO t_user (code) VALUES ('USERCODE');
`
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}

	values := []string{"AAAAAAAA", "BBBBBBBB", "23456789"}
	generate := func() (string, error) {
		value := values[0]
		values = values[1:]
		return value, nil
	}
	count, err := rewriteSeedCodes(path, generate)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("rewritten codes = %d, want 3", count)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	if strings.Contains(got, "OLDCODEA") || strings.Contains(got, "OLDCODEB") || strings.Contains(got, "USERCODE") {
		t.Fatalf("rewritten migration retains placeholders:\n%s", got)
	}
	if strings.Count(got, "AAAAAAAA") != 2 || strings.Count(got, "BBBBBBBB") != 2 || strings.Count(got, "23456789") != 1 {
		t.Fatalf("rewritten references are inconsistent:\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("migration mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestRewriteSeedCodesErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := rewriteSeedCodes(filepath.Join(t.TempDir(), "missing.sql"), func() (string, error) { return "AAAAAAAA", nil }); err == nil {
			t.Fatal("expected read error")
		}
	})

	t.Run("no codes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.sql")
		if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := rewriteSeedCodes(path, func() (string, error) { return "AAAAAAAA", nil }); err == nil {
			t.Fatal("expected no-code error")
		}
	})

	t.Run("generator failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "default-data.sql")
		if err := os.WriteFile(path, []byte("'OLD00001'\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		want := errors.New("random source failed")
		if _, err := rewriteSeedCodes(path, func() (string, error) { return "", want }); !errors.Is(err, want) {
			t.Fatalf("error = %v, want wrapped generator failure", err)
		}
	})

	t.Run("invalid generated code", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "default-data.sql")
		if err := os.WriteFile(path, []byte("'OLD00001'\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := rewriteSeedCodes(path, func() (string, error) { return "ABC12345", nil }); err == nil {
			t.Fatal("expected invalid-code error")
		}
	})
}

func TestNextUniqueCodeExhaustion(t *testing.T) {
	used := map[string]struct{}{"AAAAAAAA": {}}
	if _, err := nextUniqueCode(func() (string, error) { return "AAAAAAAA", nil }, used); err == nil {
		t.Fatal("expected unique-code exhaustion")
	}
}

func TestUniqueCodesPreservesFirstOccurrenceOrder(t *testing.T) {
	got := uniqueCodes([]string{"BBBBBBBB", "AAAAAAAA", "BBBBBBBB"})
	if len(got) != 2 || got[0] != "BBBBBBBB" || got[1] != "AAAAAAAA" {
		t.Fatalf("unique codes = %#v", got)
	}
}
