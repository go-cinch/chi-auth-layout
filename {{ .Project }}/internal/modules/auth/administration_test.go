package auth

import (
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestPermissionAdministrationUsesConfiguredResources(t *testing.T) {
	for _, test := range []struct {
		name, word, config, method, path string
		all, allowed                     bool
	}{
		{"word alone grants nothing", "admin:grant:system:permission-admin", `{"bindings":[],"ui":[],"endpoints":[]}`, "PATCH", "/user/3", false, false},
		{"explicit user update", "admin:read:arbitrary:name", `{"bindings":[{"resource_key":"admin:system:user","operation":"update"}],"ui":[],"endpoints":[]}`, "PATCH", "/user/3", false, true},
		{"update does not grant create", "admin:read:arbitrary:name", `{"bindings":[{"resource_key":"admin:system:user","operation":"update"}],"ui":[],"endpoints":[]}`, "POST", "/user", false, false},
		{"wildcard has no named identity", "admin:read:arbitrary:name", `{"bindings":[{"resource_key":"admin:system:scope","operation":"*"}],"ui":[],"endpoints":[]}`, "DELETE", "/anything/3", false, true},
		{"wildcard authorizes administration", "admin:read:arbitrary:name", `{"bindings":[{"resource_key":"admin:system:scope","operation":"*"}],"ui":[],"endpoints":[]}`, "", "", true, true},
		{"ordinary endpoint is not wildcard", "admin:all:system:all", `{"bindings":[{"resource_key":"admin:system:user","operation":"update"}],"ui":[],"endpoints":[]}`, "", "", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			ctx := authenticatedContext(t, manager, m.sessions)
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow(test.word, test.config))
			var err error
			if test.all {
				err = m.RequireAllPermissions(ctx)
			} else {
				err = m.RequirePermissionEndpoint(ctx, test.method, test.path)
			}
			if (err == nil) != test.allowed {
				t.Fatalf("allowed=%v err=%v", test.allowed, err)
			}
		})
	}
	m, _, _ := newAuthTestModule(t)
	if !errors.Is(m.RequireAllPermissions(t.Context()), authn.ErrPermissionAdministrationRequired) {
		t.Fatal("missing identity accepted")
	}
}

func TestPermissionAdministrationFailures(t *testing.T) {
	for _, lock := range []bool{true, false} {
		m, manager, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, manager, m.sessions)
		if lock {
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnError(sqlmock.ErrCancelled)
		} else {
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WITH assigned AS").WillReturnError(sqlmock.ErrCancelled)
		}
		if err := m.RequireAllPermissions(ctx); err == nil {
			t.Fatal("failed policy check was ignored")
		}
	}
}

func TestPermissionManagementOperationsFollowSelectedBindings(t *testing.T) {
	for _, test := range []struct {
		name, config, target, operation, featureMethod, featurePath string
		allowed                                                     bool
	}{
		{name: "selected update", config: userEditPermissionConfig, target: "user", operation: "update", allowed: true},
		{name: "legacy single binding", config: `{"resource_key":"admin:system:user","operation":"update","feature_keys":["admin:system:user:edit"],"ui":[],"endpoints":[]}`, target: "user", operation: "update", allowed: true},
		{name: "foreign resource", config: userEditPermissionConfig, target: "role", operation: "update"},
		{name: "foreign operation", config: userEditPermissionConfig, target: "user", operation: "create"},
		{name: "empty features", config: `{"bindings":[{"resource_key":"admin:system:user","operation":"update","feature_keys":[]}],"ui":[],"endpoints":[]}`, target: "user", operation: "update"},
		{name: "menu only", config: `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":[]}],"ui":[],"endpoints":[]}`, target: "user", operation: "read"},
		{name: "wrong owning operation", config: userReadPermissionConfig, target: "user", operation: "update", featureMethod: "PATCH", featurePath: "/user/{id}"},
		{name: "forged cached endpoint", config: `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":[]}],"ui":[],"endpoints":[{"protocol":"http","method":"PATCH","path":"/user/{id}"}]}`, target: "user", operation: "update"},
		{name: "parameter name normalization", config: userEditPermissionConfig, target: "user", operation: "update", featureMethod: "PATCH", featurePath: "/user/{user_id}", allowed: true},
		{name: "static subroute is not collection", config: userReadPermissionConfig, target: "user", operation: "read", featureMethod: "GET", featurePath: "/user/summary"},
		{name: "scoped wildcard", config: `{"bindings":[{"resource_key":"admin:system:scope","operation":"*","feature_keys":[]}],"ui":[],"endpoints":[]}`, target: "role", operation: "delete", allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			if test.featureMethod != "" {
				catalog := m.catalogSource.Current()
				key := "admin:system:user:edit"
				if test.config == userReadPermissionConfig {
					key = "admin:system:user:query"
				}
				feature := catalog.ResourceCatalog.Buttons[key]
				feature.HTTP = []localization.HTTPPath{ {Method: test.featureMethod, Path: test.featurePath}}
				feature.PermissionResource = "role" // Legacy marker cannot grant a different resource.
				catalog.ResourceCatalog.Buttons[key] = feature
				raw, err := json.Marshal(catalog)
				if err != nil {
					t.Fatal(err)
				}
				localizer := localization.New()
				if err := localizer.Install(raw); err != nil {
					t.Fatal(err)
				}
				m.catalogSource = localizer
			}
			ctx := authenticatedContext(t, manager, m.sessions)
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:any:identity", test.config))
			err := m.RequirePermissionOperation(ctx, test.target, test.operation)
			if (err == nil) != test.allowed {
				t.Fatal(test.name, err)
			}
			if !test.allowed && !errors.Is(err, authn.ErrPermissionGrantRequired) {
				t.Fatal(err)
			}
		})
	}
}

func TestPermissionOperationRejectsUnknownSubjectsAndFailures(t *testing.T) {
	m, _, _ := newAuthTestModule(t)
	for _, pair := range [][2]string{ {"unknown", "read"}, {"user", "unknown"}} {
		if err := m.RequirePermissionOperation(t.Context(), pair[0], pair[1]); !errors.Is(err, authn.ErrPermissionGrantRequired) {
			t.Fatal(err)
		}
	}
	if err := m.RequirePermissionOperation(t.Context(), "user", "read"); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatal(err)
	}
	for _, lockFails := range []bool{true, false} {
		m, manager, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, manager, m.sessions)
		lock := mock.ExpectExec("SELECT pg_advisory_xact_lock")
		if lockFails {
			lock.WillReturnError(sqlmock.ErrCancelled)
		} else {
			lock.WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WITH assigned AS").WillReturnError(sqlmock.ErrCancelled)
		}
		if err := m.RequirePermissionOperation(ctx, "user", "read"); !errors.Is(err, sqlmock.ErrCancelled) {
			t.Fatal(err)
		}
	}
}
