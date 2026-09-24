package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewProfilerHandler(t *testing.T) {
	handler := NewProfilerHandler()
	for _, test := range []struct{ path, body string }{
		{"/debug/pprof/", "Types of profiles available"},
		{"/debug/pprof/goroutine?debug=1", "goroutine profile:"},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), test.body) {
			t.Fatalf("%s: %d %s", test.path, w.Code, w.Body.String())
		}
	}
}
