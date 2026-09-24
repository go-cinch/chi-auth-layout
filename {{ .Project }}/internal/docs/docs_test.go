package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestModule(t *testing.T) {
	cfg, _, err := config.LoadDir("../../conf")
	if err != nil {
		t.Fatal(err)
	}
	module, err := New(cfg.HTTP.Docs.Servers)
	if err != nil {
		t.Fatal(err)
	}
	if module.Name() != "/docs" {
		t.Fatalf("Name() = %q", module.Name())
	}
	handler, err := server.NewRouter(&config.Config{}, nil, nil, module)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if recorder.Code != http.StatusMovedPermanently || recorder.Header().Get("Location") != "/docs/" {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("index response = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "openapi: 3.0.3") ||
		!strings.Contains(recorder.Body.String(), `https://example.com/api/{{ .Computed.service_name_kebab }}`) {
		t.Fatalf("spec response = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/docs/openapi.yaml", nil))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("HEAD response: %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/docs/", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method response = %d", recorder.Code)
	}
}
