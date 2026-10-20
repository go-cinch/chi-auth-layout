package db

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/config"

	"github.com/lib/pq"
)

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	configs := []*config.Config{
		{},
		{Database: config.DatabaseConfig{Driver: "sqlite"}},
		{Database: config.DatabaseConfig{Driver: "mysql"}},
		{Database: config.DatabaseConfig{Driver: "postgres"}},
	}
	for _, cfg := range configs {
		if _, _, err := New(t.Context(), cfg); err == nil {
			t.Fatalf("New() accepted config %#v", cfg.Database)
		}
	}
}

func TestInvalidDSNDoesNotExposeParserCredentials(t *testing.T) {
	for _, dsn := range []string{
		"host=localhost user=test password=prefix private-password-fragment",
		"postgres://test:private-password-fragment@localhost:invalid/db",
		"postgres://test:private-password-fragment@localhost/db?sslmode=invalid",
	} {
		cfg := &config.Config{Database: config.DatabaseConfig{Driver: "postgres", DSN: dsn}}
{{ if .Computed.enable_trace_final }}
		for _, tracing := range []bool{false, true} {
			cfg.Tracer.Enabled = tracing
{{ end }}
			store, cleanup, err := New(t.Context(), cfg)
			if store != nil || cleanup != nil || err == nil || err.Error() != "invalid database dsn" {
				t.Fatalf("invalid DSN result: %v", err)
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), "private-password-fragment") {
					t.Fatal("parser credentials remain reachable through error unwrapping")
				}
			}
{{ if .Computed.enable_trace_final }}
		}
{{ end }}
	}
}

func TestDatabaseConnectionError(t *testing.T) {
	tests := []struct {
		driver string
		dsn    string
		err    error
	}{
		{
			driver: "postgres",
			dsn:    "postgresql://root:password@127.0.0.1:5432/app?sslmode=disable",
			err:    &pq.Error{Code: "28P01", Message: `password authentication failed for user "root"`},
		},
	}
	for _, test := range tests {
		err := databaseConnectionError(test.err, test.driver, test.dsn)
		if strings.Contains(err.Error(), "root:password") || !strings.Contains(err.Error(), "root:pas***ord") {
			t.Fatalf("%s authentication error = %q", test.driver, err)
		}
	}

	err := databaseConnectionError(io.EOF, "postgres", "dsn")
	if !errors.Is(err, io.EOF) || strings.Contains(err.Error(), "dsn:") {
		t.Fatalf("ordinary connection error = %v", err)
	}
}

func TestNewReturnsPingError(t *testing.T) {
	cfg := &config.Config{Database: config.DatabaseConfig{
		Driver: "postgres",
		DSN:    "postgresql://root:password@127.0.0.1:1/app?sslmode=disable",
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := New(ctx, cfg); err == nil || !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("New() error = %v", err)
	}
{{- if .Computed.enable_trace_final }}
	cfg.Tracer.Enabled = true
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	if _, _, err := New(ctx, cfg); err == nil || !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("traced New() error = %v", err)
	}
{{- end }}
}
