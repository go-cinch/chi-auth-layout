package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
{{- if .Computed.enable_trace_final }}
	"go.opentelemetry.io/otel"
{{- end }}
)

func TestNewErrors(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if _, err := New(t.Context(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing config directory was accepted")
	}
	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: invalid\n")); err == nil || !strings.Contains(err.Error(), "parse log level") {
		t.Fatalf("invalid log error = %v", err)
	}

	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: info\ndatabase:\n  dsn: ''\n")); err == nil || !strings.Contains(err.Error(), "database dsn is required") {
		t.Fatalf("invalid database error = %v", err)
	}

{{- if .Computed.enable_trace_final }}
	traced := "server:\n  name: test\nlog:\n  level: info\ntracer:\n  enabled: true\n  ratio: 0\n  stdout: true\ndatabase:\n  dsn: ''\n"
	if _, err := New(t.Context(), writeConfig(t, traced)); err == nil || !strings.Contains(err.Error(), "database dsn is required") {
		t.Fatalf("traced invalid database error = %v", err)
	}

{{- end }}
}

func TestNewRejectsAmbiguousAuthenticationPath(t *testing.T) {
	for _, path := range []string{"/auth/../pub", "/auth%2fpub", "auth"} {
		t.Run(path, func(t *testing.T) {
			// Reject ambiguous authentication mounts before initializing logging
			// or dependencies; later invalid settings cannot mask this boundary.
			content := "auth:\n  path: \"" + path + "\"\nlog:\n  level: invalid\n"
			application, err := New(t.Context(), writeConfig(t, content))
			if application != nil || err == nil || !strings.Contains(err.Error(), "invalid auth.path") {
				t.Fatalf("auth.path=%q: application=%v error=%v", path, application, err)
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
 if !strings.Contains(content, "server:\n") { content = "server:\n  env: test\n  name: auth\n" + content } else { content = strings.Replace(content, "server:\n", "server:\n  env: test\n", 1) }
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInvalidPaginationConfig(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: info\npagination:\n  maxP: -1\n  maxS: 10000\n")); err == nil || !strings.Contains(err.Error(), "pagination.maxP") {
		t.Fatalf("pagination: %v", err)
	}
}

{{ if .Computed.enable_trace_final -}}
func TestNewClosesTracingOnStartupFailure(t *testing.T) {
	previousLogger := slog.Default()
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	content := "server:\n  name: test\nlog:\n  level: info\ntracer:\n  enabled: true\n  ratio: 1\n  stdout: true\ndatabase:\n  dsn: ''\n"
	application, err := New(t.Context(), writeConfig(t, content))
	if application != nil || err == nil || !strings.Contains(err.Error(), "initialize database") {
		t.Fatalf("New() = %v, %v", application, err)
	}
	provider := otel.GetTracerProvider()
	if provider == previousProvider {
		t.Fatal("tracing was not initialized before the database failure")
	}
	_, span := provider.Tracer("startup-cleanup-test").Start(t.Context(), "after failure")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("startup failure left the tracing provider running")
	}
}

{{ end -}}
