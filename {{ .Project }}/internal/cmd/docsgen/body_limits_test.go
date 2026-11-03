package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredRuntimeLimitDocumentation(t *testing.T) {
	t.Setenv("SERVICE_HTTP_BODYLIMITS_AUTH", "12345")
	t.Setenv("SERVICE_HTTP_BODYLIMITS_BUSINESS", "23456")
	t.Setenv("SERVICE_HTTP_BODYLIMITS_DICTIONARY", "34567")
	t.Setenv("SERVICE_AUTH_MFA_LOGINTICKETTTL", "37s")
	t.Setenv("SERVICE_AUDIT_DEFAULTWINDOW", "24h")
	t.Setenv("SERVICE_AUDIT_MAXWINDOW", "48h")
	t.Setenv("SERVICE_AUDIT_EXPORTMAXROWS", "27")
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "configured limits"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, limit string }{
		{"/pub/login", "12345"},
		{"/user", "23456"},
		{"/dictionary", "34567"},
	} {
		body, _ := readRequestSchema(t, output, tc.path)
		if !strings.Contains(body.Description, "Maximum JSON request body: "+tc.limit+" bytes.") {
			t.Fatalf("%s body limit missing: %s", tc.path, body.Description)
		}
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"last 24h0m0s", "at most 48h0m0s", "at most 27 records", "Ticket lifetime: 37s"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing configured documentation %q", want)
		}
	}
}
