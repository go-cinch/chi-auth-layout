package rds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/redact"

	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, cfg *config.Config) (*Client, func(), error) {
	logger := slog.Default()
	dsn := strings.TrimSpace(cfg.Redis.DSN)
	if dsn == "" {
		return nil, nil, errors.New("redis dsn is required")
	}
	options, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse redis dsn: %w", err)
	}
	client, err := newClient(options, cfg.Redis.Prefix)
	if err != nil {
		return nil, nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, redisConnectionError(err, dsn)
	}
	cleanup := func() {
		if err := client.Close(); err != nil {
			logger.Error("close redis failed: " + err.Error())
		}
	}
	logger.InfoContext(ctx, "redis initialized: "+redact.DSN(dsn))
	return client, cleanup, nil
}

func redisConnectionError(err error, dsn string) error {
	if redis.IsAuthError(err) {
		return fmt.Errorf("%v; dsn: %s", err, redact.DSN(dsn))
	}
	return fmt.Errorf("ping redis: %w", err)
}
