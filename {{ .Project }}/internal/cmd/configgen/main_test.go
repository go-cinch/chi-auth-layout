package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"testing"
)

func TestRunGeneratesAndUpdates(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "conf")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "server.yml")
	content := "http:\n  timeout: 5s\n  ratio: 0.5\n  profiler:\n    enabled: false\nitems:\n  - name: first\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "generated", "config.gen.go")
	if err := run(configDir, output); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"time.Duration", "float64", "HTTPProfilerConfig", "[]ItemsItemConfig"} {
		if !bytes.Contains(generated, []byte(expected)) {
			t.Fatalf("generated source missing %q:\n%s", expected, generated)
		}
	}
	if err := run(configDir, output); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"server:\n  name: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(configDir, output); err != nil {
		t.Fatal(err)
	}
}

func TestRunErrors(t *testing.T) {
	if err := run(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "out.go")); err == nil {
		t.Fatal("missing directory was accepted")
	}
	if err := run(t.TempDir(), filepath.Join(t.TempDir(), "out.go")); err == nil {
		t.Fatal("empty directory was accepted")
	}
	malformed := t.TempDir()
	if err := os.WriteFile(filepath.Join(malformed, "bad.yml"), []byte("x: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(malformed, filepath.Join(t.TempDir(), "out.go")); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	valid := t.TempDir()
	if err := os.WriteFile(filepath.Join(valid, "ok.yml"), []byte("x: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(valid, filepath.Join(blocked, "out.go")); err == nil {
		t.Fatal("output below a regular file was accepted")
	}
}

func TestDurationConversion(t *testing.T) {
	source := []byte("package config\n\ntype Config struct {\n\tTimeout string `koanf:\"timeout\"`\n\tChallengeTTL string `koanf:\"challengeTTL\"`\n}\n")
	converted, err := addDurationTypes(source)
	if err != nil || bytes.Count(converted, []byte("time.Duration")) != 2 {
		t.Fatalf("converted source = %s, %v", converted, err)
	}
	unchanged := []byte("package config\n\ntype Config struct { Name string }\n")
	actual, err := addDurationTypes(unchanged)
	if err != nil || !bytes.Equal(actual, unchanged) {
		t.Fatalf("unchanged source = %s, %v", actual, err)
	}
	invalid := []byte("package config\n\ntype Config struct {\n Timeout string `koanf:\"timeout\"`\n")
	if _, err := addDurationTypes(invalid); err == nil {
		t.Fatal("invalid Go was accepted")
	}
}

func TestNestedStructNaming(t *testing.T) {
	source := []byte("package config\n\ntype Config struct { HTTP struct { TLS *struct { Names []struct { Value string } } } }\n")
	converted, err := nameNestedStructs(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"HTTPConfig", "HTTPTLSConfig", "HTTPTLSNamesItemConfig"} {
		if !bytes.Contains(converted, []byte(expected)) {
			t.Fatalf("named source missing %q:\n%s", expected, converted)
		}
	}
	if _, err := nameNestedStructs([]byte("package config\ntype Other struct{}")); err == nil {
		t.Fatal("missing Config type was accepted")
	}
	if _, err := nameNestedStructs([]byte("not go")); err == nil {
		t.Fatal("invalid Go was accepted")
	}
	expression, err := parser.ParseExpr("map[string]struct{ Child struct{ Value string } }")
	if err != nil {
		t.Fatal(err)
	}
	if _, declarations, err := liftNestedType(expression, "Values", map[string]struct{}{}); err != nil || len(declarations) != 2 {
		t.Fatalf("declarations = %d, %v", len(declarations), err)
	}
	duplicate := &ast.StructType{Fields: &ast.FieldList{}}
	if _, _, err := liftNestedType(duplicate, "HTTP", map[string]struct{}{"HTTPConfig": {}}); err == nil {
		t.Fatal("duplicate type was accepted")
	}
	identifier := ast.NewIdent("string")
	if actual, declarations, err := liftNestedType(identifier, "Value", map[string]struct{}{}); err != nil || actual != identifier || len(declarations) != 0 {
		t.Fatalf("leaf = %#v, %#v, %v", actual, declarations, err)
	}
}
