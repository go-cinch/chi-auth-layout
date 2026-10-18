package authn_test

import (
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestAuthHTTPPathsAndResetAllowlist(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/identity/v2"} {
		paths := authn.AuthHTTPPaths(prefix)
		if paths["/auth/info"] != prefix+"/info" {
			t.Fatal(paths)
		}
		for _, relative := range []string{"/reset/pwd", "/challenge", "/logout", "/pub/logout", "/pub/refresh"} {
			method := "POST"
			if relative == "/reset/pwd" {
				method = "PATCH"
			}
			if !authn.PasswordResetAllowedHTTP(method, prefix+relative, prefix) {
				t.Fatal(prefix, relative)
			}
			if authn.PasswordResetAllowedHTTP("GET", prefix+relative, prefix) {
				t.Fatal("method escalation")
			}
		}
		if authn.PasswordResetAllowedHTTP("PATCH", prefix+"/change/pwd", prefix) {
			t.Fatal("ordinary change allowed")
		}
		if path, ok := authn.RelativeAuthHTTPPath(prefix+"/info", prefix); !ok || path != "/info" {
			t.Fatal(path, ok)
		}
		if _, ok := authn.RelativeAuthHTTPPath("/user", prefix); ok {
			t.Fatal("foreign module matched")
		}
	}
	manager, err := authn.New("test", testKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetHTTPPath("/auth"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetHTTPPath("relative"); err == nil {
		t.Fatal("accepted invalid path")
	}
}
