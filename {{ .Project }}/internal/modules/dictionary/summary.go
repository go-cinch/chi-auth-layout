package dictionary

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("enabled", "disabled")
	var enabled, disabled int64

	const query = `SELECT COUNT(*), COUNT(*) FILTER (WHERE enabled), COUNT(*) FILTER (WHERE NOT enabled) FROM t_dictionary`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &enabled, &disabled); err != nil {
		return nil, fmt.Errorf("summarize dictionary: %w", err)
	}
	value.Counts["enabled"] = enabled
	value.Counts["disabled"] = disabled
	return value, nil
}
