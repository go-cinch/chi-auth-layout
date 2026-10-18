package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestActionDetailFieldsDocumentation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "detail fields"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	type schemaDoc struct {
		Ref        string `yaml:"$ref"`
		Required   []string
		Properties map[string]any
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name        string
				Required    bool
				Description string
			}
			Responses map[string]struct {
				Content map[string]struct{ Schema schemaDoc }
			}
		}
		Components struct{ Schemas map[string]schemaDoc }
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	detail := doc.Paths["/action/{id}"]["get"]
	selectors := 0
	for _, parameter := range detail.Parameters {
		if parameter.Name == "f" {
			selectors++
			if parameter.Required || !strings.Contains(parameter.Description, "Omitted f") || !strings.Contains(parameter.Description, "names") || !strings.Contains(parameter.Description, "hidden dependencies") {
				t.Fatalf("selector contract: %+v", parameter)
			}
		}
	}
	if selectors != 1 {
		t.Fatalf("f parameter count = %d", selectors)
	}
	sparse := detail.Responses["200"].Content["application/json"].Schema
	if sparse.Ref != "" || len(sparse.Required) != 0 || len(sparse.Properties) != 13 {
		t.Fatalf("sparse detail schema: %+v", sparse)
	}
	if len(doc.Components.Schemas["Action"].Required) == 0 {
		t.Fatal("detail selection relaxed create/update Action schema")
	}
	for _, status := range []string{"400", "404", "500"} {
		if _, ok := detail.Responses[status]; !ok {
			t.Fatalf("missing %s response", status)
		}
	}
}
