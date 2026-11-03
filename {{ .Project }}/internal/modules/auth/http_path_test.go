package auth

import (
	"testing"
)

func TestAuthHTTPPathsAndResetAllowlist(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/identity/v2"} {
		paths := AuthHTTPPaths(prefix)
		if paths["/auth/info"] != prefix+"/info" {
			t.Fatal(paths)
		}
		if paths["/auth/mfa/test"] != prefix+"/mfa/test" {
			t.Fatal("test endpoint mount mismatch", prefix)
		}
		if PasswordResetAllowedHTTP("POST", prefix+"/mfa/test", prefix) {
			t.Fatal("test endpoint bypassed required password reset", prefix)
		}
		for _, relative := range []string{"/reset/pwd", "/mfa/challenge", "/mfa/setup", "/mfa/confirm", "/challenge", "/logout", "/pub/logout", "/pub/refresh"} {
			method := "POST"
			if relative == "/reset/pwd" {
				method = "PATCH"
			}
			if !PasswordResetAllowedHTTP(method, prefix+relative, prefix) {
				t.Fatal(prefix, relative)
			}
			if PasswordResetAllowedHTTP("GET", prefix+relative, prefix) {
				t.Fatal("method escalation")
			}
		}
		if PasswordResetAllowedHTTP("PATCH", prefix+"/change/pwd", prefix) {
			t.Fatal("ordinary change allowed")
		}
		if path, ok := RelativeAuthHTTPPath(prefix+"/info", prefix); !ok || path != "/info" {
			t.Fatal(path, ok)
		}
		if _, ok := RelativeAuthHTTPPath("/user", prefix); ok {
			t.Fatal("foreign module matched")
		}
	}
}

func TestAuthHTTPMethodRejectsUnknownPaths(t *testing.T) {
	for _, path := range []string{"/other", "/mfa/reset", "/mfa/reset/1/other", "/mfa/reset/..", "/mfa/setup/other"} {
		if method := authHTTPMethod(path); method != "" {
			t.Fatalf("unknown route %s assigned method %s", path, method)
		}
	}
}
