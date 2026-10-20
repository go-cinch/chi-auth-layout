{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"{{ .Computed.module_name_final }}/internal/common/authn"

	"{{ .Computed.module_name_final }}/internal/common/config"
)

type testModule struct {
	name    string
	handler http.Handler
}

func (m testModule) Name() string { return m.name }

func (m testModule) HTTP(r *gin.RouterGroup) { r.GET("", gin.WrapH(m.handler)) }

func (m testModule) Public() bool { return true }

func TestNewRouter(t *testing.T) {
	var cfg config.Config
	cfg.Server.Name = "test"
	cfg.HTTP.Timeout = time.Second
	module := testModule{name: "/module", handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { WriteOK(w, map[string]bool{"ok": true}) })}
	handler, err := NewRouter(&cfg, nil, nil, module)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/module", 200}, {"POST", "/module", 405},
		{"GET", "/missing", 404}, {"GET", "/module/", 404},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s %s: %d", test.method, test.path, w.Code)
		}
		if w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("content type: %s", w.Header().Get("Content-Type"))
		}
		if test.status >= 400 {
			var body ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Msg != http.StatusText(test.status) {
				t.Fatalf("error body: %s", w.Body.String())
			}
		}
	}
	if _, err := NewRouter(&cfg, nil, nil, nil); err == nil {
		t.Fatal("nil module accepted")
	}
	if _, err := NewRouter(&cfg, nil, nil, testModule{name: "invalid"}); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := NewRouter(&cfg, nil, nil, module, module); err == nil {
		t.Fatal("duplicate module accepted")
	}
}

type groupedModule struct{}

func (groupedModule) Name() string { return "/team" }

func (groupedModule) Public() bool { return true }

func (groupedModule) HTTP(r *gin.RouterGroup) {
	group := r.Group("/:team")
	group.Use(func(c *gin.Context) {
		if c.Param("team") == "private" {
			c.Abort()
			WriteError(c.Writer, c.Request, 403, apperror.Forbidden)
			return
		}
		c.Next()
	})
	group.GET("/user/:id", func(c *gin.Context) {
		WriteOK(c.Writer, map[string]string{"team": c.Param("team"), "id": c.Param("id")})
	})
}

func TestNewRouterGroupedRoutes(t *testing.T) {
	handler, err := NewRouter(&config.Config{}, nil, nil, groupedModule{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path   string
		status int
		body   string
	}{
		{"/team/public/user/42", 200, `{"id":"42","team":"public"}`},
		{"/team/private/user/42", 403, `{"error_code":"HTTP_FORBIDDEN","msg":"Forbidden"}`},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
		if w.Code != test.status || w.Body.String() != test.body {
			t.Fatalf("response: %d %s", w.Code, w.Body.String())
		}
	}
}

type permissionTestModule struct {
	allowed          bool
	err              error
	calls            int
	lastMethod       string
	lastPath         string
	permissionSuffix string
}

func (*permissionTestModule) Name() string { return "/auth" }

func (m *permissionTestModule) PermissionEndpoint() string {
	if m.permissionSuffix != "" {
		return m.permissionSuffix
	}
	return "/permission"
}

func (m *permissionTestModule) AuthorizeHTTP(_ context.Context, method, path string) (bool, error) {
	m.calls++
	m.lastMethod, m.lastPath = method, path
	return m.allowed, m.err
}

func (*permissionTestModule) HTTP(r *gin.RouterGroup) {
	r.GET("/permission", func(c *gin.Context) { WriteOK(c.Writer) })
}

type protectedTestModule struct{}

func (protectedTestModule) Name() string { return "/private" }

func (protectedTestModule) HTTP(r *gin.RouterGroup) {
	r.GET("", func(c *gin.Context) { WriteOK(c.Writer) })
}

func TestHTTPPermissionAuthorization(t *testing.T) {
	authenticator, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authenticator.Issue(authn.Identity{CredentialVersion: 1, UserID: 7, Username: "guest", Code: "CODE0007"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Auth.Authorization.Enabled = true
	authorizer := &permissionTestModule{}
	handler, err := NewRouter(&cfg, authenticator, nil, authorizer, protectedTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request("/private", false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", got)
	}
	if got := request("/private", true).Code; got != http.StatusForbidden {
		t.Fatalf("denied = %d", got)
	}
	if authorizer.calls != 1 || authorizer.lastMethod != http.MethodGet || authorizer.lastPath != "/private" {
		t.Fatalf("authorization call = %#v", authorizer)
	}
	authorizer.allowed = true
	if got := request("/private", true).Code; got != http.StatusOK {
		t.Fatalf("allowed = %d", got)
	}
	authorizer.err = errors.New("database unavailable")
	if got := request("/private", true).Code; got != http.StatusInternalServerError {
		t.Fatalf("failure = %d", got)
	}
	authorizer.err = nil
	if got := request("/auth/permission", true).Code; got != http.StatusOK {
		t.Fatalf("permission endpoint = %d", got)
	}
	if authorizer.calls != 3 {
		t.Fatalf("permission endpoint authorized recursively: %d", authorizer.calls)
	}

	if _, err := NewRouter(&cfg, authenticator, nil, protectedTestModule{}); err == nil {
		t.Fatal("missing authorizer accepted")
	}
	if _, err := NewRouter(&cfg, authenticator, nil, authorizer, &permissionTestModule{}); err == nil {
		t.Fatal("duplicate authorizer accepted")
	}
	if _, err := NewRouter(&cfg, authenticator, nil, &permissionTestModule{permissionSuffix: "invalid"}); err == nil {
		t.Fatal("invalid permission endpoint accepted")
	}
}

{{ else -}}
package server

import (
	"encoding/json"

	"bytes"
	"context"
	"errors"
	"log/slog"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"

	"{{ .Computed.module_name_final }}/internal/common/config"

	"github.com/go-chi/chi/v5"
)

type testModule struct {
	name    string
	handler http.Handler
}

func (m testModule) Name() string {
	return m.name
}

func (m testModule) HTTP() http.Handler {
	return m.handler
}

func (m testModule) Public() bool { return true }

func TestNewRouter(t *testing.T) {
	var cfg config.Config
	cfg.Server.Name = "test"
	cfg.HTTP.Timeout = time.Second
	module := testModule{name: "/module", handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteOK(w, map[string]bool{"ok": true})
	})}
	handler, err := NewRouter(&cfg, nil, nil, module)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/module", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if _, err := NewRouter(&config.Config{}, nil, nil, testModule{name: "invalid", handler: http.NotFoundHandler()}); err == nil {
		t.Fatal("invalid module was accepted")
	}
	if _, err := NewRouter(&config.Config{}, nil, nil, nil); err == nil {
		t.Fatal("nil module was accepted")
	}
	if _, err := NewRouter(&config.Config{}, nil, nil, testModule{name: "/nil-handler"}); err == nil {
		t.Fatal("nil module handler was accepted")
	}
	if _, err := NewRouter(&config.Config{}, nil, nil, module, module); err == nil {
		t.Fatal("duplicate module was accepted")
	}
}

type permissionTestModule struct {
	allowed          bool
	err              error
	calls            int
	lastMethod       string
	lastPath         string
	permissionSuffix string
}

func (*permissionTestModule) Name() string { return "/auth" }

func (m *permissionTestModule) PermissionEndpoint() string {
	if m.permissionSuffix != "" {
		return m.permissionSuffix
	}
	return "/permission"
}

func (m *permissionTestModule) AuthorizeHTTP(_ context.Context, method, path string) (bool, error) {
	m.calls++
	m.lastMethod, m.lastPath = method, path
	return m.allowed, m.err
}

func (*permissionTestModule) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/permission", func(w http.ResponseWriter, _ *http.Request) { WriteOK(w) })
	return r
}

type protectedTestModule struct{}

func (protectedTestModule) Name() string { return "/private" }

func (protectedTestModule) HTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { WriteOK(w) })
}

func TestHTTPPermissionAuthorization(t *testing.T) {
	authenticator, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authenticator.Issue(authn.Identity{CredentialVersion: 1, UserID: 7, Username: "guest", Code: "CODE0007"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Auth.Authorization.Enabled = true
	authorizer := &permissionTestModule{}
	handler, err := NewRouter(&cfg, authenticator, nil, authorizer, protectedTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request("/private", false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", got)
	}
	if got := request("/private", true).Code; got != http.StatusForbidden {
		t.Fatalf("denied = %d", got)
	}
	if authorizer.calls != 1 || authorizer.lastMethod != http.MethodGet || authorizer.lastPath != "/private" {
		t.Fatalf("authorization call = %#v", authorizer)
	}
	authorizer.allowed = true
	if got := request("/private", true).Code; got != http.StatusOK {
		t.Fatalf("allowed = %d", got)
	}
	authorizer.err = errors.New("database unavailable")
	if got := request("/private", true).Code; got != http.StatusInternalServerError {
		t.Fatalf("failure = %d", got)
	}
	authorizer.err = nil
	if got := request("/auth/permission", true).Code; got != http.StatusOK {
		t.Fatalf("permission endpoint = %d", got)
	}
	if authorizer.calls != 3 {
		t.Fatalf("permission endpoint authorized recursively: %d", authorizer.calls)
	}

	if _, err := NewRouter(&cfg, authenticator, nil, protectedTestModule{}); err == nil {
		t.Fatal("missing authorizer accepted")
	}
	if _, err := NewRouter(&cfg, authenticator, nil, authorizer, &permissionTestModule{}); err == nil {
		t.Fatal("duplicate authorizer accepted")
	}
	if _, err := NewRouter(&cfg, authenticator, nil, &permissionTestModule{permissionSuffix: "invalid"}); err == nil {
		t.Fatal("invalid permission endpoint accepted")
	}
}

{{ end -}}

type rootPathTestModule struct{}

func (rootPathTestModule) Name() string { return "" }

func (rootPathTestModule) HTTPPathConfig() string { return "auth.path" }

func (rootPathTestModule) Public() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (rootPathTestModule) HTTP(r *gin.RouterGroup) {
	r.Use(func(c *gin.Context) { c.Header("X-Module-Middleware", "true"); c.Next() })
	r.GET("/probe/:id", func(c *gin.Context) { c.Header("X-Route-Middleware", "true"); c.Next() }, func(c *gin.Context) { WriteOK(c.Writer, map[string]string{"route": c.FullPath(), "id": c.Param("id")}) })
}
{{ else -}}
func (rootPathTestModule) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Module-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	})
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Route-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}
	r.With(middleware).Get("/probe/{id}", func(w http.ResponseWriter, r *http.Request) {
		WriteOK(w, map[string]string{"route": chi.RouteContext(r.Context()).RoutePattern(), "id": chi.URLParam(r, "id")})
	})
	return r
}
{{ end }}

func TestConfiguredRootModulePreservesRouteAndMiddleware(t *testing.T) {
	handler, err := NewRouter(&config.Config{}, nil, nil, rootPathTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/probe/42", nil))
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	expected := {{ if eq .Computed.http_router_final "gin" }}"/probe/:id"{{ else }}"/probe/{id}"{{ end }}
	if response.Code != 200 || body["route"] != expected || body["id"] != "42" || response.Header().Get("X-Module-Middleware") != "true" || response.Header().Get("X-Route-Middleware") != "true" {
		t.Fatalf("root route lost routing context or middleware: %d %v %v", response.Code, body, response.Header())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/missing", nil))
	if response.Code != 404 {
		t.Fatal(response.Code)
	}
}

type authenticationFailureValidator struct{ err error }

func (v authenticationFailureValidator) ValidateIdentity(context.Context, authn.Identity) error {
	return v.err
}

func TestHTTPAuthenticationFailureLogging(t *testing.T) {
	failure := errors.New("authentication storage unavailable")
	manager, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour, authenticationFailureValidator{failure})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewRouter(&config.Config{}, manager, nil, protectedTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error_code":"HTTP_INTERNAL","msg":"internal server error"}` {
		t.Fatalf("authentication failure response = %d %s", response.Code, response.Body.String())
	}
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if record["operation"] != "authenticate_http_request" {
			continue
		}
		found = true
		if record["level"] != "ERROR" || record["outcome"] != "failure" || record["method"] != "GET" || record["error"] != failure.Error() || record["msg"] != "authenticate http request failed: "+failure.Error() {
			t.Fatalf("authentication failure log = %#v", record)
		}
	}
	if !found {
		t.Fatal("authentication storage failure was not logged")
	}
	if bytes.Contains(logs.Bytes(), []byte(token)) {
		t.Fatal("authentication log contains the bearer token")
	}
}
