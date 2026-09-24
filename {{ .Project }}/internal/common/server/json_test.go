package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

func TestJSONHelpers(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteOK(recorder)
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("empty response = %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	WriteError(recorder, httptest.NewRequest("GET", "/", nil), http.StatusBadRequest, apperror.BadRequest)
	var body ErrorResponse
	if recorder.Code != http.StatusBadRequest || json.Unmarshal(recorder.Body.Bytes(), &body) != nil || body.Msg != "Bad Request" {
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
