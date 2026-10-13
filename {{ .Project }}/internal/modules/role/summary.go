package role

import (
	"context"
	"encoding/json"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("with_permissions", "without_permissions")
	var withPermissions, withoutPermissions int64
	var groups []byte
	query := `SELECT COUNT(*), COUNT(*) FILTER (WHERE ` + assignedPermissionsSQL + `), COUNT(*) FILTER (WHERE NOT ` + assignedPermissionsSQL + `),
 COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.name) FROM (
 SELECT a.action_group AS name, COUNT(*) AS count FROM t_role r JOIN t_action a ON a.code = ANY(regexp_split_to_array(BTRIM(r.action), '\s*,\s*')) GROUP BY a.action_group
 ) g), '[]'::json) FROM t_role`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &withPermissions, &withoutPermissions, &groups); err != nil {
		return nil, fmt.Errorf("summarize role: %w", err)
	}
	value.Counts["with_permissions"] = withPermissions
	value.Counts["without_permissions"] = withoutPermissions
	if err := json.Unmarshal(groups, &value.Groups); err != nil {
		return nil, fmt.Errorf("decode summary groups: %w", err)
	}
	return value, nil
}
