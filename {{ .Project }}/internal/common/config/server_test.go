package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerNamespace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  env: dev\n  name: orders\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil || cfg.RedisPrefix() != "dev:orders:" {
		t.Fatal("default namespace", err)
	}
	t.Setenv("SERVICE_SERVER_ENV", "prod")
	t.Setenv("SERVICE_SERVER_NAME", "billing")
	t.Setenv("SERVICE_REDIS_PREFIX", "ignored:legacy:")
	cfg, overrides, err := LoadDir(dir)
	if err != nil || cfg.ValidateServer() != nil || cfg.RedisPrefix() != "prod:billing:" || len(overrides) != 2 {
		t.Fatal("derived namespace", err)
	}
	for _, invalid := range []string{"", "prod:other", " prod", "prod ", "a/b", "a\nb"} {
		cfg.Server.Env, cfg.Server.Name = invalid, "auth"
		if cfg.ValidateServer() == nil {
			t.Fatalf("invalid env accepted: %q", invalid)
		}
		cfg.Server.Env, cfg.Server.Name = "prod", invalid
		if cfg.ValidateServer() == nil {
			t.Fatalf("invalid name accepted: %q", invalid)
		}
	}
}
