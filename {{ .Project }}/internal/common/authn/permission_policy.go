package authn

import (
	"context"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

var ErrPermissionAdministrationRequired = apperror.New("AUTH_PERMISSION_ADMIN_REQUIRED", "permission administration capability is required")

var ErrPermissionGrantRequired = apperror.New("AUTH_PERMISSION_GRANT_REQUIRED", "permission grant capability is required")

type AllPermissionsPolicy interface{ RequireAllPermissions(context.Context) error }
type PermissionGrantPolicy interface{ RequirePermissionGrant(context.Context) error }
type PermissionMutationPolicy interface{ LockPermissionMutation(context.Context) error }

type CatalogAdministrationPolicy interface{ RequireCatalogAdministration(context.Context) error }

func RequireAdministrationGrant(ctx context.Context, policy AllPermissionsPolicy) error {
	if granted, ok := policy.(PermissionGrantPolicy); ok {
		return granted.RequirePermissionGrant(ctx)
	}
	return RequireAllPermissions(ctx, policy)
}

func RequireCatalogAdministration(ctx context.Context, policy AllPermissionsPolicy) error {
	if catalog, ok := policy.(CatalogAdministrationPolicy); ok {
		return catalog.RequireCatalogAdministration(ctx)
	}
	return RequireAllPermissions(ctx, policy)
}

// A missing policy is permitted only for identity-free internal construction.
// Every production HTTP mutation has a verified identity, so an unwired policy
// fails closed even when the ordinary route authorization switch is disabled.
func RequireAllPermissions(ctx context.Context, policy AllPermissionsPolicy) error {
	if policy != nil {
		return policy.RequireAllPermissions(ctx)
	}
	if _, authenticated := FromContext(ctx); authenticated {
		return ErrPermissionAdministrationRequired
	}
	if IsHTTPPermissionRequest(ctx) {
		return ErrPermissionAdministrationRequired
	}
	return nil
}

func RequirePermissionGrant(ctx context.Context, policy PermissionGrantPolicy) error {
	if policy != nil {
		return policy.RequirePermissionGrant(ctx)
	}
	if _, authenticated := FromContext(ctx); authenticated {
		return ErrPermissionGrantRequired
	}
	if IsHTTPPermissionRequest(ctx) {
		return ErrPermissionGrantRequired
	}
	return nil
}

func LockPermissionMutation(ctx context.Context, policy PermissionMutationPolicy) error {
	if policy != nil {
		return policy.LockPermissionMutation(ctx)
	}
	if _, authenticated := FromContext(ctx); authenticated {
		return ErrPermissionAdministrationRequired
	}
	if IsHTTPPermissionRequest(ctx) {
		return ErrPermissionAdministrationRequired
	}
	return nil
}

// PermissionEndpointPolicy checks the concrete operation being performed. A
// named action is never itself an administration capability.
type PermissionEndpointPolicy interface {
	RequirePermissionEndpoint(context.Context, string, string) error
}

func RequirePermissionEndpoint(ctx context.Context, policy any, method, path string) error {
	if endpoint, ok := policy.(PermissionEndpointPolicy); ok {
		return endpoint.RequirePermissionEndpoint(ctx, method, path)
	}
	if all, ok := policy.(AllPermissionsPolicy); ok {
		return RequireAllPermissions(ctx, all)
	}
	if policy == nil {
		if _, authenticated := FromContext(ctx); !authenticated && !IsHTTPPermissionRequest(ctx) {
			return nil
		}
	}
	return ErrPermissionGrantRequired
}

type PermissionOperationPolicy interface {
	RequirePermissionOperation(context.Context, string, string) error
}

func RequirePermissionOperation(ctx context.Context, policy any, resource, operation string) error {
	if configured, ok := policy.(PermissionOperationPolicy); ok {
		return configured.RequirePermissionOperation(ctx, resource, operation)
	}
	if all, ok := policy.(AllPermissionsPolicy); ok {
		return RequireAllPermissions(ctx, all)
	}
	if policy == nil {
		if _, authenticated := FromContext(ctx); !authenticated && !IsHTTPPermissionRequest(ctx) {
			return nil
		}
	}
	return ErrPermissionGrantRequired
}
