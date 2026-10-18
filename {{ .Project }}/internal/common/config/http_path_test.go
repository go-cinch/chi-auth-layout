package config_test

import (
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestNormalizeHTTPPath(t *testing.T) {
	for _, value := range []string{"", "/", "/auth", "/gateway/auth-v2"} {
		got, err := config.NormalizeHTTPPath(value)
		want := value
		if want == "/" {
			want = ""
		}
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", value, got, err)
		}
	}
	for _, value := range []string{"auth", "/auth/", "//auth", "/a/../b", "/a?b", "/a%2fb", "/a/*", " /auth", "/a b"} {
		if _, err := config.NormalizeHTTPPath(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
