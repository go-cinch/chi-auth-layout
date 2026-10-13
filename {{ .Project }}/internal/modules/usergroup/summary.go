package usergroup

import (
	"context"
	"encoding/json"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("memberships", "unique_members", "empty_groups", "with_permissions", "without_permissions")
	var memberships, uniqueMembers, emptyGroups, withPermissions, withoutPermissions int64
	var groups []byte
	query := `SELECT COUNT(*), (SELECT COUNT(*) FROM t_user_user_group_relation), (SELECT COUNT(DISTINCT user_id) FROM t_user_user_group_relation),
 COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM t_user_user_group_relation r WHERE r.user_group_id = t_user_group.id)),
 COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.id) FROM (
 SELECT g.id, g.name, COUNT(r.user_id) AS count FROM t_user_group g LEFT JOIN t_user_user_group_relation r ON r.user_group_id = g.id GROUP BY g.id ORDER BY count DESC, g.id LIMIT 8
 ) g), '[]'::json), COUNT(*) FILTER (WHERE ` + assignedPermissionsSQL + `), COUNT(*) FILTER (WHERE NOT ` + assignedPermissionsSQL + `) FROM t_user_group`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &memberships, &uniqueMembers, &emptyGroups, &groups, &withPermissions, &withoutPermissions); err != nil {
		return nil, fmt.Errorf("summarize usergroup: %w", err)
	}
	value.Counts["with_permissions"] = withPermissions
	value.Counts["without_permissions"] = withoutPermissions
	value.Counts["memberships"] = memberships
	value.Counts["unique_members"] = uniqueMembers
	value.Counts["empty_groups"] = emptyGroups
	if err := json.Unmarshal(groups, &value.Groups); err != nil {
		return nil, fmt.Errorf("decode summary groups: %w", err)
	}
	return value, nil
}
