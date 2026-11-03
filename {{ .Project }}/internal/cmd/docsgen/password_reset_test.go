package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPasswordResetResponseHeadersDocumentation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "password reset headers"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Headers map[string]any
			}
		}
		Components struct {
			Schemas map[string]struct{ Properties map[string]any }
		}
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, status string
		reset, mfa           bool
	}{
		{"get", "/info", "403", true, true},
		{"get", "/msg/unread-count", "403", true, true},
		{"get", "/permission", "403", true, true},
		{"post", "/challenge", "403", true, true},
		{"post", "/pub/login", "200", true, true},
		{"post", "/pub/refresh", "200", true, true},
		{"post", "/mfa/confirm", "200", true, false},
		{"patch", "/reset/pwd", "200", false, false},
		{"post", "/pub/login", "403", false, true},
		{"get", "/info", "200", false, false},
		{"get", "/msg/unread-count", "401", false, false},
	} {
		response, ok := doc.Paths[test.path][test.method].Responses[test.status]
		if !ok {
			t.Fatalf("missing %s %s response %s", test.method, test.path, test.status)
		}
		_, reset := response.Headers["X-Password-Reset-Required"]
		_, mfa := response.Headers["X-MFA-Required"]
		if reset != test.reset || mfa != test.mfa {
			t.Errorf("%s %s response %s: reset=%v, MFA=%v", test.method, test.path, test.status, reset, mfa)
		}
	}
	loginResult, ok := doc.Components.Schemas["LoginResult"]
	if !ok {
		t.Fatal("missing login result schema")
	}
	if _, ok := loginResult.Properties["password_reset_required"]; ok {
		t.Fatal("password reset transport metadata must not appear in login JSON")
	}
}
