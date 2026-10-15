package action

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
	value := report.New("resource", "menu", "button")
	value.Fields = selection
	var resources, menus, buttons int64
	var groups []byte
	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `COUNT(*) FILTER (WHERE BTRIM(resource) <> '')`, &resources)
	projection.Add("counts", `COUNT(*) FILTER (WHERE BTRIM(menu) <> '')`, &menus)
	projection.Add("counts", `COUNT(*) FILTER (WHERE BTRIM(btn) <> '')`, &buttons)
	projection.Add("groups", `COALESCE((SELECT json_agg(g ORDER BY g.count DESC, g.name) FROM (
 SELECT action_group AS name, COUNT(*) AS count FROM t_action GROUP BY action_group
 ) g), '[]'::json)`, &groups)
	from := "t_action"
	if !selection.Has("total") && !selection.Has("counts") {
		from = ""
	}
	query := projection.SQL(from)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize action: %w", err)
	}
	value.Counts["resource"] = resources
	value.Counts["menu"] = menus
	value.Counts["button"] = buttons
	if selection.Has("groups") {
		if err := json.Unmarshal(groups, &value.Groups); err != nil {
			return nil, fmt.Errorf("decode summary groups: %w", err)
		}
	}
	return value, nil
}
