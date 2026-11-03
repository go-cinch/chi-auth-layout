package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestJSONHelpers(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteOK(recorder)
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("empty response = %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	WriteError(recorder, httptest.NewRequest("GET", "/", nil), http.StatusBadRequest, apperror.InvalidBody)
	var body ErrorResponse
	if recorder.Code != http.StatusBadRequest || json.Unmarshal(recorder.Body.Bytes(), &body) != nil || body.Msg != "invalid request body" {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	WriteNoContent(recorder)
	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Fatalf("no content response = %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	WriteJSON(recorder, http.StatusOK, func() {})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("encoding failure status = %d", recorder.Code)
	}
}

func TestPasswordResetErrorHeader(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{"required", authn.ErrPasswordResetRequired, http.StatusForbidden, "true"},
		{"wrapped", fmt.Errorf("restricted: %w", authn.ErrPasswordResetRequired), http.StatusForbidden, "true"},
		{"unauthorized", apperror.Unauthorized, http.StatusUnauthorized, ""},
		{"forbidden", apperror.Forbidden, http.StatusForbidden, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			WriteError(response, httptest.NewRequest("GET", "/msg/unread-count", nil), test.status, test.err)
			if got := response.Header().Get(authn.PasswordResetRequiredHeader); got != test.want {
				t.Fatalf("reset header = %q, want %q", got, test.want)
			}
			var body ErrorResponse
			if response.Code != test.status || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.ErrorCode != apperror.Code(test.err) {
				t.Fatal("error response contract changed")
			}
			if test.want != "" && response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("reset requirement response must not be cached")
			}
		})
	}
}
