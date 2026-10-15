package whitelist

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context, fields ...string) (*report.ManagementSummary, error) {
	selection := report.Fields(fields)
	if err := selection.Validate("total", "counts"); err != nil {
		return nil, err
	}
	value := report.New("permission", "jwt")
	value.Fields = selection
	var permission, jwt int64

	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `COUNT(*) FILTER (WHERE category = 0)`, &permission)
	projection.Add("counts", `COUNT(*) FILTER (WHERE category = 1)`, &jwt)
	query := projection.SQL("t_whitelist")
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize whitelist: %w", err)
	}
	value.Counts["permission"] = permission
	value.Counts["jwt"] = jwt
	return value, nil
}
