package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCatalogDocumentationMatchesHTTPContracts(t *testing.T) {
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "contract test"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name        string
				Required    bool
				Description string
			}
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string
				Properties map[string]struct {
					Type     string
					Nullable bool
				}
			}
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/action/catalog", "/action/effective", "/dictionary/pub/i18n"} {
		found := false
		for _, parameter := range doc.Paths[path]["get"].Parameters {
			if parameter.Name == "f" {
				found = true
				if parameter.Required || !strings.Contains(parameter.Description, "Omitted f") {
					t.Fatalf("selector contract: %+v", parameter)
				}
			}
			if path == "/action/effective" && parameter.Name == "scope" && !parameter.Required {
				t.Fatal("scope must be required")
			}
		}
		if !found {
			t.Fatalf("missing f: %s", path)
		}
	}
	if len(doc.Components.Schemas["CatalogResult"].Required) != 0 {
		t.Fatal("unselected catalog fields must be optional")
	}
	effective := doc.Components.Schemas["EffectiveResult"]
	if len(effective.Required) != 1 || effective.Required[0] != "scope" {
		t.Fatal(effective.Required)
	}
	for _, key := range []string{"btn", "http", "grpc"} {
		if effective.Properties[key].Type != "array" {
			t.Fatalf("%s is not documented as an array", key)
		}
	}
	for _, name := range []string{"Config", "ResourceBinding"} {
		if !doc.Components.Schemas[name].Properties["feature_keys"].Nullable {
			t.Fatalf("%s.feature_keys must accept null", name)
		}
	}
	if doc.Components.Schemas["Config"].Properties["endpoints"].Nullable {
		t.Fatal("ordinary arrays must not become nullable")
	}
	public := doc.Components.Schemas["PublicCatalog"]
	if len(public.Required) != 1 || public.Required[0] != "schema_version" {
		t.Fatal(public.Required)
	}
	if doc.Components.Schemas["PublicCatalog"].Properties["messages"].Type != "object" {
		t.Fatal("public dictionary response has no message schema")
	}
}
