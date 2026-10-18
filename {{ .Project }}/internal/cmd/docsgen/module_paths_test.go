package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredModulePaths(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/identity/v2"} {
		t.Run(prefix, func(t *testing.T) {
			root := t.TempDir()
			writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
			writeSource(t, root, "conf/auth.yml", "auth:\n  path: \""+prefix+"\"\n")
			writeSource(t, root, "auth/http.go", `package auth
type Module struct{ path string }
func(m *Module)Name()string{return m.path}
func(*Module)HTTPPathConfig()string{return "auth.path"}
func(m *Module)HTTP()http.Handler{r:=chi.NewRouter();r.Get("/info",m.info);r.Post("/pub/login",m.login);return r}
func(m *Module)info(w http.ResponseWriter,r *http.Request){server.WriteOK(w)}
func(m *Module)login(w http.ResponseWriter,r *http.Request){server.WriteOK(w)}
`)
			output := filepath.Join(root, "openapi.yaml")
			if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
				t.Fatal(err)
			}
			doc, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{prefix + "/info", prefix + "/pub/login"} {
				if !strings.Contains(string(doc), "  "+path+":") {
					t.Fatal(string(doc))
				}
			}
			t.Setenv("SERVICE_AUTH_PATH", "/configured")
			if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
				t.Fatal(err)
			}
			doc, _ = os.ReadFile(output)
			if !strings.Contains(string(doc), "  /configured/info:") {
				t.Fatal(string(doc))
			}
			t.Setenv("SERVICE_AUTH_PATH", "")
			if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
				t.Fatal(err)
			}
			doc, _ = os.ReadFile(output)
			if !strings.Contains(string(doc), "  /info:") || strings.Contains(string(doc), "  /configured/info:") {
				t.Fatal(string(doc))
			}
			t.Setenv("SERVICE_AUTH_PATH", "invalid")
			if err := run(root, filepath.Join(root, "conf"), output, "test"); err == nil {
				t.Fatal("accepted invalid path")
			}
		})
	}
}
