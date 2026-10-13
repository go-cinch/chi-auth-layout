package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStructuredHidesNestedSecrets(t *testing.T) {
	input := map[string]any{"safe": "visible", "password": "hidden", "nested": []any{map[string]any{"refresh_token": "hidden", "jwk": map[string]any{"n": "hidden"}}, `{"secret":"hidden"}`}, "count": 42, "blank": ""}
	result, err := json.Marshal(New().Structured(input))
	if err != nil || strings.Contains(string(result), "hidden") || !strings.Contains(string(result), "visible") {
		t.Fatal(string(result), err)
	}
	if input["password"] != "hidden" {
		t.Fatal("input mutated")
	}
}
