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
		"/pub/i18n", "/pub/status", "/status", "/pub/opt", "/pub/timezones", "/pub/register/username", "/pub/challenge", "/pub/register",
		"/pub/slider/challenge", "/pub/slider/verify", "/pub/login/verification",
		"/pub/captcha", "/pub/captcha/verify", "/pub/login", "/pub/refresh", "/pub/logout",
		"/permission", "/logout", "/challenge", "/captcha", "/captcha/verify",
		"/change/pwd", "/reset/pwd", "/info",
		"/totp", "/totp/challenge", "/totp/setup", "/totp/confirm", "/totp/test", "/totp/recovery/email-code", "/totp/recovery/confirm", "/email/code", "/email/sensitive-code", "/email/code/status", "/email/bind", "/email/unbind/status", "/email/unbind/request", "/email/unbind", "/pub/totp/reset/email-code", "/pub/totp/reset/confirm", "/pub/email/unbind", "/pub/login/email-code", "/pub/login/email-code/status",
	} {
		paths["/auth"+relative] = prefix + relative
	}
	paths["/auth/totp/reset/{id}"] = prefix + "/totp/reset/{{ if eq .Computed.http_router_final "gin" }}:id{{ else }}{id}{{ end }}"
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

// ownsAuthHTTPMethod checks the allowed methods for each canonical Auth path.
// Parameterized paths are recognized with the same matcher as route permissions.
func ownsAuthHTTPMethod(relative, method string) bool {
	switch relative {
	case "/info":
		return method == http.MethodGet || method == http.MethodPatch
	case "/pub/i18n", "/status", "/pub/status", "/email/unbind/status", "/pub/opt", "/pub/timezones", "/pub/register/username", "/pub/login/verification", "/permission", "/totp", "/email/code/status", "/pub/login/email-code/status":
		return method == http.MethodGet
	case "/change/pwd", "/reset/pwd":
		return method == http.MethodPatch
	case "/email/unbind/request", "/email/unbind", "/pub/totp/reset/email-code", "/pub/totp/reset/confirm", "/pub/email/unbind", "/pub/challenge", "/pub/register", "/pub/slider/challenge", "/pub/slider/verify",
		"/pub/captcha", "/pub/captcha/verify", "/pub/login", "/pub/refresh", "/pub/logout",
		"/logout", "/challenge", "/captcha", "/captcha/verify",
		"/totp/challenge", "/totp/setup", "/totp/confirm", "/totp/test", "/totp/recovery/email-code", "/totp/recovery/confirm", "/email/code", "/email/sensitive-code", "/email/bind", "/pub/login/email-code":
		return method == http.MethodPost
	}
	if matchRouteTemplate("/totp/reset/{id}", relative) {
		return method == http.MethodPost
	}
	return false
}

// PasswordResetAllowedHTTP is owned by the auth capability, independently of
// permission definitions. Gateway checks are added only to the login boundary.
func PasswordResetAllowedHTTP(method, path string, prefixes ...string) bool {
	prefix := ""
	if len(prefixes) > 0 {
		prefix = prefixes[0]
	}
	return (method == http.MethodGet && (path == prefix+"/totp" || path == prefix+"/status")) ||
		(method == http.MethodPost && (path == prefix+"/totp/challenge" || path == prefix+"/totp/setup" || path == prefix+"/totp/confirm" || path == prefix+"/email/sensitive-code")) ||
		(method == http.MethodPatch && (path == prefix+"/reset/pwd" || path == prefix+"/info")) ||
		(method == http.MethodPost && (path == prefix+"/challenge" || path == prefix+"/logout" || path == prefix+"/pub/logout" || path == prefix+"/pub/refresh"))
}

func (m *Module) AllowRestrictedHTTP(method, path string) bool {
	return PasswordResetAllowedHTTP(method, path, m.httpPath) || (method == http.MethodGet && path == m.httpPath+m.PermissionEndpoint())
}
