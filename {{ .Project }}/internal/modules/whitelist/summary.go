package whitelist

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

// Summary aggregates the complete resource set in one database snapshot.
func (m *Module) Summary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("permission", "jwt")
	var permission, jwt int64

	const query = `SELECT COUNT(*), COUNT(*) FILTER (WHERE category = 0), COUNT(*) FILTER (WHERE category = 1) FROM t_whitelist`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &permission, &jwt); err != nil {
		return nil, fmt.Errorf("summarize whitelist: %w", err)
	}
	value.Counts["permission"] = permission
	value.Counts["jwt"] = jwt
	return value, nil
}
