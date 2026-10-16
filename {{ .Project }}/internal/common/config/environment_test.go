package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexedEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	data := `http:
  addr: ":8080"
  docs:
    servers:
      - url: "https://prod.example"
        description: "prod"
      - url: "https://dev.example"
        description: "dev"
      - url: "http://127.0.0.1:8080"
        description: "local"
`
	if err := os.WriteFile(filepath.Join(dir, "http.yml"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_2_URL", "http://127.0.0.1:8083")
	t.Setenv("SERVICE_HTTP_ADDR", ":8083")
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_3_URL", "https://out-of-range.example")
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_2_UNKNOWN", "ignored")
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_-1_URL", "ignored")
	cfg, overrides, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	servers := cfg.HTTP.Docs.Servers
	if len(servers) != 3 || servers[0].URL != "https://prod.example" || servers[1].URL != "https://dev.example" || servers[2].URL != "http://127.0.0.1:8083" || servers[2].Description != "local" || cfg.HTTP.Addr != ":8083" {
		t.Fatalf("config: %#v", cfg)
	}
	if len(overrides) != 2 || overrides[0].Environment != "SERVICE_HTTP_ADDR" || overrides[1].Key != "http.docs.servers.2.url" {
		t.Fatalf("overrides: %#v", overrides)
	}
	// An explicitly empty string is an override, not a missing variable.
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_2_DESCRIPTION", "")
	cfg, _, err = LoadDir(dir)
	if err != nil || cfg.HTTP.Docs.Servers[2].Description != "" {
		t.Fatalf("empty override: %#v, %v", cfg, err)
	}
}

func TestNestedListsAndSensitiveOverrides(t *testing.T) {
	dir := t.TempDir()
	data := `custom:
  labels: [first, second]
  ports: [8080, 8081]
  enabled: [false, true]
  matrix:
    - [{token: original, retries: 1}]
  empty: []
`
	if err := os.WriteFile(filepath.Join(dir, "custom.yml"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_CUSTOM_LABELS_1", "new label")
	t.Setenv("SERVICE_CUSTOM_PORTS_0", "9090")
	t.Setenv("SERVICE_CUSTOM_ENABLED_0", "true")
	t.Setenv("SERVICE_CUSTOM_MATRIX_0_0_TOKEN", "abcdefghijk")
	t.Setenv("SERVICE_CUSTOM_MATRIX_0_0_RETRIES", "3")
	t.Setenv("SERVICE_CUSTOM_EMPTY_0", "ignored")
	values, overrides, err := LoadValues(dir)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Custom struct {
			Labels  []string
			Ports   []int
			Enabled []bool
			Matrix  [][]struct {
				Token   string
				Retries int
			}
			Empty []string
		}
	}
	if err := values.Unmarshal("", &cfg); err != nil {
		t.Fatal(err)
	}
	custom := cfg.Custom
	if custom.Labels[0] != "first" || custom.Labels[1] != "new label" || custom.Ports[0] != 9090 || custom.Ports[1] != 8081 || !custom.Enabled[0] || !custom.Enabled[1] || len(custom.Empty) != 0 {
		t.Fatalf("lists: %#v", custom)
	}
	if custom.Matrix[0][0].Token != "abcdefghijk" || custom.Matrix[0][0].Retries != 3 {
		t.Fatalf("nested list: %#v", custom.Matrix)
	}
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	LogOverrides(overrides)
	if strings.Contains(output.String(), "abcdefghijk") || !strings.Contains(output.String(), "load env: SERVICE_CUSTOM_MATRIX_0_0_TOKEN=abc***ijk") {
		t.Fatalf("redacted log: %s", output.String())
	}
	t.Setenv("SERVICE_CUSTOM_PORTS_0", "not-a-number")
	values, _, err = LoadValues(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := values.Unmarshal("", &cfg); err == nil {
		t.Fatal("invalid typed list value accepted")
	}
}
