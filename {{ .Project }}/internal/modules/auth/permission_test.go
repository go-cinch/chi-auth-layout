package auth

import (
	"errors"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPermissions(t *testing.T) {
	t.Run("aggregate", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
			sqlmock.NewRows([]string{"resource", "menu", "btn"}).
				AddRow("GET|/auth/info|/auth.v1.Auth/Info\n\n GET|/user|/auth.v1.User/List ", "/dashboard/overview\n/system/user", "system.user.read").
				AddRow("GET|/user|/auth.v1.User/List\nPOST|/user|/auth.v1.User/Create", "/system/user", "system.user.read\n system.user.create "),
		)
		permission, err := m.permissions(t.Context(), 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"GET|/auth/info|/auth.v1.Auth/Info", "GET|/user|/auth.v1.User/List", "POST|/user|/auth.v1.User/Create"}; !reflect.DeepEqual(permission.Resources, want) {
			t.Fatalf("resources = %#v, want %#v", permission.Resources, want)
		}
		if want := []string{"/dashboard/overview", "/system/user"}; !reflect.DeepEqual(permission.Menus, want) {
			t.Fatalf("menus = %#v, want %#v", permission.Menus, want)
		}
		if want := []string{"system.user.read", "system.user.create"}; !reflect.DeepEqual(permission.Buttons, want) {
			t.Fatalf("buttons = %#v, want %#v", permission.Buttons, want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
			sqlmock.NewRows([]string{"resource", "menu", "btn"}),
		)
		permission, err := m.permissions(t.Context(), 3)
		if err != nil {
			t.Fatal(err)
		}
		if permission.Resources == nil || permission.Menus == nil || permission.Buttons == nil {
			t.Fatalf("empty permissions must be JSON arrays: %#v", permission)
		}
	})

	t.Run("query failure", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnError(sqlmock.ErrCancelled)
		if _, err := m.permissions(t.Context(), 3); err == nil {
			t.Fatal("query failure was ignored")
		}
	})

	t.Run("scan failure", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
			sqlmock.NewRows([]string{"resource", "menu", "btn"}).AddRow(nil, "", ""),
		)
		if _, err := m.permissions(t.Context(), 3); err == nil {
			t.Fatal("scan failure was ignored")
		}
	})

	t.Run("row failure", func(t *testing.T) {
		m, _, mock := newAuthTestModule(t)
		rowErr := errors.New("row failure")
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
			sqlmock.NewRows([]string{"resource", "menu", "btn"}).
				AddRow("GET|/auth/info", "", "").
				AddRow("GET|/user", "", "").
				RowError(1, rowErr),
		)
		if _, err := m.permissions(t.Context(), 3); !errors.Is(err, rowErr) {
			t.Fatalf("row failure = %v", err)
		}
	})
}

func TestMatchPermissionRule(t *testing.T) {
	tests := []struct {
		name   string
		rule   string
		target PermissionTarget
		want   bool
	}{
		{"all", "*", PermissionTarget{Method: "GET", Path: "/anything"}, true},
		{"http exact", "POST|/auth/logout|/auth.v1.Auth/Logout", PermissionTarget{Method: "POST", Path: "/auth/logout"}, true},
		{"http method list", "PUT,PATCH|/user/*|/auth.v1.User/Update", PermissionTarget{Method: "patch", Path: "/user/42"}, true},
		{"http query removed", "GET|/order/*|/order.v1.Order/Get", PermissionTarget{Method: "GET", Path: "/order/42?details=true"}, true},
		{"rpc", "POST|/auth/logout|/auth.v1.Auth/Logout", PermissionTarget{Resource: "auth.v1.Auth/Logout"}, true},
		{"wrong method", "POST|/auth/logout|/auth.v1.Auth/Logout", PermissionTarget{Method: "GET", Path: "/auth/logout"}, false},
		{"wrong path", "GET|/user/*|/auth.v1.User/Get", PermissionTarget{Method: "GET", Path: "/role/1"}, false},
		{"wrong rpc", "GET|/user/*|/auth.v1.User/Get", PermissionTarget{Resource: "/auth.v1.Role/Get"}, false},
		{"http two fields", "GET|/user", PermissionTarget{Method: "GET", Path: "/user"}, true},
		{"http two fields wrong method", "GET|/user", PermissionTarget{Method: "DELETE", Path: "/user"}, false},
		{"http two fields glob", "GET,PATCH|/user/*", PermissionTarget{Method: "PATCH", Path: "/user/7"}, true},
		{"empty rpc rejected", "GET|/user|", PermissionTarget{Method: "GET", Path: "/user"}, false},
		{"blank rpc rejected", "GET|/user|  ", PermissionTarget{Method: "GET", Path: "/user"}, false},
		{"rpc only", "||/auth.v1.Auth/Info", PermissionTarget{Resource: "/auth.v1.Auth/Info"}, true},
		{"empty", "", PermissionTarget{Method: "GET", Path: "/user"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchPermissionRule(test.rule, normalizePermissionTarget(test.target)); got != test.want {
				t.Fatalf("matchPermissionRule(%q, %#v) = %v, want %v", test.rule, test.target, got, test.want)
			}
		})
	}
}

func TestCheckPermission(t *testing.T) {
	t.Run("self service requires configured permissions", func(t *testing.T) {
		m, authenticator, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, authenticator, m.sessions)
		for _, target := range []PermissionTarget{
			{Method: "POST", Path: "/auth/logout"},
			{Method: "POST", Path: "/auth/challenge"},
			{Method: "POST", Path: "/auth/captcha/verify"},
			{Method: "PATCH", Path: "/auth/reset/pwd"},
		} {
			for _, granted := range []bool{false, true} {
				rows := sqlmock.NewRows([]string{"resource", "menu", "btn"})
				if granted {
					rows.AddRow(target.Method+"|"+target.Path, "", "")
				}
				mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(rows)
				_, allowed, err := m.CheckPermission(ctx, target)
				if err != nil || allowed != granted {
					t.Fatalf("%s %s granted=%v: allowed=%v err=%v", target.Method, target.Path, granted, allowed, err)
				}
			}
		}
	})

	t.Run("allowed", func(t *testing.T) {
		m, authenticator, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, authenticator, m.sessions)
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
			sqlmock.NewRows([]string{"resource", "menu", "btn"}).
				AddRow("GET|/order/*|/order.v1.Order/Get", "", ""),
		)
		identity, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: "get", Path: "/order/42"})
		if err != nil || !allowed || identity.Username != "readonly" {
			t.Fatalf("check = %#v, %v, %v", identity, allowed, err)
		}
	})

	t.Run("invalid target fails closed", func(t *testing.T) {
		m, authenticator, _ := newAuthTestModule(t)
		ctx := authenticatedContext(t, authenticator, m.sessions)
		if _, allowed, err := m.CheckPermission(ctx, PermissionTarget{}); err != nil || allowed {
			t.Fatalf("check = %v, %v", allowed, err)
		}
	})

	t.Run("missing identity", func(t *testing.T) {
		m, _, _ := newAuthTestModule(t)
		if _, _, err := m.CheckPermission(t.Context(), PermissionTarget{Method: "GET", Path: "/order/42"}); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v", err)
		}
	})
}
