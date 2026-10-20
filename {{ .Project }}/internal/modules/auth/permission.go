package auth

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules"
	"{{ .Computed.module_name_final }}/internal/modules/action"
)

type Permission struct {
	PermissionScopes []string               `json:"permission_scopes,omitempty"`
	PermissionScope  string                 `json:"permission_scope,omitempty"`
	Words            []string               `json:"words"`
	Menus            []PermissionMenu       `json:"menus"`
	Navigation       []PermissionNavigation `json:"navigation"`
	// Internal transport rules never cross the info boundary.
	Resources    []string `json:"-"`
	Buttons      []string `json:"-"`
	buttonGrants map[string]bool
	endpoints    []action.Endpoint
}

type PermissionMenu struct {
	MenuKey       string   `json:"menu_key"`
	ParentMenuKey string   `json:"parent_menu_key,omitempty"`
	MenuPath      string   `json:"menu_path"`
	Buttons       []string `json:"btns"`
}

type PermissionTarget struct {
	Method   string
	Path     string
	Resource string
}

func (m *Module) AuthorizeHTTP(ctx context.Context, method, path string) (bool, error) {
	_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Method: method, Path: path})
	return allowed, err
}

func (m *Module) CheckPermission(ctx context.Context, target PermissionTarget) (authn.Identity, bool, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return authn.Identity{}, false, ErrUnauthorized
	}
	target = normalizePermissionTarget(target)
	if !target.valid() {
		return identity, false, nil
	}
	if identity.PasswordResetRequired && (target.Resource != "" || !PasswordResetAllowedHTTP(target.Method, target.Path, m.httpPath)) {
		return identity, false, authn.ErrPasswordResetRequired
	}
	protocol := "http"
	if target.Resource != "" {
		protocol = "grpc"
	}
	permission, err := m.permissionsFor(ctx, identity.UserID, authn.PermissionScope(ctx), report.Fields{protocol})
	if err != nil {
		return authn.Identity{}, false, err
	}
	resolved, registered := m.resolveHTTPRoute(target)
	for _, endpoint := range permission.endpoints {
		if registered {
			if endpoint.Protocol == "http" && endpoint.Method == target.Method && canonicalRoute(endpoint.Path) == canonicalRoute(resolved) {
				return identity, true, nil
			}
			continue
		}
		if matchEndpoint(endpoint, target) {
			return identity, true, nil
		}
	}
	if permission.PermissionScope == "*" {
		// A wildcard covers new routes in this scope, but never routes declared
		// exclusively by another scope in the shared resource catalog.
		scope := authn.PermissionScope(ctx)
		catalog := m.catalogSource.Select([]string{"resource_catalog"}).ResourceCatalog
		for key, feature := range catalog.Buttons {
			if strings.HasPrefix(key, scope+":") {
				continue
			}
			for _, endpoint := range feature.HTTP {
				if matchEndpoint(action.Endpoint{Protocol: "http", Method: endpoint.Method, Path: endpoint.Path}, target) {
					return identity, false, nil
				}
			}
			for _, path := range feature.GRPC {
				if matchEndpoint(action.Endpoint{Protocol: "grpc", Method: "GRPC", Path: path}, target) {
					return identity, false, nil
				}
			}
		}
		return identity, true, nil
	}
	return identity, false, nil
}

func normalizePermissionTarget(target PermissionTarget) PermissionTarget {
	target.Method = strings.ToUpper(strings.TrimSpace(target.Method))
	target.Path = strings.TrimSpace(target.Path)
	if index := strings.IndexByte(target.Path, '?'); index >= 0 {
		target.Path = target.Path[:index]
	}
	target.Resource = normalizeRPCResource(target.Resource)
	return target
}

func normalizeRPCResource(value string) string {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func (target PermissionTarget) valid() bool {
	return (target.Method != "" && target.Path != "") || target.Resource != ""
}

func matchEndpoint(endpoint action.Endpoint, target PermissionTarget) bool {
	if target.Resource != "" {
{{ if .Computed.enable_grpc_final }}
		return endpoint.Protocol == "grpc" && endpoint.Path == target.Resource
{{ else }}
		return false
{{ end }}
	}
	return endpoint.Protocol == "http" && endpoint.Method == target.Method && matchRouteTemplate(endpoint.Path, target.Path)
}

// Router parameters match exactly one segment. Wildcards and path prefixes do
// not grant sibling routes or management APIs beneath personal endpoints.
func matchRouteTemplate(template, path string) bool {
	expected, actual := strings.Split(strings.TrimSuffix(template, "/"), "/"), strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(expected) != len(actual) {
		return false
	}
	for index, part := range expected {
		parameter := strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") || strings.HasPrefix(part, ":")
		if parameter {
			if actual[index] == "" || actual[index] == "." || actual[index] == ".." || strings.ContainsAny(actual[index], "%\\") {
				return false
			}
			continue
		}
		if part != actual[index] {
			return false
		}
	}
	return true
}

func (m *Module) permissions(ctx context.Context, userID int64, scopes ...string) (Permission, error) {
	scope := authn.DefaultPermissionScope
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	return m.permissionsFor(ctx, userID, scope, nil)
}

func (m *Module) permissionsFor(ctx context.Context, userID int64, scope string, fields report.Fields) (Permission, error) {
	catalog := m.catalogSource.Select(report.Fields{"resource_catalog"})
	return m.permissionsFromCatalog(ctx, userID, scope, fields, catalog)
}

func (m *Module) permissionsFromCatalog(ctx context.Context, userID int64, scope string, fields report.Fields, catalog localization.Catalog, operations ...string) (Permission, error) {
	needUI := fields.Has("btn") || fields.Has("menus")
	needWords := fields.Has("words")
	grantFields := report.Fields{}
	if needUI {
		grantFields = append(grantFields, "ui")
	}
	if fields.Has("http") {
		grantFields = append(grantFields, "http")
	}
	if fields.Has("grpc") {
		grantFields = append(grantFields, "grpc")
	}
	permission := Permission{Words: []string{}, Menus: []PermissionMenu{}, Resources: []string{}, Buttons: []string{}, endpoints: []action.Endpoint{}}
	const query = `WITH assigned AS (
	SELECT BTRIM(direct.code) AS code, 1 AS source_order, u.id AS assignment_order, direct.code_order
	FROM t_user AS u
	CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(u.action, ',')) WITH ORDINALITY AS direct(code, code_order)
	WHERE u.id = $1 AND BTRIM(direct.code) <> ''
	UNION ALL
	SELECT BTRIM(role_action.code), 2, r.id, role_action.code_order
	FROM t_user AS u
	JOIN t_role AS r ON r.id = u.role_id
	CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(r.action, ',')) WITH ORDINALITY AS role_action(code, code_order)
	WHERE u.id = $1 AND BTRIM(role_action.code) <> ''
	UNION ALL
	SELECT BTRIM(group_action.code), 3, relation.id, group_action.code_order
	FROM t_user_user_group_relation AS relation
	JOIN t_user_group AS user_group ON user_group.id = relation.user_group_id
	CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(user_group.action, ',')) WITH ORDINALITY AS group_action(code, code_order)
	WHERE relation.user_id = $1 AND BTRIM(group_action.code) <> ''
), deduplicated AS (
	SELECT code, source_order, assignment_order, code_order,
		ROW_NUMBER() OVER (PARTITION BY code ORDER BY source_order, assignment_order, code_order) AS occurrence
	FROM assigned
)
SELECT catalog.word, catalog.config - 'ui' - 'endpoints' AS config
FROM t_action AS catalog
WHERE catalog.scope = $2
AND EXISTS (SELECT 1 FROM deduplicated WHERE occurrence = 1 AND code = catalog.code)
ORDER BY catalog.id`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, userID, scope)
	if err != nil {
		return Permission{}, fmt.Errorf("query current user permissions: %w", err)
	}
	defer rows.Close()
	type menuGrant struct {
		menu  PermissionMenu
		entry bool
		seen  map[string]bool
	}
	menus := map[string]*menuGrant{}
	order := []string{}
	seenEndpoints := map[action.Endpoint]bool{}
	for rows.Next() {
		var word string
		var config action.Config
		if err := rows.Scan(&word, &config); err != nil {
			return Permission{}, fmt.Errorf("scan current user permissions: %w", err)
		}
		if !action.ValidWord(word) {
			return Permission{}, fmt.Errorf("invalid action catalog word")
		}
		if scope != "" && strings.Split(word, ":")[0] != scope {
			continue
		}
		if len(operations) > 0 {
			config = permissionOperationConfig(config, operations[0])
		}
		resolved, resolveErr := action.ResolveConfig(catalog, word, config, grantFields)
		if resolveErr != nil {
			return Permission{}, fmt.Errorf("resolve current permission: %w", resolveErr)
		}
		config = resolved
		if err := config.Validate(word); err != nil {
			return Permission{}, fmt.Errorf("invalid action catalog config: %w", err)
		}
		if config.PermissionScope == "*" {
			permission.PermissionScope = "*"
		}
		permission.PermissionScopes = append(permission.PermissionScopes, config.PermissionScopes...)
		if needWords {
			permission.Words = append(permission.Words, word)
		}
		for _, endpoint := range config.Endpoints {
			if !seenEndpoints[endpoint] {
				seenEndpoints[endpoint] = true
				permission.endpoints = append(permission.endpoints, endpoint)
				if fields.Has("resources") {
					permission.Resources = append(permission.Resources, endpoint.Display())
				}
			}
		}
		for _, binding := range config.UI {
			menu := menus[binding.MenuKey]
			if menu == nil {
				menu = &menuGrant{menu: PermissionMenu{MenuKey: binding.MenuKey, ParentMenuKey: binding.ParentMenuKey, MenuPath: binding.MenuPath, Buttons: []string{}}, seen: map[string]bool{}}
				menus[binding.MenuKey] = menu
				order = append(order, binding.MenuKey)
			}
			if menu.menu.MenuPath != binding.MenuPath || menu.menu.ParentMenuKey != binding.ParentMenuKey {
				return Permission{}, fmt.Errorf("conflicting action menu mapping")
			}
			menu.entry = menu.entry || binding.PageEntry
			for _, button := range binding.ButtonKeys {
				if permission.buttonGrants == nil {
					permission.buttonGrants = map[string]bool{}
				}
				permission.buttonGrants[button] = true
				if !menu.seen[button] {
					menu.seen[button] = true
					menu.menu.Buttons = append(menu.menu.Buttons, button)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Permission{}, fmt.Errorf("query current user permissions: %w", err)
	}
	for _, key := range order {
		menu := menus[key]
		if menu.entry {
			permission.Menus = append(permission.Menus, menu.menu)
			permission.Buttons = append(permission.Buttons, menu.menu.Buttons...)
		}
	}
	return permission, nil
}

func (m *Module) SetHTTPPermissionRoutes(routes []modules.HTTPPermissionRoute) {
	m.permissionRoutes = append([]modules.HTTPPermissionRoute{}, routes...)
}

func canonicalRoute(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") || (strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")) {
			parts[index] = "{}"
		}
	}
	return strings.Join(parts, "/")
}

func (m *Module) resolveHTTPRoute(target PermissionTarget) (string, bool) {
	if target.Resource != "" {
		return "", false
	}
	best, bestPriority := "", ""
	for _, route := range m.permissionRoutes {
		if route.Method != target.Method || !matchRouteTemplate(route.Path, target.Path) {
			continue
		}
		priority := ""
		for _, part := range strings.Split(canonicalRoute(route.Path), "/") {
			if part == "{}" {
				priority += "0"
			} else {
				priority += "1"
			}
		}
		if best == "" || priority > bestPriority {
			best, bestPriority = route.Path, priority
		}
	}
	return best, best != ""
}

// PermissionPaths returns the same effective rules enforced by CheckPermission.
func (m *Module) PermissionPaths(ctx context.Context, scope string, fields report.Fields) (action.PermissionPaths, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return action.PermissionPaths{}, ErrUnauthorized
	}
	permission, err := m.permissionsFor(ctx, identity.UserID, scope, fields)
	if err != nil {
		return action.PermissionPaths{}, err
	}
	result := action.PermissionPaths{Btn: []string{}, HTTP: []localization.HTTPPath{}, GRPC: []string{}}
	seen := map[string]bool{}
	for _, button := range permission.Buttons {
		if fields.Has("btn") && !seen[button] {
			result.Btn = append(result.Btn, button)
			seen[button] = true
		}
	}
	for _, endpoint := range permission.endpoints {
		if endpoint.Protocol == "http" && fields.Has("http") {
			result.HTTP = append(result.HTTP, localization.HTTPPath{Method: endpoint.Method, Path: endpoint.Path})
		} else if endpoint.Protocol == "grpc" && fields.Has("grpc") {
			result.GRPC = append(result.GRPC, endpoint.Path)
		}
	}
	sort.Strings(result.Btn)
	sort.Strings(result.GRPC)
	sort.Slice(result.HTTP, func(i, j int) bool {
		return result.HTTP[i].Method+result.HTTP[i].Path < result.HTTP[j].Method+result.HTTP[j].Path
	})
	return result, nil
}
