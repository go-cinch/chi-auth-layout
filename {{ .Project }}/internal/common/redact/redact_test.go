package redact

import "testing"

func TestPolicyAndMask(t *testing.T) {
	policy := New("tenantCode")
	for _, name := range []string{"password", "accessToken", "SERVICE_DATABASE_DSN", "tenant_code"} {
		if !policy.IsSensitive(name) {
			t.Errorf("%q is not sensitive", name)
		}
	}
	if policy.IsSensitive("monkey") {
		t.Fatal("monkey matched key")
	}
	for input, expected := range map[string]string{
		"abcdefgh": "abc***fgh",
		"abcdef":   "abc***",
		"abcde":    "abc***",
		"ab":       "ab***",
		"":         "***",
	} {
		if actual := Mask(input); actual != expected {
			t.Errorf("Mask(%q) = %q, want %q", input, actual, expected)
		}
	}
	for input, expected := range map[string]string{
		"redis://127.0.0.1:6379/0":               "redis://127.0.0.1:6379/0",
		"redis://root:password@127.0.0.1:6379/0": "redis://root:pas***ord@127.0.0.1:6379/0",
		"root:password@tcp(127.0.0.1:3306)/app":  "root:pas***ord@tcp(127.0.0.1:3306)/app",
		"root@tcp(127.0.0.1:3306)/app":           "root@tcp(127.0.0.1:3306)/app",
		"redis://root@127.0.0.1:6379/0":          "redis://root@127.0.0.1:6379/0",
		"not-a-dsn":                              "not-a-dsn",
	} {
		if actual := DSN(input); actual != expected {
			t.Errorf("DSN(%q) = %q, want %q", input, actual, expected)
		}
	}
	if value := policy.Value("username", "plain"); value != "plain" {
		t.Fatalf("plain value = %q", value)
	}
}
