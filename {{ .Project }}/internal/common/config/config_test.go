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

func TestEmailTemplateCamelCaseKeysAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	keys := []string{"signInCode", "emailBindingCode", "sensitiveOperationCode", "authenticatorResetCode", "confirmEmailUnlinking", "emailUnlinked", "emailLinked", "authenticatorLinked", "authenticatorRelinked", "sensitiveOperationNotification"}
	content := "auth:\n  email:\n    templates:\n"
	for _, key := range keys {
		content += "      " + key + ": default." + key + "\n"
		t.Setenv("SERVICE_AUTH_EMAIL_TEMPLATES_"+strings.ToUpper(key), "custom."+key)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	ids := cfg.Auth.Email.Templates
	actual := []string{ids.SignInCode, ids.EmailBindingCode, ids.SensitiveOperationCode, ids.AuthenticatorResetCode, ids.ConfirmEmailUnlinking, ids.EmailUnlinked, ids.EmailLinked, ids.AuthenticatorLinked, ids.AuthenticatorRelinked, ids.SensitiveOperationNotification}
	for i, key := range keys {
		if actual[i] != "custom."+key {
			t.Fatalf("%s did not load its override", key)
		}
	}
}

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
	content := []byte("auth:\n  testing:\n    enableE2ETest: false\n    enableE2ETestFile: ''\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_AUTH_TESTING_ENABLEE2ETEST", "true")
	t.Setenv("SERVICE_AUTH_TESTING_ENABLEE2ETESTFILE", "/etc/podinfo/e2e-enabled")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Testing.EnableE2ETest {
		t.Fatalf("auth switches = %#v", cfg.Auth.Testing)
	}
	if cfg.Auth.Testing.EnableE2ETestFile != "/etc/podinfo/e2e-enabled" {
		t.Fatalf("e2e file override = %q", cfg.Auth.Testing.EnableE2ETestFile)
	}
	if len(overrides) != 2 {
		t.Fatalf("auth switch overrides = %#v", overrides)
	}
}

func TestTOTPRateLimitEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  totp:\n    period: 1m\n    enrollmentTTL: 5m\n  verification:\n    sensitiveLifetime: 1h\n    rateLimit:\n      window: 5m\n      maxAttempts: 10\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Verification.RateLimit.Window != 5*time.Minute || cfg.Auth.Verification.RateLimit.MaxAttempts != 10 {
		t.Fatalf("default Verification rate limit = %#v", cfg.Auth.Verification.RateLimit)
	}
	t.Setenv("SERVICE_AUTH_VERIFICATION_RATELIMIT_WINDOW", "2m30s")
	t.Setenv("SERVICE_AUTH_VERIFICATION_RATELIMIT_MAXATTEMPTS", "4")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Verification.RateLimit.Window != 150*time.Second || cfg.Auth.Verification.RateLimit.MaxAttempts != 4 {
		t.Fatalf("overridden Verification rate limit = %#v", cfg.Auth.Verification.RateLimit)
	}
	if cfg.Auth.TOTP.Period != time.Minute || cfg.Auth.TOTP.EnrollmentTTL != 5*time.Minute || cfg.Auth.Verification.SensitiveLifetime != time.Hour {
		t.Fatal("rate-limit overrides changed authenticator, enrollment or sensitive timing")
	}
	if len(overrides) != 2 {
		t.Fatalf("Verification rate-limit overrides = %#v", overrides)
	}
}

func TestTOTPSensitiveLifetimeEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	content := []byte("auth:\n  totp:\n    period: 1m\n    enrollmentTTL: 5m\n  verification:\n    sensitiveLifetime: 1h\n    rateLimit:\n      window: 5m\n      maxAttempts: 10\n")
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Verification.SensitiveLifetime != time.Hour {
		t.Fatalf("default Verification sensitive lifetime = %v", cfg.Auth.Verification.SensitiveLifetime)
	}
	t.Setenv("SERVICE_AUTH_VERIFICATION_SENSITIVELIFETIME", "20m30s")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Verification.SensitiveLifetime != 20*time.Minute+30*time.Second {
		t.Fatalf("overridden Verification sensitive lifetime = %v", cfg.Auth.Verification.SensitiveLifetime)
	}
	if cfg.Auth.TOTP.Period != time.Minute || cfg.Auth.TOTP.EnrollmentTTL != 5*time.Minute || cfg.Auth.Verification.RateLimit.Window != 5*time.Minute || cfg.Auth.Verification.RateLimit.MaxAttempts != 10 {
		t.Fatal("sensitive lifetime override changed unrelated Verification timing or rate limits")
	}
	if len(overrides) != 1 || overrides[0].Environment != "SERVICE_AUTH_VERIFICATION_SENSITIVELIFETIME" {
		t.Fatalf("Verification sensitive lifetime overrides = %#v", overrides)
	}
}

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
		value   string
		want    time.Duration
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
				if err == nil {
					t.Fatal("invalid interval accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Dictionary.LocalizationRefreshInterval != tc.want {
				t.Fatalf("interval = %v, want %v", cfg.Dictionary.LocalizationRefreshInterval, tc.want)
			}
		})
	}
}

func TestEmailSendLimitEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), []byte("auth:\n  email:\n    sendLimit: 3\n    maxAttempts: 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil || cfg.Auth.Email.SendLimit != 3 {
		t.Fatal("default email send limit", err)
	}
	t.Setenv("SERVICE_AUTH_EMAIL_SENDLIMIT", "1")
	cfg, _, err = LoadDir(dir)
	if err != nil || cfg.Auth.Email.SendLimit != 1 || cfg.Auth.Email.MaxAttempts != 5 {
		t.Fatal("email send limit override", err)
	}
}

func TestOnboardingRequirementConfiguration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), []byte("auth: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defaults, _, err := LoadDir(dir)
	if err != nil || !defaults.Auth.Password.Reset.Required || !defaults.Auth.TOTP.RequireBinding {
		t.Fatal("omitted policies must preserve enforcement", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), []byte("auth:\n  totp:\n    enabled: true\n    requireBinding: true\n  password:\n    reset:\n      required: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_AUTH_PASSWORD_RESET_REQUIRED", "false")
	t.Setenv("SERVICE_AUTH_TOTP_REQUIREBINDING", "false")
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Password.Reset.Required || cfg.Auth.TOTP.RequireBinding || !cfg.Auth.TOTP.Enabled {
		t.Fatal("independent onboarding overrides were not applied")
	}
}

func TestOnboardingVisibilityConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.yml")
	if err := os.WriteFile(path, []byte("auth: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.TOTP.HideOnboarding || cfg.Auth.Email.HideOnboarding || cfg.Auth.Password.Reset.HideOnboarding {
		t.Fatal("missing visibility must remain false")
	}
	if err := os.WriteFile(path, []byte("auth:\n  email:\n    hideOnboarding: false\n  totp:\n    hideOnboarding: false\n  password:\n    reset:\n      hideOnboarding: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_AUTH_TOTP_HIDEONBOARDING", "true")
	t.Setenv("SERVICE_AUTH_EMAIL_HIDEONBOARDING", "true")
	t.Setenv("SERVICE_AUTH_PASSWORD_RESET_HIDEONBOARDING", "true")
	cfg, _, err = LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.TOTP.HideOnboarding || !cfg.Auth.Email.HideOnboarding || !cfg.Auth.Password.Reset.HideOnboarding {
		t.Fatal("visibility overrides not applied")
	}
}

func TestEmailAndTOTPDebugEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "debug.yml"), []byte("email:\n  enabled: true\n  skipDelivery: false\nauth:\n  totp:\n    enabled: true\n  testing:\n    emailCode: false\n    totpCode: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil || cfg.Email.SkipDelivery || cfg.Auth.Testing.TOTPCode || cfg.Auth.Testing.EmailCode {
		t.Fatal("debug must default off", err)
	}
	t.Setenv("SERVICE_EMAIL_SKIPDELIVERY", "true")
	cfg, _, err = LoadDir(dir)
	if err != nil || !cfg.Email.SkipDelivery || cfg.Auth.Testing.TOTPCode || cfg.Auth.Testing.EmailCode {
		t.Fatal("email debug must be independent", err)
	}
	t.Setenv("SERVICE_AUTH_TESTING_EMAILCODE", "true")
	t.Setenv("SERVICE_EMAIL_SKIPDELIVERY", "false")
	cfg, _, err = LoadDir(dir)
	if err != nil || !cfg.Auth.Testing.EmailCode || cfg.Auth.Testing.TOTPCode || cfg.Email.SkipDelivery {
		t.Fatal("fixed OTP and SMTP suppression must be independent", err)
	}
	t.Setenv("SERVICE_EMAIL_SKIPDELIVERY", "true")
	t.Setenv("SERVICE_AUTH_TESTING_TOTPCODE", "true")
	cfg, _, err = LoadDir(dir)
	if err != nil || !cfg.Email.SkipDelivery || !cfg.Auth.Testing.TOTPCode || !cfg.Email.Enabled || !cfg.Auth.TOTP.Enabled {
		t.Fatal("debug overrides changed feature availability", err)
	}
}

func TestHistoryRetentionConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.yml")
	if err := os.WriteFile(path, []byte("auth: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadDir(dir)
	if err != nil || cfg.Auth.History.RetentionDays != 30 {
		t.Fatal("default history retention", err)
	}
	if err := os.WriteFile(path, []byte("auth:\n  history:\n    retentionDays: 30\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value   string
		want    int
		invalid bool
	}{
		{"7", 7, false}, {"0", 0, true}, {"-1", 0, true}, {"106752", 0, true}, {"invalid", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SERVICE_AUTH_HISTORY_RETENTIONDAYS", tc.value)
			cfg, _, err := LoadDir(dir)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid retention accepted")
				}
				return
			}
			if err != nil || cfg.Auth.History.RetentionDays != tc.want {
				t.Fatal("retention override", err)
			}
		})
	}
}

func TestSessionKeyOverrideRedacted(t *testing.T) {
	dir := t.TempDir()
	content := "auth:\n  session:\n    jwt:\n      key: original\nredact:\n  keys:\n    - auth.session.jwt.key\n"
	if err := os.WriteFile(filepath.Join(dir, "auth.yml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	const secret = "new-session-signing-secret"
	t.Setenv("SERVICE_AUTH_SESSION_JWT_KEY", secret)
	cfg, overrides, err := LoadDir(dir)
	if err != nil || cfg.Auth.Session.Jwt.Key != secret || len(overrides) != 1 || overrides[0].Value == secret {
		t.Fatal("session key override must remain redacted", err)
	}
}
