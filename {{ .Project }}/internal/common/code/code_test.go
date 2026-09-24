package code

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	for range 100 {
		value, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if len(value) != Length {
			t.Fatalf("code length = %d, want %d", len(value), Length)
		}
		for _, character := range value {
			if !strings.ContainsRune("23456789ABCDEFGHJKLMNPQRSTVWXY", character) {
				t.Fatalf("code %q contains invalid character %q", value, character)
			}
		}
	}
}
