package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)
{{ if eq .Computed.http_router_final "gin" -}}
func authenticationMiddleware(authenticator *authn.Manager, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint, modulePath string, module modules.Module) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, allowed := authenticateHTTPRequest(c.Writer, c.Request, authenticator, authorizer, permissionEndpoint, modulePath, module)
		c.Request = request
		if !allowed {
			c.Abort()
			return
		}
		c.Next()
	}
}
{{ else -}}
func authenticationMiddleware(authenticator *authn.Manager, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint, modulePath string, module modules.Module, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, allowed := authenticateHTTPRequest(w, r, authenticator, authorizer, permissionEndpoint, modulePath, module)
		if !allowed {
			return
		}
		next.ServeHTTP(w, request)
	})
}
{{ end -}}
// authenticateHTTPRequest preserves the same guard order for both routers:
// identity, scope, audit actor, permissions, then the business security policy.
// The caller runs idempotency and business handlers only after these checks.
func authenticateHTTPRequest(w http.ResponseWriter, r *http.Request, authenticator *authn.Manager, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint, modulePath string, module modules.Module) (*http.Request, bool) {
	if anonymous, ok := module.(modules.AnonymousHTTPModule); ok && anonymous.AllowAnonymousHTTP(r) {
		return r, true
	}
	if isPublicEndpoint(r.URL.Path, modulePath) {
		return r, true
	}
	request, err := authenticator.AuthenticateRequest(r)
	if err != nil {
		writeAuthenticationError(w, r, err)
		return r, false
	}
	request, err = authn.ScopeRequest(request)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, err)
		return r, false
	}
	identity, _ := authn.FromContext(request.Context())
	auditctx.Identity(request.Context(), identity.UserID, identity.Username)
	if !authorizeHTTPRequest(w, request, authorizer, permissionEndpoint) {
		return request, false
	}
	if err := authenticator.CheckHTTPSecurity(request); err != nil {
		WriteHTTPSecurityError(w, request, err)
		return request, false
	}
	return request, true
}

func isPublicEndpoint(path, modulePath string) bool {
	publicPath := strings.TrimSuffix(modulePath, "/") + "/pub"
	return path == publicPath || strings.HasPrefix(path, publicPath+"/")
}

func writeAuthenticationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, authn.ErrPasswordResetRequired):
		WriteError(w, r, http.StatusForbidden, err)
	case errors.Is(err, authn.ErrUnauthorized):
		WriteError(w, r, http.StatusUnauthorized, apperror.Unauthorized)
	default:
		slog.ErrorContext(r.Context(), "authenticate http request failed: "+err.Error(),
			"operation", "authenticate_http_request", "outcome", "failure", "method", r.Method, "error", err)
		WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
	}
}
