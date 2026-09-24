{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	
	"context"
	"errors"
	
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
	if m.permissionSuffix != "" { return m.permissionSuffix }
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
	if err != nil { t.Fatal(err) }
	token, _, err := authenticator.Issue(authn.Identity{CredentialVersion: 1, UserID: 7, Username: "guest", Code: "CODE0007"})
	if err != nil { t.Fatal(err) }
	var cfg config.Config
	cfg.Auth.Authorization.Enabled = true
	authorizer := &permissionTestModule{}
	handler, err := NewRouter(&cfg, authenticator, nil, authorizer, protectedTestModule{})
	if err != nil { t.Fatal(err) }
	request := func(path string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authenticated { req.Header.Set("Authorization", "Bearer "+token) }
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request("/private", false).Code; got != http.StatusUnauthorized { t.Fatalf("unauthenticated = %d", got) }
	if got := request("/private", true).Code; got != http.StatusForbidden { t.Fatalf("denied = %d", got) }
	if authorizer.calls != 1 || authorizer.lastMethod != http.MethodGet || authorizer.lastPath != "/private" { t.Fatalf("authorization call = %#v", authorizer) }
	authorizer.allowed = true
	if got := request("/private", true).Code; got != http.StatusOK { t.Fatalf("allowed = %d", got) }
	authorizer.err = errors.New("database unavailable")
	if got := request("/private", true).Code; got != http.StatusInternalServerError { t.Fatalf("failure = %d", got) }
	authorizer.err = nil
	if got := request("/auth/permission", true).Code; got != http.StatusOK { t.Fatalf("permission endpoint = %d", got) }
	if authorizer.calls != 3 { t.Fatalf("permission endpoint authorized recursively: %d", authorizer.calls) }

	if _, err := NewRouter(&cfg, authenticator, nil, protectedTestModule{}); err == nil { t.Fatal("missing authorizer accepted") }
	if _, err := NewRouter(&cfg, authenticator, nil, authorizer, &permissionTestModule{}); err == nil { t.Fatal("duplicate authorizer accepted") }
	if _, err := NewRouter(&cfg, authenticator, nil, &permissionTestModule{permissionSuffix: "invalid"}); err == nil { t.Fatal("invalid permission endpoint accepted") }
}

{{ else -}}
package server

import (
	
	"context"
	"errors"
	
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
	if m.permissionSuffix != "" { return m.permissionSuffix }
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
	if err != nil { t.Fatal(err) }
	token, _, err := authenticator.Issue(authn.Identity{CredentialVersion: 1, UserID: 7, Username: "guest", Code: "CODE0007"})
	if err != nil { t.Fatal(err) }
	var cfg config.Config
	cfg.Auth.Authorization.Enabled = true
	authorizer := &permissionTestModule{}
	handler, err := NewRouter(&cfg, authenticator, nil, authorizer, protectedTestModule{})
	if err != nil { t.Fatal(err) }
	request := func(path string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authenticated { req.Header.Set("Authorization", "Bearer "+token) }
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request("/private", false).Code; got != http.StatusUnauthorized { t.Fatalf("unauthenticated = %d", got) }
	if got := request("/private", true).Code; got != http.StatusForbidden { t.Fatalf("denied = %d", got) }
	if authorizer.calls != 1 || authorizer.lastMethod != http.MethodGet || authorizer.lastPath != "/private" { t.Fatalf("authorization call = %#v", authorizer) }
	authorizer.allowed = true
	if got := request("/private", true).Code; got != http.StatusOK { t.Fatalf("allowed = %d", got) }
	authorizer.err = errors.New("database unavailable")
	if got := request("/private", true).Code; got != http.StatusInternalServerError { t.Fatalf("failure = %d", got) }
	authorizer.err = nil
	if got := request("/auth/permission", true).Code; got != http.StatusOK { t.Fatalf("permission endpoint = %d", got) }
	if authorizer.calls != 3 { t.Fatalf("permission endpoint authorized recursively: %d", authorizer.calls) }

	if _, err := NewRouter(&cfg, authenticator, nil, protectedTestModule{}); err == nil { t.Fatal("missing authorizer accepted") }
	if _, err := NewRouter(&cfg, authenticator, nil, authorizer, &permissionTestModule{}); err == nil { t.Fatal("duplicate authorizer accepted") }
	if _, err := NewRouter(&cfg, authenticator, nil, &permissionTestModule{permissionSuffix: "invalid"}); err == nil { t.Fatal("invalid permission endpoint accepted") }
}

{{ end -}}
