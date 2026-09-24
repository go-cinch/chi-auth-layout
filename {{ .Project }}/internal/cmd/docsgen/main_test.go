package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesResponses(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "conf")
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers:\n      - url: https://example.com/api/example\n        description: prod\n")
	writeSource(t, root, "modules/user/user.go", `package user
import (
    "context"
    "errors"
    "time"
)
var ErrNotFound = errors.New("user not found")
type Module struct{}
type User struct {
    ID int64 `+"`json:\"id\"`"+`
    CreatedAt time.Time `+"`json:\"created_at\"`"+`
    Name string `+"`json:\"name\"`"+`
}
func (*Module) Name() string { return "/user" }
func (*Module) Idempotent() bool { return true }
func (*Module) Find(context.Context, int64) (*User, error) { return nil, nil }
`)
	writeSource(t, root, "modules/user/http.go", `package user
import (
    "net/http"
    "strconv"
)
func (m *Module) HTTP() http.Handler {
    r := chi.NewRouter()
    r.Post("/", m.create)
    r.Get("/{id}", m.get)
    return r
}
func (m *Module) create(w http.ResponseWriter, r *http.Request) { server.WriteOK(w, User{}) }
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil { server.WriteError(w, http.StatusBadRequest, "invalid user id"); return }
    if id <= 0 { server.WriteError(w, http.StatusBadRequest, "user id must be positive"); return }
    value, err := m.Find(r.Context(), id)
    if errors.Is(err, ErrNotFound) { server.WriteError(w, http.StatusNotFound, ErrNotFound.Error()); return }
    server.WriteOK(w, value)
}
`)
	writeSource(t, root, "common/server/health.go", `package server
import "net/http"
type Health struct{}
func (*Health) Name() string { return "/healthz" }
func (h *Health) HTTP() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet { WriteError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)); return }
        WriteOK(w)
    })
}
`)
	output := filepath.Join(root, "docs", "openapi.yaml")
	if err := run(root, configDir, output, "Example API"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	for _, expected := range []string{
		`title: "Example API"`, `url: "https://example.com/api/example"`, `description: "prod"`, "/healthz:", "/user/{id}:", `operationId: getUserId`,
		`"400":`, `description: "invalid user id; user id must be positive"`, `"404":`, `description: "user not found"`,
		"User:", "created_at:", "format: date-time", "ErrorResponse:",
		"name: X-Idempotent", "in: header", `description: "idempotency key has already been used"`,
	} {
		if !strings.Contains(generated, expected) {
			t.Fatalf("generated OpenAPI missing %q:\n%s", expected, generated)
		}
	}
	if err := run(root, configDir, output, "Example API"); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithoutRoutesAndInvalidSource(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "conf")
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
	writeSource(t, root, "empty/value.go", "package empty\ntype Value string\n")
	output := filepath.Join(root, "openapi.yaml")
	if err := run(root, configDir, output, "Empty API"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || !strings.Contains(string(data), "paths: {}") {
		t.Fatalf("empty document = %s, %v", data, err)
	}
	if err := run(filepath.Join(root, "missing"), configDir, output, "Invalid"); err == nil {
		t.Fatal("missing source was accepted")
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(file, configDir, output, "Invalid"); err == nil {
		t.Fatal("file source was accepted")
	}
	if err := run(root, filepath.Join(root, "missing-conf"), output, "Invalid"); err == nil {
		t.Fatal("missing config was accepted")
	}
}

func TestHelpers(t *testing.T) {
	expression, err := parser.ParseExpr("http.StatusServiceUnavailable")
	if err != nil || statusCode(expression) != 503 {
		t.Fatalf("status = %d, %v", statusCode(expression), err)
	}
	if actual := pathParameters("/team/{team}/user/{id}"); strings.Join(actual, ",") != "team,id" {
		t.Fatalf("parameters = %v", actual)
	}
	if operationID("POST", "/user/{id}/password") != "postUserIdPassword" {
		t.Fatal("unexpected operation ID")
	}
	if primitiveSchema("float64").format != "double" || primitiveSchema("unknown") != nil {
		t.Fatal("unexpected primitive schema")
	}
}

func writeSource(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAPIGenerationUsesIndexedEnvironment(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "conf/http.yml", `http:
  docs:
    servers:
      - url: "https://prod.example"
        description: "prod"
      - url: "https://dev.example"
        description: "dev"
      - url: "http://127.0.0.1:8080"
        description: "local"
`)
	writeSource(t, root, "empty/value.go", "package empty\ntype Value string\n")
	t.Setenv("SERVICE_HTTP_DOCS_SERVERS_2_URL", "http://127.0.0.1:8083")
	output := filepath.Join(root, "openapi.yaml")
	if err := run(root, filepath.Join(root, "conf"), output, "Environment API"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"https://prod.example", "https://dev.example", "http://127.0.0.1:8083", `description: "local"`} {
		if !strings.Contains(string(data), value) {
			t.Fatalf("missing %q:\n%s", value, data)
		}
	}
	if strings.Contains(string(data), "http://127.0.0.1:8080") {
		t.Fatalf("old server URL remained:\n%s", data)
	}
}

func TestErrorDescriptionsAreUnique(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", `package example
func handler(w http.ResponseWriter,r *http.Request){
 server.WriteError(w,400,"invalid query")
 server.WriteError(w,400,"invalid p")
 server.WriteError(w,400,"invalid p")
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &generator{components: map[string]*schema{}, visiting: map[string]bool{}}
	value := route{handler: file.Decls[0].(*ast.FuncDecl), pkg: &sourcePackage{}}
	g.describeRoute(&value)
	if got := value.responses[400].description; got != "invalid query; invalid p" {
		t.Fatal(got)
	}
}
