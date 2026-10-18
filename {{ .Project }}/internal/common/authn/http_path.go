package authn

import (
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

// AuthHTTPPaths maps persisted auth endpoint identities to their deployed paths.
// The stored /auth namespace remains stable when an application's mount changes.
func AuthHTTPPaths(prefix string) map[string]string {
	paths := make(map[string]string)
	for _, relative := range []string{
		"/pub/register/username", "/pub/challenge", "/pub/register",
		"/pub/slider/challenge", "/pub/slider/verify", "/pub/login/verification",
		"/pub/captcha", "/pub/captcha/verify", "/pub/login", "/pub/refresh", "/pub/logout",
		"/permission", "/logout", "/challenge", "/captcha", "/captcha/verify",
		"/change/pwd", "/reset/pwd", "/info",
	} {
		paths["/auth"+relative] = prefix + relative
	}
	return paths
}

func (m *Manager) SetHTTPPath(prefix string) error {
	value, err := config.NormalizeHTTPPath(prefix)
	if err != nil {
		return err
	}
	m.httpPath = value
	return nil
}

// RelativeAuthHTTPPath recognizes only endpoints owned by the auth module.
func RelativeAuthHTTPPath(path, prefix string) (string, bool) {
	for canonical, actual := range AuthHTTPPaths(prefix) {
		if path == actual {
			return strings.TrimPrefix(canonical, "/auth"), true
		}
	}
	return "", false
}
