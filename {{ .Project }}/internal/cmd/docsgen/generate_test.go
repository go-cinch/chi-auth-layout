package main

import (
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

func TestAuthInfoUsesHeaderWithoutScopeQuery(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
	source := `package auth
 type Module struct{}
 func (*Module) Name() string { return "/" }
 func (m *Module) HTTP() http.Handler {r:=chi.NewRouter();r.Get("/info",m.info);return r}
 func (m *Module) info(w http.ResponseWriter,r *http.Request) {server.WriteOK(w)}
 `
	writeSource(t, root, "auth/http.go", source)
	output := filepath.Join(root, "openapi.yaml")
	if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "name: scope") || !strings.Contains(string(encoded), "name: X-Auth-Scope") {
		t.Fatalf("info must document only the scope header: %s", encoded)
	}
}

func TestProtectedErrorsPreserveHandlerDescription(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
	writeSource(t, root, "sample/http.go", `package sample
 var errDenied=apperror.New("SAMPLE_DENIED","custom denial reason")
 type Module struct{}
 func (*Module) Name() string{return "/sample"}
 func(m *Module)HTTP()http.Handler{r:=chi.NewRouter();r.Get("/",m.get);r.Post("/",m.create);return r}
 func(m *Module)get(w http.ResponseWriter,r *http.Request){server.WriteOK(w)}
 func(m *Module)create(w http.ResponseWriter,r *http.Request){server.WriteError(w,r,http.StatusForbidden,errDenied)}
 `)
	output := filepath.Join(root, "openapi.yaml")
	if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(encoded)
	for _, wanted := range []string{"invalid permission scope header", "permission denied", "custom denial reason", "X-Auth-Scope"} {
		if !strings.Contains(doc, wanted) {
			t.Fatalf("missing %s: %s", wanted, doc)
		}
	}
	if strings.Contains(doc, "custom denial reason; permission denied") {
		t.Fatal("handler denial description overwritten")
	}
}
