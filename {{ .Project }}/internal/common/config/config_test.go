package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDirAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	content := []byte("server:\n  name: test-server\nhttp:\n  addr: :8080\n  timeout: 5s\n  readHeaderTimeout: 4s\nlog:\n  level: debug\nauth:\n  token: original\ndatabase:\n  dsn: postgresql://root:original@127.0.0.1/app\n")
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("SERVICE_HTTP_ADDR", ":19090")
	t.Setenv("SERVICE_AUTH_TOKEN", "abcdefghijk")
	t.Setenv("SERVICE_DATABASE_DSN", "postgresql://root:abcdefghijk@127.0.0.1/app")
	t.Setenv("SERVICE_HTTP_READHEADERTIMEOUT", "6s")
	t.Setenv("SERVICE_HTTP_READ_HEADER_TIMEOUT", "7s")

	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	if cfg.Server.Name != "test-server" || cfg.HTTP.Addr != ":19090" || cfg.HTTP.Timeout != 5*time.Second || cfg.HTTP.ReadHeaderTimeout != 6*time.Second {
		t.Fatalf("config = %#v", cfg)
	}
	values := make(map[string]string, len(overrides))
	for _, override := range overrides {
		values[override.Environment] = override.Value
	}
	if values["SERVICE_HTTP_ADDR"] != ":19090" || values["SERVICE_AUTH_TOKEN"] != "abc***ijk" || values["SERVICE_DATABASE_DSN"] != "postgresql://root:abc***ijk@127.0.0.1/app" {
		t.Fatalf("environment overrides = %#v", values)
	}
	if _, exists := values["SERVICE_HTTP_READ_HEADER_TIMEOUT"]; exists {
		t.Fatalf("unexpected snake_case override = %#v", values)
	}

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	LogOverrides(overrides)
	if strings.Contains(output.String(), "abcdefghijk") || !strings.Contains(output.String(), "abc***ijk") {
		t.Fatalf("override log = %q", output.String())
	}
}

func TestAuthSwitchEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  switches:\n    passwordResetRequired: true\n    protectSuper: false\n    protectCaptchaDictionaries: false\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_AUTH_SWITCHES_PASSWORDRESETREQUIRED", "false")
	t.Setenv("SERVICE_AUTH_SWITCHES_PROTECTSUPER", "true")
	t.Setenv("SERVICE_AUTH_SWITCHES_PROTECTCAPTCHADICTIONARIES", "true")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Switches.PasswordResetRequired || !cfg.Auth.Switches.ProtectSuper || !cfg.Auth.Switches.ProtectCaptchaDictionaries {
		t.Fatalf("auth switches = %#v", cfg.Auth.Switches)
	}
	if len(overrides) != 3 {
		t.Fatalf("auth switch overrides = %#v", overrides)
	}
}

{{ if .Computed.enable_redis_final -}}
func TestRedisPrefixEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "redis.yml"), []byte("redis:\n  prefix: \"dev:auth:\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_REDIS_PREFIX", "prod:auth:")
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Redis.Prefix != "prod:auth:" {
		t.Fatalf("redis prefix = %q", cfg.Redis.Prefix)
	}
}
{{ end -}}

func TestLoadDirErrors(t *testing.T) {
	if _, _, err := LoadDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory was accepted")
	}
	if _, _, err := LoadDir(t.TempDir()); err == nil {
		t.Fatal("empty directory was accepted")
	}
	malformed := t.TempDir()
	if err := os.WriteFile(filepath.Join(malformed, "bad.yml"), []byte("http: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadDir(malformed); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	invalidDuration := t.TempDir()
	if err := os.WriteFile(filepath.Join(invalidDuration, "bad.yml"), []byte("http:\n  timeout: never\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadDir(invalidDuration); err == nil {
		t.Fatal("invalid duration was accepted")
	}
}

func TestYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.yaml", "a.yml", "ignored.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.yml"), 0o700); err != nil {
		t.Fatal(err)
	}
	paths, err := yamlFiles(dir)
	if err != nil || len(paths) != 2 || filepath.Base(paths[0]) != "a.yml" {
		t.Fatalf("yaml files = %#v, %v", paths, err)
	}
}
