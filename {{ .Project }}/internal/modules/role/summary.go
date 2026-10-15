package role

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
	value := report.New("with_permissions", "without_permissions")
	value.Fields = selection
	var withPermissions, withoutPermissions int64
	var groups []byte
	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `COUNT(*) FILTER (WHERE `+assignedPermissionsSQL+`)`, &withPermissions)
	projection.Add("counts", `COUNT(*) FILTER (WHERE NOT `+assignedPermissionsSQL+`)`, &withoutPermissions)
	projection.Add("groups", `COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.name) FROM (
 SELECT a.action_group AS name, COUNT(*) AS count FROM t_role r JOIN t_action a ON a.code = ANY(regexp_split_to_array(BTRIM(r.action), '\s*,\s*')) GROUP BY a.action_group
 ) g), '[]'::json)`, &groups)
	from := "t_role"
	if !selection.Has("total") && !selection.Has("counts") {
		from = ""
	}
	query := projection.SQL(from)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize role: %w", err)
	}
	value.Counts["with_permissions"] = withPermissions
	value.Counts["without_permissions"] = withoutPermissions
	if selection.Has("groups") {
		if err := json.Unmarshal(groups, &value.Groups); err != nil {
			return nil, fmt.Errorf("decode summary groups: %w", err)
		}
	}
	return value, nil
}
