package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCause(t *testing.T) {
	root := errors.New("root")
	if rootCause(root) != root {
		t.Fatal("rootCause changed an unwrapped error")
	}
	if rootCause(fmt.Errorf("outer: %w", root)) != root {
		t.Fatal("rootCause did not unwrap an error")
	}
	joined := errors.Join(errors.New("one"), errors.New("two"))
	if rootCause(joined) != joined {
		t.Fatal("rootCause changed a joined error")
	}
}

func TestRunErrors(t *testing.T) {
	if err := run([]string{"-unknown"}); err == nil {
		t.Fatal("run accepted an unknown flag")
	}
	err := run([]string{"-c", filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.Contains(err.Error(), "initialize application") {
		t.Fatalf("run error = %v", err)
	}
}
