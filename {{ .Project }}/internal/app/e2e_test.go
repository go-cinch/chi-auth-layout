package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestE2ETestEnabledWithoutFile(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Auth.Switches.EnableE2ETest = enabled
		if got := e2eTestEnabledFromConfig(cfg)(); got != enabled {
			t.Fatalf("static e2e = %v, want %v", got, enabled)
		}
	}
}

func TestE2ETestFileOverridesStaticFlagAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e-enabled")
	for _, fallback := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Auth.Switches.EnableE2ETest = fallback
		cfg.Auth.Switches.EnableE2ETestFile = path
		enabled := e2eTestEnabledFromConfig(cfg)
		if enabled() {
			t.Fatal("missing file enabled e2e")
		}
		for _, value := range []string{"true", "false", " \ttrue\n", "", "TRUE", "1", "invalid", "true"} {
			writeE2ETestFile(t, path, value)
			want := value == "true" || value == " \ttrue\n"
			if got := enabled(); got != want {
				t.Fatalf("file %q with fallback %v: got %v, want %v", value, fallback, got, want)
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if enabled() {
			t.Fatal("removed file retained e2e")
		}
	}
	// Reading a directory fails even when tests run with elevated privileges.
	cfg := &config.Config{}
	cfg.Auth.Switches.EnableE2ETest = true
	cfg.Auth.Switches.EnableE2ETestFile = t.TempDir()
	if e2eTestEnabledFromConfig(cfg)() {
		t.Fatal("unreadable file enabled e2e")
	}
}

func TestE2ETestFileFollowsProjectedVolumeReplacement(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary")
	stable := filepath.Join(dir, "stable")
	for _, path := range []string{canary, stable} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeE2ETestFile(t, filepath.Join(canary, "e2e-enabled"), "true")
	writeE2ETestFile(t, filepath.Join(stable, "e2e-enabled"), "false")
	if err := os.Symlink(canary, filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "e2e-enabled")
	if err := os.Symlink("..data/e2e-enabled", path); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Auth.Switches.EnableE2ETestFile = path
	enabled := e2eTestEnabledFromConfig(cfg)
	if !enabled() {
		t.Fatal("canary file did not enable e2e")
	}
	if err := os.Symlink(stable, filepath.Join(dir, "..data-next")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "..data-next"), filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}
	if enabled() {
		t.Fatal("stable file did not disable e2e after symlink replacement")
	}
}

func TestSliderCaptchaFileSwitchWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e-enabled")
	writeE2ETestFile(t, path, "true")
	cfg := &config.Config{}
	cfg.Auth.Switches.EnableE2ETest = true
	cfg.Auth.Switches.EnableE2ETestFile = path
	cfg.Auth.SliderCaptcha.TTL = time.Minute
	cfg.Auth.SliderCaptcha.MinimumDuration = 100 * time.Millisecond
	manager, err := sliderCaptchaFromConfig(cfg, authmodule.NewMemoryPointCaptchaStore())
	if err != nil {
		t.Fatal(err)
	}
	verify := func() (*authmodule.SliderCaptchaVerificationResult, error) {
		challenge, err := manager.IssueSliderChallenge(t.Context(), "login", "readonly")
		if err != nil {
			t.Fatal(err)
		}
		return manager.VerifySlider(t.Context(), authmodule.SliderCaptchaVerification{
			CaptchaID: challenge.CaptchaID, Purpose: "login", Username: "readonly",
		})
	}
	proof, err := verify()
	if err != nil {
		t.Fatal(err)
	}
	writeE2ETestFile(t, path, "false")
	if err := manager.ConsumeProof(t.Context(), "login", "readonly", proof.Proof); !errors.Is(err, authmodule.ErrSliderCaptchaRequired) {
		t.Fatalf("stable accepted e2e proof: %v", err)
	}
	if _, err := verify(); !errors.Is(err, authmodule.ErrSliderCaptchaRequired) {
		t.Fatalf("stable accepted invalid drag: %v", err)
	}
	writeE2ETestFile(t, path, "true")
	if _, err := verify(); err != nil {
		t.Fatalf("re-enabled e2e rejected test drag: %v", err)
	}
}

func writeE2ETestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
