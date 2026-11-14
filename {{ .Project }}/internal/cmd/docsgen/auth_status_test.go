package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStatusDocumentation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "status.yaml")
	if err := run("../..", "../../../conf", output, "Status"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string
				Required bool
			}
			Responses map[string]struct {
				Description string
				Schema      any
			}
		}
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/status", "/pub/status"} {
		op := doc.Paths[path]["get"]
		required := map[string]bool{}
		for _, p := range op.Parameters {
			required[p.Name] = p.Required
		}
		if !required["purpose"] || !required["f"] || required["browser_uuid"] {
			t.Fatal(path, required)
		}
		if !strings.Contains(op.Responses["200"].Description, "server_time") {
			t.Fatal(path, op)
		}
		for _, code := range []string{"400", "401", "403", "500", "503"} {
			if op.Responses[code].Description == "" {
				t.Fatal(path, code)
			}
		}
	}
	if _, ok := doc.Paths["/pub/opt"]; !ok {
		t.Fatal("public options were removed")
	}
	if !strings.Contains(doc.Paths["/totp"]["get"].Responses["200"].Description, "Compatibility") {
		t.Fatal("legacy replacement undocumented")
	}
	if !strings.Contains(string(raw), "StatusResult:") || !strings.Contains(string(raw), "TOTPInfo:") {
		t.Fatal("status or account binding schema missing")
	}
}

func TestOnboardingResponseHeadersDocumentation(t *testing.T) {
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
		reset, verification           bool
	}{
		{"get", "/info", "403", true, true},
		{"get", "/msg/unread-count", "403", true, true},
		{"get", "/permission", "403", true, true},
		{"post", "/challenge", "403", true, true},
		{"post", "/pub/login", "200", true, true},
		{"post", "/pub/refresh", "200", true, true},
		{"post", "/totp/confirm", "200", true, false},
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
		_, verification := response.Headers["X-Verification-Required"]
		if reset || (verification != (test.verification && test.status != "200")) {
			t.Errorf("%s %s response %s: reset=%v, Verification=%v", test.method, test.path, test.status, reset, verification)
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
