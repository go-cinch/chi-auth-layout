package main

import (
	"testing"
)

func TestSchemaExamples(t *testing.T) {
	for _, tc := range []struct{ typ, input, want string }{
		{"integer", "1990", "1990"}, {"number", "1.5", "1.5"}, {"boolean", "true", "true"},
		{"string", "hello", `"hello"`}, {"integer", "invalid", `"invalid"`},
	} {
		if got := schemaExample(&schema{typeName: tc.typ, example: tc.input}); got != tc.want {
			t.Fatalf("example: %s", got)
		}
	}
}

func TestOperationID(t *testing.T) {
	if operationID("POST", "/user/{id}/password") != "postUserIdPassword" {
		t.Fatal("unexpected operation ID")
	}
}
