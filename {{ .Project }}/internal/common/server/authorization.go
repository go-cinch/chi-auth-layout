package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/modules"
)

func newHTTPPermissionAuthorizer(cfg *config.Config, mounted []modules.Module) (modules.HTTPPermissionAuthorizer, string, error) {
	if !cfg.Auth.Authorization.Enabled {
		return nil, "", nil
	}
	var found modules.HTTPPermissionAuthorizer
	for _, module := range mounted {
		candidate, ok := module.(modules.HTTPPermissionAuthorizer)
		if !ok {
			continue
		}
		if found != nil {
			return nil, "", errors.New("http permission authorizer is duplicated")
		}
		found = candidate
	}
	if found == nil {
		return nil, "", errors.New("http permission authorizer is required")
	}
	endpoint := strings.TrimSpace(found.PermissionEndpoint())
	if endpoint == "" || !strings.HasPrefix(endpoint, "/") {
		return nil, "", errors.New("http permission endpoint must start with /")
	}
	return found, strings.TrimSuffix(found.Name(), "/") + endpoint, nil
}

// authorizeHTTPRequest skips only the gateway's own permission endpoint.
func authorizeHTTPRequest(w http.ResponseWriter, r *http.Request, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint string) bool {
	if authorizer == nil || r.URL.Path == permissionEndpoint {
		return true
	}
	allowed, err := authorizer.AuthorizeHTTP(r.Context(), r.Method, r.URL.Path)
	if err != nil {
		slog.ErrorContext(r.Context(), "authorize http request failed: "+err.Error(),
			"operation", "authorize_http_request", "outcome", "failure", "method", r.Method, "error", err)
		WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return false
	}
	if !allowed {
		WriteError(w, r, http.StatusForbidden, apperror.Forbidden)
		return false
	}
	return true
}
