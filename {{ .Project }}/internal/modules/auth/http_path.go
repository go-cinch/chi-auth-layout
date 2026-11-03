package auth

import (
	"net/http"
	"strings"
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
		"/mfa", "/mfa/challenge", "/mfa/setup", "/mfa/confirm", "/mfa/test",
	} {
		paths["/auth"+relative] = prefix + relative
	}
	paths["/auth/mfa/reset/{id}"] = prefix + "/mfa/reset/{{ if eq .Computed.http_router_final "gin" }}:id{{ else }}{id}{{ end }}"
	return paths
}

// RelativeAuthHTTPPath recognizes only endpoints owned by the auth module.
func RelativeAuthHTTPPath(path, prefix string) (string, bool) {
	for canonical, actual := range AuthHTTPPaths(prefix) {
		if path == actual || ((strings.Contains(actual, "{id}") || strings.Contains(actual, ":id")) && matchRouteTemplate(actual, path)) {
			if strings.Contains(actual, "{id}") || strings.Contains(actual, ":id") {
				return strings.TrimPrefix(path, prefix), true
			}
			return strings.TrimPrefix(canonical, "/auth"), true
		}
	}
	return "", false
}

// authHTTPMethod identifies the method owned by each canonical Auth path.
// Parameterized paths are recognized with the same matcher as route permissions.
func authHTTPMethod(relative string) string {
	switch relative {
	case "/pub/register/username", "/pub/login/verification", "/permission", "/mfa", "/info":
		return http.MethodGet
	case "/change/pwd", "/reset/pwd":
		return http.MethodPatch
	case "/pub/challenge", "/pub/register", "/pub/slider/challenge", "/pub/slider/verify",
		"/pub/captcha", "/pub/captcha/verify", "/pub/login", "/pub/refresh", "/pub/logout",
		"/logout", "/challenge", "/captcha", "/captcha/verify",
		"/mfa/challenge", "/mfa/setup", "/mfa/confirm", "/mfa/test":
		return http.MethodPost
	}
	if matchRouteTemplate("/mfa/reset/{id}", relative) {
		return http.MethodPost
	}
	return ""
}

// PasswordResetAllowedHTTP is owned by the auth capability, independently of
// permission definitions. Gateway checks are added only to the login boundary.
func PasswordResetAllowedHTTP(method, path string, prefixes ...string) bool {
	prefix := ""
	if len(prefixes) > 0 {
		prefix = prefixes[0]
	}
	return (method == http.MethodGet && path == prefix+"/mfa") ||
		(method == http.MethodPost && (path == prefix+"/mfa/challenge" || path == prefix+"/mfa/setup" || path == prefix+"/mfa/confirm")) ||
		(method == http.MethodPatch && path == prefix+"/reset/pwd") ||
		(method == http.MethodPost && (path == prefix+"/challenge" || path == prefix+"/logout" || path == prefix+"/pub/logout" || path == prefix+"/pub/refresh"))
}

func (m *Module) AllowRestrictedHTTP(method, path string) bool {
	return PasswordResetAllowedHTTP(method, path, m.httpPath) || (method == http.MethodGet && path == m.httpPath+m.PermissionEndpoint())
}
