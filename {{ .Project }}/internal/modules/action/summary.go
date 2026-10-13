package action

import (
	"context"
	"encoding/json"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("resource", "menu", "button")
	var resources, menus, buttons int64
	var groups []byte
	const query = `SELECT COUNT(*), COUNT(*) FILTER (WHERE BTRIM(resource) <> ''), COUNT(*) FILTER (WHERE BTRIM(menu) <> ''), COUNT(*) FILTER (WHERE BTRIM(btn) <> ''),
 COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.name) FROM (
 SELECT action_group AS name, COUNT(*) AS count FROM t_action GROUP BY action_group
 ) g), '[]'::json) FROM t_action`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &resources, &menus, &buttons, &groups); err != nil {
		return nil, fmt.Errorf("summarize action: %w", err)
	}
	value.Counts["resource"] = resources
	value.Counts["menu"] = menus
	value.Counts["button"] = buttons
	if err := json.Unmarshal(groups, &value.Groups); err != nil {
		return nil, fmt.Errorf("decode summary groups: %w", err)
	}
	return value, nil
}
