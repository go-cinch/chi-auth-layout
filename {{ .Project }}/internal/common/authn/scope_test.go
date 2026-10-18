package authn

import (
	"net/http/httptest"
	"testing"
)

func TestPermissionRequestScope(t *testing.T) {
	for _, test := range []struct {
		name, path string
		headers    []string
		want       string
		invalid    bool
	}{
		{"legacy admin", "/user", nil, "admin", false},
		{"action filter keeps caller scope", "/action?scope=mini", []string{"admin"}, "admin", false},
		{"diagnostic target keeps caller scope", "/action/effective?scope=mini", []string{"admin"}, "admin", false},
		{"message audience is independent", "/msg/sent?scope=targeted", []string{"admin"}, "admin", false},
		{"info uses header only", "/info", []string{"mini"}, "mini", false},
		{"explicit mini", "/user", []string{"mini"}, "mini", false},
		{"query cannot select scope", "/info?scope=mini", nil, "admin", false},
		{"matching info", "/info?scope=mini", []string{"mini"}, "mini", false},
		{"header wins over legacy query", "/info?scope=mini", []string{"admin"}, "admin", false},
		{"empty header", "/user", []string{""}, "", true},
		{"blank header", "/user", []string{" "}, "", true},
		{"duplicate header", "/user", []string{"admin", "admin"}, "", true},
		{"comma header", "/user", []string{"admin,mini"}, "", true},
		{"invalid scope", "/user", []string{"Admin"}, "", true},
		{"duplicate query does not select scope", "/info?scope=admin&scope=mini", nil, "admin", false},
		{"query parsing belongs to HTTP boundary", "/info?scope=%zz", nil, "admin", false},
		{"empty legacy query is ignored", "/info?scope=", nil, "admin", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.path, nil)
			for _, header := range test.headers {
				request.Header.Add(PermissionScopeHeader, header)
			}
			scoped, err := ScopeRequest(request)
			if test.invalid {
				if err == nil {
					t.Fatal("accepted invalid scope")
				}
				return
			}
			if err != nil || PermissionScope(scoped.Context()) != test.want {
				t.Fatalf("scope=%v err=%v", scoped, err)
			}
		})
	}
}
