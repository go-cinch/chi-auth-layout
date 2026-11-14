package genfile_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/genfile"
)

func captureOutput(t *testing.T, run func() error) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		_ = reader.Close()
		_ = writer.Close()
	}()
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestWriteCreatesUpdatesAndPreservesUnchangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated", "output.go")
	write := func(source string) string {
		return captureOutput(t, func() error { return genfile.Write(path, []byte(source)) })
	}
	if output := write("old\n"); output != "create "+path+"\n" {
		t.Fatalf("creation report = %q", output)
	}
	if output := write("new\n"); !strings.Contains(output, "--- "+path) || !strings.Contains(output, "-old\n+new\n") {
		t.Fatalf("update diff = %q", output)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("updated content = %q, %v", content, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("generated file permissions: %v, %v", info, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1234567890, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if output := write("new\n"); output != path+" unchanged\n" {
		t.Fatalf("unchanged report = %q", output)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() {
		t.Fatal("unchanged output replaced or modified the existing file")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary output was left behind: %v, %v", entries, err)
	}
}

func TestDiffDoesNotDeleteStaleOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.pb.go")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := captureOutput(t, func() error { return genfile.Diff(path, nil) })
	if !strings.Contains(output, "-stale\n") {
		t.Fatalf("deletion diff = %q", output)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "stale\n" {
		t.Fatalf("diff modified output: %q, %v", content, err)
	}
}

func TestWriteRejectsInvalidDestinations(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Join(file, "child")} {
		if err := genfile.Write(path, []byte("replacement")); err == nil {
			t.Fatalf("accepted invalid destination %s", path)
		}
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != "preserve" {
		t.Fatalf("failed write changed parent file: %q, %v", content, err)
	}
}
