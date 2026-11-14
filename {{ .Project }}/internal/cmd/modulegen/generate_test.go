package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesModules(t *testing.T) {
	root := t.TempDir()
	goMod := writeTestFile(t, root, "go.mod", "module example.com/service\n")
	appFile := writeTestFile(t, root, "internal/app/app.go", `package app
import "example.com/service/internal/infra/db"
type Application struct {
	db *db.Store
	cleanups []func()
}
`)
	modulesDir := filepath.Join(root, "internal/modules")
	writeTestFile(t, root, "internal/modules/module.go", "package modules\n")
	writeTestFile(t, root, "internal/modules/notes/notes.go", "package notes\n")
	writeTestFile(t, root, "internal/modules/user/user.go", `package user
import (
	"example.com/service/internal/infra/db"
	"example.com/service/internal/modules"
)
type Module struct{}
var _ modules.Module = (*Module)(nil)
func New(store *db.Store) *Module { return &Module{} }
`)
	output := filepath.Join(root, "internal/app/modules.gen.go")
	if err := run(modulesDir, appFile, goMod, output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), `user.New(a.db)`) || !strings.Contains(string(generated), `example.com/service/internal/modules/user`) {
		t.Fatalf("generated modules:\n%s", generated)
	}
	if err := run(modulesDir, appFile, goMod, output); err != nil {
		t.Fatalf("unchanged run() error = %v", err)
	}
}

func TestRunReusesInitializedModule(t *testing.T) {
	for _, fields := range []string{"dictionary", "dictionary, duplicate"} {
		t.Run(fields, func(t *testing.T) {
			root := t.TempDir()
			goMod := writeTestFile(t, root, "go.mod", "module example.com/service\n")
			appFile := writeTestFile(t, root, "internal/app/app.go", `package app
import dictionarymodule "example.com/service/internal/modules/dictionary"
type Application struct { `+fields+` *dictionarymodule.Module }
`)
			modulesDir := filepath.Join(root, "internal/modules")
			writeTestFile(t, root, "internal/modules/dictionary/dictionary.go", `package dictionary
import (
	"net/http"
	"example.com/service/internal/modules"
)
type Module struct{}
var _ modules.HTTPModule = (*Module)(nil)
func New(client *http.Client) (*Module, error) { return &Module{}, nil }
`)
			output := filepath.Join(root, "modules.gen.go")
			err := run(modulesDir, appFile, goMod, output)
			if strings.Contains(fields, ",") {
				if err == nil || !strings.Contains(err.Error(), "matches multiple Application fields") {
					t.Fatalf("ambiguous instance error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			generated, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			text := string(generated)
			if !strings.Contains(text, "module0 := a.dictionary") || !strings.Contains(text, "httpModules = append(httpModules, module0)") || strings.Contains(text, "dictionary.New") || strings.Contains(text, "internal/modules/dictionary") {
				t.Fatalf("initialized module registration:\n%s", text)
			}
		})
	}
}

func TestRunRejectsMissingDependency(t *testing.T) {
	root := t.TempDir()
	goMod := writeTestFile(t, root, "go.mod", "module example.com/service\n")
	appFile := writeTestFile(t, root, "internal/app/app.go", "package app\ntype Application struct{}\n")
	modulesDir := filepath.Join(root, "internal/modules")
	writeTestFile(t, root, "internal/modules/payment/payment.go", `package payment
import (
	"net/http"
	"example.com/service/internal/modules"
)
type Module struct{}
var _ modules.Module = (*Module)(nil)
func New(client *http.Client) *Module { return &Module{} }
`)
	err := run(modulesDir, appFile, goMod, filepath.Join(root, "modules.gen.go"))
	if err == nil || !strings.Contains(err.Error(), "has no Application field") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunMatchesModuleLocalDependency(t *testing.T) {
	root := t.TempDir()
	goMod := writeTestFile(t, root, "go.mod", "module example.com/service\n")
	appFile := writeTestFile(t, root, "internal/app/app.go", `package app
import user "example.com/service/internal/modules/user"
type Application struct { credentials *user.Credentials }
`)
	modulesDir := filepath.Join(root, "internal/modules")
	writeTestFile(t, root, "internal/modules/module.go", "package modules\n")
	writeTestFile(t, root, "internal/modules/user/user.go", `package user
import "example.com/service/internal/modules"
type Credentials struct{}
type Module struct{}
var _ modules.Module = (*Module)(nil)
func New(credentials *Credentials) *Module { return &Module{} }
`)
	output := filepath.Join(root, "internal/app/modules.gen.go")
	if err := run(modulesDir, appFile, goMod, output); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "user.New(a.credentials)") {
		t.Fatalf("generated modules:\n%s", generated)
	}
}

func TestInvalidInputs(t *testing.T) {
	root := t.TempDir()
	goMod := writeTestFile(t, root, "go.mod", "go 1.27.1\n")
	if _, err := readModulePath(goMod); err == nil {
		t.Fatal("module-less go.mod was accepted")
	}
	if _, err := applicationFields(writeTestFile(t, root, "app.go", "package app\ntype Other struct{}\n")); err == nil {
		t.Fatal("missing Application was accepted")
	}
	if _, err := applicationFields(writeTestFile(t, root, "bad.go", "not go")); err == nil {
		t.Fatal("invalid Go was accepted")
	}
}

func writeTestFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

{{ if .Computed.enable_grpc_final -}}
func TestTransportCapabilities(t *testing.T) {
	root := t.TempDir()
	goMod := writeTestFile(t, root, "go.mod", "module example.com/service\n")
	app := writeTestFile(t, root, "internal/app/app.go", "package app\ntype Application struct{}\n")
	modulesDir := filepath.Join(root, "internal/modules")
	writeTestFile(t, root, "internal/modules/module.go", "package modules\n")
	for _, item := range []struct{ name, assertions string }{
		{"web", "var _ modules.HTTPModule = (*Module)(nil)"},
		{"rpc", "var _ modules.GRPCModule = (*Module)(nil)"},
		{"both", "var _ modules.HTTPModule = (*Module)(nil)\nvar _ modules.GRPCModule = (*Module)(nil)"},
	} {
		writeTestFile(t, root, "internal/modules/"+item.name+"/module.go", "package "+item.name+"\nimport \"example.com/service/internal/modules\"\ntype Module struct{}\n"+item.assertions+"\nfunc New()*Module{return &Module{}}\n")
	}
	output := filepath.Join(root, "modules.gen.go")
	if err := run(modulesDir, app, goMod, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "both.New()") != 1 || strings.Count(text, "httpModules = append") != 2 || strings.Count(text, "grpcModules = append") != 2 {
		t.Fatalf("registry: %s", text)
	}
	if err := os.RemoveAll(filepath.Join(modulesDir, "rpc")); err != nil {
		t.Fatal(err)
	}
	if err := run(modulesDir, app, goMod, output); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(output)
	if strings.Contains(string(data), "rpc.New()") {
		t.Fatalf("stale registration: %s", data)
	}
}
{{ end -}}
