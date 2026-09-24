package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGinRoutesAndResponses(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
	writeSource(t, root, "user/http.go", `package user
func (*Module) Name() string { return "/user" }
func (m *Module) HTTP(r *gin.RouterGroup) {
    r.GET("/:id", auth, m.get)
    v1 := r.Group("/v1")
    nested := v1.Group("/:team")
    nested.POST("/member", auth, m.create)
    { v1 := r.Group("/v2"); v1.HEAD("/ready", func(c *gin.Context){ c.Status(204) }) }
    v1.GET("/ping", m.ping)
    r.Handle(http.MethodDelete, "/:id", m.remove)
    r.GET("/file/*filepath", m.ping)
    unrelated.GET("/not-a-route", m.ping)
}
func (m *Module) get(c *gin.Context) {
    id, err := strconv.ParseInt(c.Param("id"),10,64)
    if err != nil { server.WriteError(c.Writer,400,"invalid id"); return }
    value, err := m.Find(c.Request.Context(),id)
    server.WriteOK(c.Writer,value)
}
type User struct { ID int64 }
func (m *Module) Find(context.Context,int64) (*User,error) { return nil,nil }
func (m *Module) create(c *gin.Context) {
    c.JSON(http.StatusCreated, User{})
    c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg":"bad input"})
}
func (m *Module) ping(c *gin.Context) { c.JSON(200,gin.H{"ok":true}) }
func (m *Module) remove(c *gin.Context) { c.AbortWithStatus(204) }
`)
	writeSource(t, root, "server/health.go", `package server
func (*Health) Name() string { return "/healthz" }
func (h *Health) HTTP(r *gin.RouterGroup) { r.GET("", gin.WrapH(h.handler())) }
func (h *Health) handler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.Method != http.MethodGet { WriteError(w,405,"Method Not Allowed"); return }
        WriteOK(w)
        WriteError(w,503,"Service Unavailable")
    })
}
`)
	output := filepath.Join(root, "openapi.yaml")
	if err := run(root, filepath.Join(root, "conf"), output, "Gin API"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	for _, expected := range []string{
		"/user/{id}:", "format: int64", "/user/v1/{team}/member:", "/user/v2/ready:", "/user/v1/ping:",
		"delete:", "/user/file/{filepath}:", "/healthz:", `"503":`, `"201":`, `"400":`, `"204":`,
		`#/components/schemas/User`, "msg:", "ok:", "type: boolean",
	} {
		if !strings.Contains(generated, expected) {
			t.Fatalf("missing %q:\n%s", expected, generated)
		}
	}
	if strings.Contains(generated, "not-a-route") || strings.Contains(generated, "/user/v2/ping") {
		t.Fatalf("incorrect scope:\n%s", generated)
	}
}

func TestChiNestedRoutes(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "user/http.go", `package user
func (*Module) Name() string { return "/user" }
func (m *Module) HTTP() http.Handler {
    r := chi.NewRouter()
    r.Route("/{team}",func(r chi.Router){
        r.With(auth).Get("/{id}", m.get)
        r.Group(func(r chi.Router){ r.Post("/member", m.get) })
    })
    r.Get("/flat",m.get)
    r.MethodFunc(http.MethodPut,"/flat",m.get)
    return r
}
func (m *Module) get(w http.ResponseWriter,r *http.Request){ server.WriteOK(w) }
`)
	packages, err := loadPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	routes := discoverRoutes(packages)
	got := make(map[string]bool)
	for _, route := range routes {
		got[route.method+" "+route.path] = true
	}
	for _, expected := range []string{"GET /user/{team}/{id}", "POST /user/{team}/member", "GET /user/flat", "PUT /user/flat"} {
		if !got[expected] {
			t.Fatalf("missing %q in %#v", expected, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("routes: %#v", got)
	}
}
