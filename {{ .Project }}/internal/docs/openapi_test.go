package docs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/knadh/koanf/parsers/yaml"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestOpenAPIServersUseRuntimeEnvironment(t *testing.T) {
	dir := t.TempDir()
	data := `http:
  docs:
    servers:
      - url: "https://prod.example"
        description: "prod"
      - url: "https://dev.example"
        description: "dev"
      - url: "http://127.0.0.1:8080"
        description: "local"
`
	if err := os.WriteFile(filepath.Join(dir, "http.yml"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_2_URL", "http://127.0.0.1:8083")
	cfg, _, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	module, err := New(cfg.HTTP.Docs.Servers)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := server.NewRouter(cfg, nil, nil, module)
	if err != nil {
		t.Fatal(err)
	}
	// The module owns a rendered snapshot, not the caller's mutable slice.
	cfg.HTTP.Docs.Servers[2].URL = "https://changed.example"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/yaml" {
		t.Fatalf("response: %d %v", w.Code, w.Header())
	}
	document, err := yaml.Parser().Unmarshal(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	servers := document["servers"].([]any)
	for i, want := range []string{"https://prod.example", "https://dev.example", "http://127.0.0.1:8083"} {
		if servers[i].(map[string]any)["url"] != want {
			t.Fatalf("servers: %#v", servers)
		}
	}
	if len(servers) != 3 || servers[2].(map[string]any)["description"] != "local" {
		t.Fatalf("servers: %#v", servers)
	}
	embedded, err := files.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	original, err := yaml.Parser().Unmarshal(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original["paths"], document["paths"]) || !reflect.DeepEqual(original["components"], document["components"]) {
		t.Fatal("runtime rendering changed API definitions")
	}
	empty, err := renderOpenAPI(embedded, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := yaml.Parser().Unmarshal(empty)
	if err != nil || len(cleared["servers"].([]any)) != 0 {
		t.Fatalf("empty server list: %s, %v", empty, err)
	}
	if _, err := renderOpenAPI([]byte("openapi: ["), nil); err == nil {
		t.Fatal("malformed document accepted")
	}
}

func TestRuntimePagination(t *testing.T) {
	document := []byte(`openapi: 3.0.3
paths:
  /resource:
    get:
      parameters:
        - name: p
          in: query
          schema: {type: integer, x-pagination: p, description: "Values outside 1..10000 return an empty list.", default: 1}
        - name: s
          in: query
          schema: {type: integer, x-pagination: s, description: "Values outside 1..10000 return an empty list.", default: 1}
`)
	limits, err := pagination.New(3, 5)
	if err != nil {
		t.Fatal(err)
	}
	data, err := renderOpenAPI(document, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := yaml.Parser().Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	parameters := parsed["paths"].(map[string]any)["/resource"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
	p := parameters[0].(map[string]any)["schema"].(map[string]any)
	s := parameters[1].(map[string]any)["schema"].(map[string]any)
	if p["maximum"] != nil || s["maximum"] != nil || p["description"] != "Values outside 1..3 return an empty list." || s["description"] != "Values outside 1..5 return an empty list." || s["default"] != 1 {
		t.Fatalf("runtime pagination: %s", data)
	}
}
