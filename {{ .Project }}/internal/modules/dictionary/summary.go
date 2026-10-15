package dictionary

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
	value := report.New("enabled", "disabled")
	value.Fields = selection
	var enabled, disabled int64

	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `COUNT(*) FILTER (WHERE enabled)`, &enabled)
	projection.Add("counts", `COUNT(*) FILTER (WHERE NOT enabled)`, &disabled)
	query := projection.SQL("t_dictionary")
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize dictionary: %w", err)
	}
	value.Counts["enabled"] = enabled
	value.Counts["disabled"] = disabled
	return value, nil
}
