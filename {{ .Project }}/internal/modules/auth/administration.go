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

// Edit and registration review share a PATCH route. Check the selected control
// and its update endpoint together, under the same permission mutation lock.
func (m *Module) RequireUserEditControl(ctx context.Context, control string) error {
	return m.requirePatchControl(ctx, "user", "update", control, "/user/{id}")
}

// Group creation, editing and deletion share a narrow endpoint. The selected
// feature must grant both the corresponding button and the concrete route.
func (m *Module) RequireActionGroupControl(ctx context.Context, control string) error {
	operation, path := "", "/action/group"
	switch control {
	case "create-group":
		operation = "create"
	case "edit-group":
		operation = "update"
	case "delete-group":
		operation = "delete"
	case "reorder-groups":
		operation, path = "update", "/action/group/order"
	default:
		return authn.ErrPermissionGrantRequired
	}
	return m.requirePatchControl(ctx, "action", operation, control, path)
}

func (m *Module) requirePatchControl(ctx context.Context, resource, operation, control, path string) error {
	if err := m.LockPermissionMutation(ctx); err != nil {
		return err
	}
	identity, _ := authn.FromContext(ctx)
	scope := authn.PermissionScope(ctx)
	catalog := m.catalogSource.Select(report.Fields{"resource_catalog"})
	permission, err := m.permissionsFromCatalog(ctx, identity.UserID, scope, report.Fields{"http", "btn"}, catalog, operation)
	if err != nil || permission.PermissionScope == "*" {
		return err
	}
	if permission.buttonGrants[scope+":btn:system:"+resource+":"+control] {
		for _, endpoint := range permission.endpoints {
			if endpoint.Protocol == "http" && endpoint.Method == http.MethodPatch && canonicalRoute(endpoint.Path) == canonicalRoute(path) {
				return nil
			}
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
