package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type Permission struct {
	Resources []string `json:"resources"`
	Menus     []string `json:"menus"`
	Buttons   []string `json:"btns"`
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

	if identity.PasswordResetRequired && (target.Resource != "" || !authn.PasswordResetAllowedHTTP(target.Method, target.Path)) {
		return identity, false, authn.ErrPasswordResetRequired
	}
	permission, err := m.permissions(ctx, identity.UserID)
	if err != nil {
		return authn.Identity{}, false, err
	}
	for _, rule := range permission.Resources {
		if matchPermissionRule(rule, target) {
			return identity, true, nil
		}
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
	httpTarget := target.Method != "" && target.Path != ""
	return httpTarget || target.Resource != ""
}

func matchPermissionRule(rule string, target PermissionTarget) bool {
	rule = strings.TrimSpace(rule)
	if rule == "*" {
		return true
	}
	parts := strings.Split(rule, "|")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	methodSpec := strings.TrimSpace(parts[0])
	pathPattern := strings.TrimSpace(parts[1])
	rpcResource := ""
	if len(parts) == 3 {
		rpcResource = normalizeRPCResource(parts[2])
		if rpcResource == "" {
			return false
		}
	}
	if target.Resource != "" && rpcResource != "" && target.Resource == rpcResource {
		return true
	}
	if target.Method == "" || target.Path == "" || methodSpec == "" || pathPattern == "" {
		return false
	}
	methodMatches := false
	for method := range strings.SplitSeq(methodSpec, ",") {
		if strings.EqualFold(strings.TrimSpace(method), target.Method) {
			methodMatches = true
			break
		}
	}
	return methodMatches && matchPermissionGlob(pathPattern, target.Path)
}

func matchPermissionGlob(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == value
	}
	expression := regexp.QuoteMeta(pattern)
	expression = strings.ReplaceAll(expression, `\*`, `.*`)
	expression = strings.ReplaceAll(expression, `\?`, `.`)
	return regexp.MustCompile("^" + expression + "$").MatchString(value)
}

func (m *Module) permissions(ctx context.Context, userID int64) (Permission, error) {
	permission := Permission{
		Resources: make([]string, 0),
		Menus:     make([]string, 0),
		Buttons:   make([]string, 0),
	}
	const query = `WITH assigned AS (
	SELECT BTRIM(a.code) AS code, 0 AS source_order, a.id AS assignment_order, 0::BIGINT AS code_order
	FROM t_action AS a
	WHERE a.word = 'default'
	UNION ALL
	SELECT BTRIM(direct.code), 1, u.id, direct.code_order
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
SELECT action.resource, action.menu, action.btn
FROM deduplicated
JOIN t_action AS action ON BTRIM(action.code) = deduplicated.code
WHERE deduplicated.occurrence = 1
ORDER BY deduplicated.source_order, deduplicated.assignment_order, deduplicated.code_order, action.id`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, userID)
	if err != nil {
		return Permission{}, fmt.Errorf("query current user permissions: %w", err)
	}
	defer rows.Close()

	seenResources := make(map[string]struct{})
	seenMenus := make(map[string]struct{})
	seenButtons := make(map[string]struct{})
	for rows.Next() {
		var resources, menus, buttons string
		if err := rows.Scan(&resources, &menus, &buttons); err != nil {
			return Permission{}, fmt.Errorf("scan current user permissions: %w", err)
		}
		appendPermissionLines(&permission.Resources, seenResources, resources)
		appendPermissionLines(&permission.Menus, seenMenus, menus)
		appendPermissionLines(&permission.Buttons, seenButtons, buttons)
	}
	if err := rows.Err(); err != nil {
		return Permission{}, fmt.Errorf("query current user permissions: %w", err)
	}
	return permission, nil
}

func appendPermissionLines(destination *[]string, seen map[string]struct{}, value string) {
	for line := range strings.SplitSeq(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		*destination = append(*destination, line)
	}
}
