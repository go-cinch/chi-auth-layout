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

func TestNewRejectsMalformedDSNWithoutCredentials(t *testing.T) {
	const secret = "review-secret"
	for name, dsn := range map[string]string{
		"password slash": "redis://default:" + secret + "/fragment@127.0.0.1:6379/0",
		"invalid port":   "redis://default:" + secret + "@127.0.0.1:invalid/0",
		"invalid escape": "redis://default:" + secret + "%zz@127.0.0.1:6379/0",
	} {
		t.Run(name, func(t *testing.T) {
			var cfg config.Config
			cfg.Redis.DSN, cfg.Redis.Prefix = dsn, "test:auth:"
			client, cleanup, err := New(t.Context(), &cfg)
			if cleanup != nil {
				cleanup()
			}
			if client != nil || cleanup != nil || err == nil || !strings.Contains(err.Error(), "parse redis dsn") {
				t.Fatalf("invalid DSN result = %v, cleanup present = %v, error = %v", client, cleanup != nil, err)
			}
			// Startup logging unwraps errors; every level must be safe to log.
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), secret) {
					t.Fatalf("DSN parse error exposes credentials: %v", cause)
				}
			}
		})
	}
}
