package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRuntimeLimitsDefaultsAndOverrides(t *testing.T) {
	cfg, _, err := LoadDir("../../../conf")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Audit, DefaultAudit()) || !reflect.DeepEqual(cfg.HTTP.BodyLimits, DefaultHTTPBodyLimits()) || cfg.Dictionary.CacheTTL != 10*time.Minute || cfg.Auth.MFA.LoginTicketTTL != 5*time.Minute {
		t.Fatal("configuration defaults drifted")
	}
	for key, value := range map[string]string{
		"SERVICE_DICTIONARY_CACHETTL": "43s", "SERVICE_AUTH_MFA_LOGINTICKETTTL": "37s",
		"SERVICE_AUDIT_DEFAULTWINDOW": "24h", "SERVICE_AUDIT_MAXWINDOW": "48h", "SERVICE_AUDIT_EXPORTMAXROWS": "3",
		"SERVICE_AUDIT_RECORDTIMEOUT": "120ms", "SERVICE_AUDIT_PAYLOADLIMIT": "512",
		"SERVICE_HTTP_BODYLIMITS_AUTHCHALLENGE": "101", "SERVICE_HTTP_BODYLIMITS_SLIDERCHALLENGE": "202", "SERVICE_HTTP_BODYLIMITS_AUTHVERIFICATION": "303",
		"SERVICE_HTTP_BODYLIMITS_AUTH": "404", "SERVICE_HTTP_BODYLIMITS_BUSINESS": "505", "SERVICE_HTTP_BODYLIMITS_DICTIONARY": "606",
	} {
		t.Setenv(key, value)
	}
	cfg, _, err = LoadDir("../../../conf")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dictionary.CacheTTL != 43*time.Second || cfg.Auth.MFA.LoginTicketTTL != 37*time.Second || cfg.Audit.DefaultWindow != 24*time.Hour || cfg.Audit.MaxWindow != 48*time.Hour || cfg.Audit.ExportMaxRows != 3 || cfg.Audit.RecordTimeout != 120*time.Millisecond || cfg.Audit.PayloadLimit != 512 {
		t.Fatal("runtime overrides lost")
	}
	want := HTTPBodyLimitsConfig{AuthChallenge: 101, SliderChallenge: 202, AuthVerification: 303, Auth: 404, Business: 505, Dictionary: 606}
	if cfg.HTTP.BodyLimits != want {
		t.Fatalf("body limits = %+v", cfg.HTTP.BodyLimits)
	}
}

func TestRuntimeLimitsRejectInvalidSettings(t *testing.T) {
	for _, body := range []string{
		"dictionary:\n  cacheTTL: 0s\n", "dictionary:\n  cacheTTL: -1s\n",
		"auth:\n  mfa:\n    loginTicketTTL: 0s\n", "auth:\n  mfa:\n    loginTicketTTL: 1ns\n",
		"audit:\n  defaultWindow: 0s\n", "audit:\n  maxWindow: -1s\n", "audit:\n  defaultWindow: 49h\n  maxWindow: 48h\n",
		"audit:\n  defaultWindow: 1ns\n", "audit:\n  exportMaxRows: -1\n", "audit:\n  recordTimeout: -1s\n", "audit:\n  payloadLimit: -1\n",
		"http:\n  bodyLimits:\n    auth: 0\n", "http:\n  bodyLimits:\n    business: -1\n",
	} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "limits.yml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadDir(dir); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.yml"), []byte("server:\n  name: old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if cfg, _, err := LoadDir(dir); err != nil || cfg.Audit != DefaultAudit() || cfg.HTTP.BodyLimits != DefaultHTTPBodyLimits() {
		t.Fatal("older configuration lost defaults", err)
	}
}
