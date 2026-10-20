package auth

import (
	"context"
	"fmt"
	"net/http"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/action"
)

// The transaction lock serializes changes which can affect a caller's current
// grants. Read committed checks happen after the lock, so a queued mutation
// cannot use a grant removed by the preceding transaction.
func (m *Module) LockPermissionMutation(ctx context.Context) error {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID <= 0 {
		return authn.ErrPermissionAdministrationRequired
	}
	if _, err := m.store.SQL(ctx).ExecContext(ctx, `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		return fmt.Errorf("lock permission mutation: %w", err)
	}
	return nil
}

func (m *Module) RequireAllPermissions(ctx context.Context) error {
	if err := m.LockPermissionMutation(ctx); err != nil {
		return err
	}
	identity, _ := authn.FromContext(ctx)
	permission, err := m.permissionsFor(ctx, identity.UserID, authn.PermissionScope(ctx), report.Fields{"http"})
	if err != nil {
		return err
	}
	if permission.PermissionScope != "*" {
		return authn.ErrPermissionAdministrationRequired
	}
	return nil
}

func (m *Module) RequirePermissionEndpoint(ctx context.Context, method, path string) error {
	if err := m.LockPermissionMutation(ctx); err != nil {
		return err
	}
	_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: method, Path: path})
	if err != nil {
		return err
	}
	if !allowed {
		return authn.ErrPermissionGrantRequired
	}
	return nil
}

func (m *Module) RequireCatalogAdministration(ctx context.Context) error {
	if err := m.LockPermissionMutation(ctx); err != nil {
		return err
	}
	identity, _ := authn.FromContext(ctx)
	permission, err := m.permissionsFor(ctx, identity.UserID, authn.PermissionScope(ctx), report.Fields{"btn"})
	if err != nil {
		return err
	}
	if permission.PermissionScope == "*" || permission.buttonGrants[authn.PermissionScope(ctx)+":btn:system:developer:publish"] {
		return nil
	}
	return authn.ErrPermissionAdministrationRequired
}

// RequirePermissionOperation rechecks the selected CRUD binding and its concrete
// HTTP resource after taking the transaction lock. Directory references are
// expanded into those bindings when the action is saved, never on authorization.
func (m *Module) RequirePermissionOperation(ctx context.Context, resource, operation string) error {
	method, path, valid := permissionOperationTarget(resource, operation)
	if !valid {
		return authn.ErrPermissionGrantRequired
	}
	if err := m.LockPermissionMutation(ctx); err != nil {
		return err
	}
	identity, _ := authn.FromContext(ctx)
	catalog := m.catalogSource.Select([]string{"resource_catalog"})
	permission, err := m.permissionsFromCatalog(ctx, identity.UserID, authn.PermissionScope(ctx), report.Fields{"http"}, catalog, operation)
	if err != nil {
		return err
	}
	if permission.PermissionScope == "*" {
		return nil
	}
	for _, endpoint := range permission.endpoints {
		if endpoint.Protocol == "http" && endpoint.Method == method && canonicalRoute(endpoint.Path) == canonicalRoute(path) {
			return nil
		}
	}
	return authn.ErrPermissionGrantRequired
}

func permissionOperationTarget(resource, operation string) (string, string, bool) {
	switch resource {
	case "user", "role", "action", "user-group":
	default:
		return "", "", false
	}
	path := "/" + resource
	switch operation {
	case "read":
		return http.MethodGet, path, true
	case "create":
		return http.MethodPost, path, true
	case "update":
		return http.MethodPatch, path + "/{id}", true
	case "delete":
		return http.MethodDelete, path + "/{id}", true
	default:
		return "", "", false
	}
}

func permissionOperationConfig(config action.Config, operation string) action.Config {
	if config.Builtin == "all" || config.Builtin == "default" && operation == "read" {
		return config
	}
	selected := []action.ResourceBinding{}
	if config.Bindings != nil {
		for _, binding := range *config.Bindings {
			if binding.Operation == operation || binding.Operation == "*" {
				selected = append(selected, binding)
			}
		}
	} else if config.ResourceKey != "" && (config.Operation == operation || config.Operation == "*") {
		selected = append(selected, action.ResourceBinding{ResourceKey: config.ResourceKey, Operation: config.Operation, FeatureKeys: config.FeatureKeys})
	}
	// ResolveConfig regenerates endpoints from the catalog, ignoring stored caches.
	return action.Config{Bindings: &selected, UI: []action.UIBinding{}, Endpoints: []action.Endpoint{}}
}
