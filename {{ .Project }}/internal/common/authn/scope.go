package authn

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

const DefaultPermissionScope = "admin"
const PermissionScopeHeader = "X-Auth-Scope"

var ErrPermissionScopeInvalid = apperror.New("AUTH_INFO_SCOPE_INVALID", "invalid permission scope")
var permissionScopePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

type permissionScopeKey struct{}
type httpPermissionRequestKey struct{}

func IsHTTPPermissionRequest(ctx context.Context) bool {
	value, _ := ctx.Value(httpPermissionRequestKey{}).(bool)
	return value
}

func WithPermissionScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, permissionScopeKey{}, scope)
}

func PermissionScope(ctx context.Context) string {
	if scope, ok := ctx.Value(permissionScopeKey{}).(string); ok {
		return scope
	}
	return DefaultPermissionScope
}

// ScopeRequest fixes one scope for authentication and gateway authorization.
// Older clients retain admin scope; a supplied header must be unambiguous.
func ScopeRequest(r *http.Request) (*http.Request, error) {
	scope := DefaultPermissionScope
	values, present := r.Header[http.CanonicalHeaderKey(PermissionScopeHeader)]
	if present {
		if len(values) != 1 {
			return nil, ErrPermissionScopeInvalid
		}
		scope = strings.TrimSpace(values[0])
		if !permissionScopePattern.MatchString(scope) {
			return nil, ErrPermissionScopeInvalid
		}
	}

	return r.WithContext(context.WithValue(WithPermissionScope(r.Context(), scope), httpPermissionRequestKey{}, true)), nil
}
