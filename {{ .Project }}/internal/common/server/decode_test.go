package server

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

func TestDecodeJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		valid      bool
	}{
		{"object", `{"name":"ok"}`, 13, true},
		{"whitespace", "{\"name\":\"ok\"} \n", 16, true},
		{"too large", `{"name":"ok"}`, 12, false},
		{"unknown field", `{"other":"ok"}`, 64, false},
		{"multiple values", `{"name":"ok"}{}`, 64, false},
		{"trailing garbage", `{"name":"ok"}x`, 64, false},
		{"empty", "", 64, false},
		{"malformed", `{"name":`, 64, false},
		{"wrong field type", `{"name":1}`, 64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input struct {
				Name string `json:"name"`
			}
			w := httptest.NewRecorder()
			err := decodeJSON(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), &input, tc.limit)
			if tc.valid {
				if err != nil || input.Name != "ok" {
					t.Fatalf("decode = %q, %v", input.Name, err)
				}
			} else if !errors.Is(err, apperror.InvalidBody) {
				t.Fatalf("error = %v", err)
			}
			if w.Body.Len() != 0 || len(w.Header()) != 0 {
				t.Fatal("decoder wrote a response")
			}
		})
	}
}

func TestDecodeJSONPreservesPointerNull(t *testing.T) {
	// Login continuation rejects null at its HTTP boundary; decoding alone
	// preserves it so callers can distinguish it from an empty object.
	var input *struct{}
	if err := decodeJSON(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("null")), &input, 16); err != nil || input != nil {
		t.Fatal("null presence changed", err)
	}
	if err := decodeJSON(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("{}")), &input, 16); err != nil || input == nil {
		t.Fatal("empty object lost", err)
	}
}
