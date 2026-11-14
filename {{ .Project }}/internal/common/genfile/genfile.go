// Package genfile reports and atomically replaces generated files.
package genfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmezard/go-difflib/difflib"
)

// Write reports a unified diff and replaces output with source. Identical
// content leaves the existing file, permissions and modification time intact.
func Write(output string, source []byte) error {
	if err := Diff(output, source); err != nil {
		return err
	}
	return writeAtomic(output, source)
}

// Diff reports creation, unchanged content, or a unified diff without writing.
// Callers removing stale generated files can pass nil before deleting the file.
func Diff(output string, source []byte) error {
	current, err := os.ReadFile(output)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("create %s\n", output)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read generated file %q: %w", output, err)
	}
	if bytes.Equal(current, source) {
		fmt.Printf("%s unchanged\n", output)
		return nil
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(current)),
		B:        difflib.SplitLines(string(source)),
		FromFile: output,
		ToFile:   output + " (generated)",
		Context:  3,
	})
	if err != nil {
		return fmt.Errorf("render generated file diff: %w", err)
	}
	fmt.Print(diff)
	return nil
}

func writeAtomic(output string, source []byte) error {
	current, err := os.ReadFile(output)
	if err == nil && bytes.Equal(current, source) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read generated file %q: %w", output, err)
	}
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".gen-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(source); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set output permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Rename(name, output); err != nil {
		return fmt.Errorf("replace generated file %q: %w", output, err)
	}
	return nil
}
