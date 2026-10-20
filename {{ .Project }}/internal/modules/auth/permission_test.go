package auth

import (
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"
	"{{ .Computed.module_name_final }}/internal/modules/action"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ else }}
	"github.com/go-chi/chi/v5"
{{ end }}
)

func permissionRows() *sqlmock.Rows { return sqlmock.NewRows([]string{"word", "config"}) }

func installAuthCatalog(t *testing.T, localizer *localization.Module) {
	t.Helper()
	c := localization.Catalog{SchemaVersion: 2, DefaultLocale: "en-US", Scopes: []string{"admin"}, Operations: []string{"read", "create", "update", "delete"}, Locales: []localization.Language{ {Tag: "en-US", Name: "English", Enabled: true}}, Messages: map[string]localization.Messages{"en-US": {}}, ResourceCatalog: localization.ResourceCatalog{Resources: map[string]localization.Resource{}, Buttons: map[string]localization.Feature{}}}
	for _, key := range []string{"admin:system:user", "admin:system:info", "admin:account:profile"} {
		c.ResourceCatalog.Resources[key] = localization.Resource{Kind: "resource", Operations: map[string]localization.ResourceOperation{
			"read": {EntryKeys: []string{}, ButtonKeys: []string{key + ":transport"}}, "update": {EntryKeys: []string{}, ButtonKeys: []string{key + ":modify"}}}}
		c.ResourceCatalog.Buttons[key+":transport"] = localization.Feature{Btn: []string{}, HTTP: []localization.HTTPPath{}, GRPC: []string{}}
		c.ResourceCatalog.Buttons[key+":modify"] = localization.Feature{Btn: []string{}, HTTP: []localization.HTTPPath{ {Method: "POST", Path: "/challenge"}, {Method: "PATCH", Path: "/reset/pwd"}}, GRPC: []string{}}
	}
	r := c.ResourceCatalog.Resources["admin:system:user"]
	r.Entries = map[string]localization.ResourceEntry{"admin:menu:system:user": {Path: "/system/user", ParentMenuKey: "admin:menu:system"}}
	r.Operations["read"] = localization.ResourceOperation{EntryKeys: []string{"admin:menu:system:user"}, ButtonKeys: []string{"admin:system:user:query", "admin:system:user:transport"}}
	r.Operations["update"] = localization.ResourceOperation{EntryKeys: []string{}, ButtonKeys: []string{"admin:system:user:edit"}}
	c.ResourceCatalog.Resources["admin:system:user"] = r
	c.ResourceCatalog.Buttons["admin:system:user:query"] = localization.Feature{Btn: []string{"admin:btn:system:user:query"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user"}}, GRPC: []string{}}
	c.ResourceCatalog.Buttons["admin:system:user:edit"] = localization.Feature{Btn: []string{"admin:btn:system:user:edit"}, HTTP: []localization.HTTPPath{ {Method: "PATCH", Path: "/user/{id}"}}, GRPC: []string{}}
	delete(c.ResourceCatalog.Buttons, "admin:system:user:modify")
	c.ResourceCatalog.Resources["admin:system:scope"] = localization.Resource{Kind: "resource", PermissionScope: "*"}
	current := localizer.Current()
	c.Messages = current.Messages
	c.Locales = current.Locales
	c.DefaultLocale = current.DefaultLocale
	raw, _ := json.Marshal(c)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(localizer.Reset)
}

func permissionConfig(localizer *localization.Module, method, path string) string {
	c := localizer.Current()
	for _, key := range []string{"admin:system:user:transport", "admin:system:info:transport", "admin:account:profile:transport", "admin:account:profile:modify"} {
		f := c.ResourceCatalog.Buttons[key]
		f.HTTP = []localization.HTTPPath{ {Method: method, Path: path}}
		c.ResourceCatalog.Buttons[key] = f
	}
	f := c.ResourceCatalog.Buttons["admin:system:user:query"]
	f.HTTP = []localization.HTTPPath{}
	c.ResourceCatalog.Buttons["admin:system:user:query"] = f
	raw, _ := json.Marshal(c)
	if err := localizer.Install(raw); err != nil {
		panic(err)
	}
	return `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":["admin:system:user:transport"]}],"ui":[],"endpoints":[]}`
}

const userReadPermissionConfig = `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":["admin:system:user:query"]}],"ui":[],"endpoints":[]}`
const userEditPermissionConfig = `{"bindings":[{"resource_key":"admin:system:user","operation":"update","feature_keys":["admin:system:user:edit"]}],"ui":[],"endpoints":[]}`

func TestPermissions(t *testing.T) {
	t.Run("aggregate and keep transport private", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig).AddRow("admin:update:system:user", userEditPermissionConfig))
		value, err := m.permissions(t.Context(), 3, "admin")
		if err != nil {
			t.Fatal(err)
		}
		if len(value.Words) != 2 || len(value.Menus) != 1 || len(value.Menus[0].Buttons) != 2 {
			t.Fatalf("permission=%+v", value)
		}
		encoded, _ := json.Marshal(value)
		if strings.Contains(string(encoded), "endpoints") || strings.Contains(string(encoded), "resources") || strings.Contains(string(encoded), "GET|") {
			t.Fatalf("transport leaked: %s", encoded)
		}
	})
	t.Run("edit alone cannot enter page", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:update:system:user", userEditPermissionConfig))
		value, err := m.permissions(t.Context(), 3, "admin")
		if err != nil || len(value.Menus) != 0 || len(value.Words) != 1 {
			t.Fatalf("permission=%+v err=%v", value, err)
		}
	})
	t.Run("empty arrays", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows())
		value, err := m.permissions(t.Context(), 3)
		if err != nil || value.Words == nil || value.Menus == nil {
			t.Fatalf("permission=%+v err=%v", value, err)
		}
	})
	for _, kind := range []string{"query", "scan", "row", "invalid-config"} {
		t.Run(kind, func(t *testing.T) {
			m, _, mock := newAuthTestModule(t)
			expected := mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin")
			switch kind {
			case "query":
				expected.WillReturnError(sqlmock.ErrCancelled)
			case "scan":
				expected.WillReturnRows(permissionRows().AddRow(nil, "{}"))
			case "row":
				expected.WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig).RowError(0, sqlmock.ErrCancelled))
			case "invalid-config":
				expected.WillReturnRows(permissionRows().AddRow("admin:read:system:user", `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":["admin:system:user:missing"]}],"ui":[],"endpoints":[]}`))
			}
			if _, err := m.permissions(t.Context(), 3); err == nil {
				t.Fatal("failure ignored")
			}
		})
	}
}

func TestMatchEndpoint(t *testing.T) {
	for _, test := range []struct {
		template, path string
		want           bool
	}{
		{"/user/{id}", "/user/42", true}, {"/user/:id", "/user/42", true}, {"/user/{id}", "/user/42/member", false}, {"/msg/inbox/{id}", "/msg/sent", false}, {"/user/{id}", "/user/", false}, {"/user/{id}", "/user/..", false}, {"/user/{id}", "/user/%2fmsg", false}, {"/user", "/user", true}, {"/user", "/users", false}, {"/user/*", "/user/1", false},
	} {
		if got := matchRouteTemplate(test.template, test.path); got != test.want {
			t.Fatalf("%s %s=%v", test.template, test.path, got)
		}
	}
	endpoint := action.Endpoint{Protocol: "http", Method: "GET", Path: "/user/{id}"}
	if matchEndpoint(endpoint, PermissionTarget{Method: "DELETE", Path: "/user/1"}) {
		t.Fatal("method escalation")
	}
{{ if .Computed.enable_grpc_final }}
	endpoint = action.Endpoint{Protocol: "grpc", Method: "GRPC", Path: "/auth.v1.User/GetUser"}
	if !matchEndpoint(endpoint, normalizePermissionTarget(PermissionTarget{Resource: "auth.v1.User/GetUser"})) || matchEndpoint(endpoint, PermissionTarget{Resource: "/auth.v1.User/DeleteUser"}) {
		t.Fatal("RPC boundary")
	}
{{ end }}
}

func TestCheckPermission(t *testing.T) {
	localizer := localization.New()
	t.Run("configured permissions and current assignments", func(t *testing.T) {
		m, authenticator, mock := newAuthTestModule(t)
		localizer = m.catalogSource.(*localization.Module)
		ctx := authenticatedContext(t, authenticator, m.sessions)
		for _, target := range []PermissionTarget{
			{Method: "POST", Path: "/logout"}, {Method: "POST", Path: "/challenge"}, {Method: "PATCH", Path: "/reset/pwd"}, {Method: "GET", Path: "/user"}} {
			for _, granted := range []bool{true, false} {
				rows := permissionRows()
				if granted {
					rows.AddRow("admin:read:system:user", permissionConfig(localizer, target.Method, target.Path))
				}
				mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(rows)
				_, allowed, err := m.CheckPermission(ctx, target)
				if err != nil || allowed != granted {
					t.Fatalf("target=%+v granted=%v allowed=%v err=%v", target, granted, allowed, err)
				}
			}
		}
	})
	t.Run("read does not grant write", func(t *testing.T) {
		m, a, mock := newAuthTestModule(t)
		localizer = m.catalogSource.(*localization.Module)
		ctx := authenticatedContext(t, a, m.sessions)
		for _, method := range []string{"POST", "PATCH", "DELETE"} {
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig))
			if _, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: method, Path: "/user"}); err != nil || allowed {
				t.Fatalf("%s allowed=%v err=%v", method, allowed, err)
			}
		}
	})
	t.Run("invalid target", func(t *testing.T) {
		m, a, _ := newAuthTestModule(t)
		localizer = m.catalogSource.(*localization.Module)
		ctx := authenticatedContext(t, a, m.sessions)
		if _, allowed, err := m.CheckPermission(ctx, PermissionTarget{}); err != nil || allowed {
			t.Fatalf("allowed=%v err=%v", allowed, err)
		}
	})
	t.Run("missing identity", func(t *testing.T) {
		m, _, _ := newAuthTestModule(t)
		localizer = m.catalogSource.(*localization.Module)
		if _, _, err := m.CheckPermission(t.Context(), PermissionTarget{Method: "GET", Path: "/user"}); !errors.Is(err, ErrUnauthorized) {
			t.Fatal(err)
		}
	})
}

func TestRegisteredStaticRouteCannotBeGrantedByParameter(t *testing.T) {
	localizer := localization.New()
	m, manager, mock := newAuthTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	ctx := authenticatedContext(t, manager, m.sessions)
	m.SetHTTPPermissionRoutes([]modules.HTTPPermissionRoute{
		{Method: "GET", Path: "/user/{id}"}, {Method: "GET", Path: "/user/summary"}, {Method: "GET", Path: "/user/detail/{id}"}})
	for _, test := range []struct {
		path    string
		allowed bool
	}{
		{"/user/42", true}, {"/user/summary", false}, {"/user/detail/42", false}} {
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", permissionConfig(localizer, "GET", "/user/{id}")))
		_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: "GET", Path: test.path})
		if err != nil || allowed != test.allowed {
			t.Fatalf("path=%s allowed=%v err=%v", test.path, allowed, err)
		}
	}
}

type permissionRouteTestModule struct{}

func (permissionRouteTestModule) Name() string { return "/user" }

{{ if eq .Computed.http_router_final "gin" -}}
func (permissionRouteTestModule) HTTP(r *gin.RouterGroup) {
	r.GET("/:id", func(c *gin.Context) { server.WriteOK(c.Writer) })
	r.GET("/summary", func(c *gin.Context) { server.WriteOK(c.Writer) })
}
{{ else -}}
func (permissionRouteTestModule) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/{id}", func(w http.ResponseWriter, _ *http.Request) { server.WriteOK(w) })
	r.Get("/summary", func(w http.ResponseWriter, _ *http.Request) { server.WriteOK(w) })
	return r
}
{{ end }}
func TestGatewayRoutePrecedenceWithoutLocalAuthorization(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "local_authorization_disabled"
		if enabled {
			name = "local_authorization_enabled"
		}
		t.Run(name, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			cfg := &config.Config{}
			cfg.Auth.Authorization.Enabled = enabled
			if _, err := server.NewRouter(cfg, manager, nil, m, permissionRouteTestModule{}); err != nil {
				t.Fatal(err)
			}
			ctx := authenticatedContext(t, manager, m.sessions)
			localizer := m.catalogSource.(*localization.Module)
			permission := permissionConfig(localizer, http.MethodGet, "/user/{id}")
			for _, target := range []struct {
				path    string
				allowed bool
			}{ {"/user/42", true}, {"/user/summary", false}} {
				mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").
					WillReturnRows(permissionRows().AddRow("admin:read:system:user", permission))
				_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: http.MethodGet, Path: target.path})
				if err != nil || allowed != target.allowed {
					t.Fatalf("gateway target=%s allowed=%v want=%v err=%v", target.path, allowed, target.allowed, err)
				}
			}
		})
	}
}

func TestPermissionPathsAndLiveCatalogReplacement(t *testing.T) {
	localizer := localization.New()
	m, manager, mock := newAuthTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	ctx := authenticatedContext(t, manager, m.sessions)
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig))
	value, err := m.PermissionPaths(ctx, "admin", report.Fields{"btn", "http", "grpc"})
	if err != nil || len(value.Btn) != 1 || len(value.HTTP) != 1 || value.HTTP[0].Path != "/user" || value.GRPC == nil {
		t.Fatalf("%+v %v", value, err)
	}
	catalog := localizer.Current()
	feature := catalog.ResourceCatalog.Buttons["admin:system:user:query"]
	feature.HTTP = []localization.HTTPPath{ {Method: "GET", Path: "/user/new"}}
	catalog.ResourceCatalog.Buttons["admin:system:user:query"] = feature
	raw, _ := json.Marshal(catalog)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path    string
		allowed bool
	}{ {"/user", false}, {"/user/new", true}} {
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig))
		_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: "GET", Path: test.path})
		if err != nil || allowed != test.allowed {
			t.Fatalf("%s %v %v", test.path, allowed, err)
		}
	}
}

type selectedPermissionCatalog struct{ catalog localization.Catalog }

func (s selectedPermissionCatalog) Current() localization.Catalog {
	panic("full language catalog requested")
}

func (s selectedPermissionCatalog) Select(fields []string) localization.Catalog {
	if len(fields) != 1 || fields[0] != "resource_catalog" {
		panic("unrelated catalog fields requested")
	}
	return s.catalog
}

func TestHTTPPermissionSelectionSkipsUIAndOtherProtocols(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	catalog := localization.Catalog{ResourceCatalog: localization.ResourceCatalog{
		Resources: map[string]localization.Resource{"admin:system:user": {Kind: "resource", Entries: map[string]localization.ResourceEntry{"invalid": {Path: "invalid"}}, Operations: map[string]localization.ResourceOperation{"read": {EntryKeys: []string{"invalid"}, ButtonKeys: []string{"admin:system:user:query"}}}}},
		Buttons:   map[string]localization.Feature{"admin:system:user:query": {Btn: []string{"invalid"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user"}}, GRPC: []string{"invalid"}}},
	}}
	m.catalogSource = selectedPermissionCatalog{catalog}
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:default:system:default", `{"builtin":"default","resource_keys":["admin:system:user"],"feature_keys":null,"ui":[],"endpoints":[]}`))
	permission, err := m.permissionsFor(t.Context(), 3, "admin", report.Fields{"http"})
	if err != nil {
		t.Fatal(err)
	}
	if len(permission.endpoints) != 1 || permission.endpoints[0].Path != "/user" || len(permission.Menus) != 0 || len(permission.Buttons) != 0 || len(permission.Words) != 0 {
		t.Fatalf("%+v", permission)
	}
}
