package config

import (
	"bytes"
	"encoding/json"
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
	decoder := json.NewDecoder(&output)
	for _, override := range overrides {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["operation"] != "config_override" || record["outcome"] != "success" || record["env_key"] != override.Environment || record["config_key"] != override.Key || record["value"] != override.Value {
			t.Fatalf("override fields = %#v", record)
		}
		if record["msg"] != "load env: "+override.Environment+"="+override.Value {
			t.Fatalf("override message disagrees with fields: %#v", record)
		}
	}
}

func TestAuthSwitchEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  switches:\n    enableE2ETest: false\n    enableE2ETestFile: \"\"\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_AUTH_SWITCHES_ENABLEE2ETEST", "true")
	t.Setenv("SERVICE_AUTH_SWITCHES_ENABLEE2ETESTFILE", "/etc/podinfo/e2e-enabled")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Switches.EnableE2ETest {
		t.Fatalf("auth switches = %#v", cfg.Auth.Switches)
	}
	if cfg.Auth.Switches.EnableE2ETestFile != "/etc/podinfo/e2e-enabled" {
		t.Fatalf("e2e file override = %q", cfg.Auth.Switches.EnableE2ETestFile)
	}
	if len(overrides) != 2 {
		t.Fatalf("auth switch overrides = %#v", overrides)
	}
}

func TestMFARateLimitEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  mfa:\n    totpPeriod: 1m\n    enrollmentTTL: 5m\n    sensitiveLifetime: 1h\n    rateLimit:\n      window: 5m\n      maxAttempts: 10\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.MFA.RateLimit.Window != 5*time.Minute || cfg.Auth.MFA.RateLimit.MaxAttempts != 10 {
		t.Fatalf("default MFA rate limit = %#v", cfg.Auth.MFA.RateLimit)
	}
	t.Setenv("SERVICE_AUTH_MFA_RATELIMIT_WINDOW", "2m30s")
	t.Setenv("SERVICE_AUTH_MFA_RATELIMIT_MAXATTEMPTS", "4")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.MFA.RateLimit.Window != 150*time.Second || cfg.Auth.MFA.RateLimit.MaxAttempts != 4 {
		t.Fatalf("overridden MFA rate limit = %#v", cfg.Auth.MFA.RateLimit)
	}
	if cfg.Auth.MFA.TOTPPeriod != time.Minute || cfg.Auth.MFA.EnrollmentTTL != 5*time.Minute || cfg.Auth.MFA.SensitiveLifetime != time.Hour {
		t.Fatal("rate-limit overrides changed authenticator, enrollment or sensitive timing")
	}
	if len(overrides) != 2 {
		t.Fatalf("MFA rate-limit overrides = %#v", overrides)
	}
}

func TestMFASensitiveLifetimeEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  mfa:\n    totpPeriod: 1m\n    enrollmentTTL: 5m\n    sensitiveLifetime: 1h\n    rateLimit:\n      window: 5m\n      maxAttempts: 10\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.MFA.SensitiveLifetime != time.Hour {
		t.Fatalf("default MFA sensitive lifetime = %v", cfg.Auth.MFA.SensitiveLifetime)
	}
	t.Setenv("SERVICE_AUTH_MFA_SENSITIVELIFETIME", "20m30s")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.MFA.SensitiveLifetime != 20*time.Minute+30*time.Second {
		t.Fatalf("overridden MFA sensitive lifetime = %v", cfg.Auth.MFA.SensitiveLifetime)
	}
	if cfg.Auth.MFA.TOTPPeriod != time.Minute || cfg.Auth.MFA.EnrollmentTTL != 5*time.Minute || cfg.Auth.MFA.RateLimit.Window != 5*time.Minute || cfg.Auth.MFA.RateLimit.MaxAttempts != 10 {
		t.Fatal("sensitive lifetime override changed unrelated MFA timing or rate limits")
	}
	if len(overrides) != 1 || overrides[0].Environment != "SERVICE_AUTH_MFA_SENSITIVELIFETIME" {
		t.Fatalf("MFA sensitive lifetime overrides = %#v", overrides)
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

func TestDictionaryLocalizationRefreshInterval(t *testing.T) {
	for _, tc := range []struct {
		value string
		want time.Duration
		invalid bool
	}{
		{"5s", 5 * time.Second, false},
		{"1m", time.Minute, false},
		{"0s", 0, false},
		{"-1s", 0, true},
		{"invalid", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "dictionary.yml"), []byte("dictionary:\n  localizationRefreshInterval: 5s\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SERVICE_DICTIONARY_LOCALIZATIONREFRESHINTERVAL", tc.value)
			cfg, _, err := LoadDir(dir)
			if tc.invalid {
				if err == nil { t.Fatal("invalid interval accepted") }
				return
			}
			if err != nil { t.Fatal(err) }
			if cfg.Dictionary.LocalizationRefreshInterval != tc.want {
				t.Fatalf("interval = %v, want %v", cfg.Dictionary.LocalizationRefreshInterval, tc.want)
			}
		})
	}
}
