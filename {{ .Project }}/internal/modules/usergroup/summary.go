package usergroup

import (
	"context"
	"encoding/json"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context, fields ...string) (*report.ManagementSummary, error) {
	selection := report.Fields(fields)
	if err := selection.Validate("total", "counts", "groups"); err != nil {
		return nil, err
	}
	value := report.New("memberships", "unique_members", "empty_groups", "with_permissions", "without_permissions")
	value.Fields = selection
	var memberships, uniqueMembers, emptyGroups, withPermissions, withoutPermissions int64
	var groups []byte
	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `(SELECT COUNT(*) FROM t_user_user_group_relation)`, &memberships)
	projection.Add("counts", `(SELECT COUNT(DISTINCT user_id) FROM t_user_user_group_relation)`, &uniqueMembers)
	projection.Add("counts", `COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM t_user_user_group_relation r WHERE r.user_group_id = t_user_group.id))`, &emptyGroups)
	projection.Add("groups", `COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.id) FROM (
 SELECT g.id, g.name, COUNT(r.user_id) AS count FROM t_user_group g LEFT JOIN t_user_user_group_relation r ON r.user_group_id = g.id GROUP BY g.id ORDER BY count DESC, g.id LIMIT 8
 ) g), '[]'::json)`, &groups)
	projection.Add("counts", `COUNT(*) FILTER (WHERE `+assignedPermissionsSQL+`)`, &withPermissions)
	projection.Add("counts", `COUNT(*) FILTER (WHERE NOT `+assignedPermissionsSQL+`)`, &withoutPermissions)
	from := "t_user_group"
	if !selection.Has("total") && !selection.Has("counts") {
		from = ""
	}
	query := projection.SQL(from)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize usergroup: %w", err)
	}
	value.Counts["with_permissions"] = withPermissions
	value.Counts["without_permissions"] = withoutPermissions
	value.Counts["memberships"] = memberships
	value.Counts["unique_members"] = uniqueMembers
	value.Counts["empty_groups"] = emptyGroups
	if selection.Has("groups") {
		if err := json.Unmarshal(groups, &value.Groups); err != nil {
			return nil, fmt.Errorf("decode summary groups: %w", err)
		}
	}
	return value, nil
}
