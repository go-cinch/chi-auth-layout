package rds

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestNewValidationAndPingError(t *testing.T) {
	if _, _, err := New(t.Context(), &config.Config{}); err == nil {
		t.Fatal("empty DSN was accepted")
	}
	var cfg config.Config
	cfg.Redis.DSN = "redis://127.0.0.1:6379/0"
	if _, _, err := New(t.Context(), &cfg); err == nil || !strings.Contains(err.Error(), "redis.prefix") {
		t.Fatalf("empty prefix error = %v", err)
	}
	cfg.Redis.Prefix = "test:auth:"
	cfg.Redis.DSN = "://invalid"
	if _, _, err := New(t.Context(), &cfg); err == nil || !strings.Contains(err.Error(), "parse redis dsn") {
		t.Fatalf("invalid DSN error = %v", err)
	}
	cfg.Redis.DSN = "redis://127.0.0.1:6379/0"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := New(ctx, &cfg); err == nil || !strings.Contains(err.Error(), "ping redis") {
		t.Fatalf("ping error = %v", err)
	}
}

func TestRedisConnectionError(t *testing.T) {
	dsn := "redis://default:password@127.0.0.1:6379/0"
	err := redisConnectionError(errors.New("WRONGPASS invalid username-password pair"), dsn)
	if strings.Contains(err.Error(), "default:password") || !strings.Contains(err.Error(), "default:pas***ord") {
		t.Fatalf("authentication error = %q", err)
	}

	err = redisConnectionError(io.EOF, dsn)
	if !errors.Is(err, io.EOF) || strings.Contains(err.Error(), "dsn:") {
		t.Fatalf("ordinary connection error = %v", err)
	}
}
