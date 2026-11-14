package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEmailHTTPDocumentation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "email.yaml")
	if err := run("../..", "../../../conf", output, "Email login"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string
				In       string
				Required bool
			}
			Responses map[string]struct{ Description string }
		}
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/pub/login/email-code/status", "/email/code/status", "/email/unbind/status"} {
		operation := document.Paths[path]["get"]
		if operation.Responses["200"].Description == "" || operation.Responses["500"].Description == "" {
			t.Fatal("status endpoint missing responses", path)
		}
		found := false
		for _, parameter := range operation.Parameters {
			if parameter.Name == "browser_uuid" && parameter.In == "query" && parameter.Required {
				found = true
			}
		}
		if !found {
			t.Fatal("persistent UUID omitted from status contract", path)
		}
	}
	for _, path := range []string{"/pub/login/email-code", "/email/code", "/email/unbind/request"} {
		operation := document.Paths[path]["post"]
		for _, status := range []string{"200", "400", "429", "500", "503"} {
			if operation.Responses[status].Description == "" {
				t.Fatal("email send status missing", path, status)
			}
		}
	}
	if document.Paths["/email/bind"]["post"].Responses["409"].Description == "" {
		t.Fatal("binding uniqueness conflict missing")
	}
}
