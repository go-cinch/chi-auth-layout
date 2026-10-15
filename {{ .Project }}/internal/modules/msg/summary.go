package msg

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

// SentSummary counts all surviving publications, including expired ones, like ListSent.
// Personal deletion and read receipts do not change management statistics.
func (m *Module) SentSummary(ctx context.Context, fields ...string) (*report.ManagementSummary, error) {
	selection := report.Fields(fields)
	if err := selection.Validate("total", "counts"); err != nil {
		return nil, err
	}
	value := report.New("notice", "system", "all", "targeted", "latest_published_at")
	value.Fields = selection
	var notices, systems, broadcasts, targeted, latestPublishedAt int64

	projection := report.Projection{Fields: selection}
	projection.Add("total", `COUNT(*)`, &value.Total)
	projection.Add("counts", `COUNT(*) FILTER (WHERE type = 'notice')`, &notices)
	projection.Add("counts", `COUNT(*) FILTER (WHERE type = 'system')`, &systems)
	projection.Add("counts", `COUNT(*) FILTER (WHERE scope = 'all')`, &broadcasts)
	projection.Add("counts", `COUNT(*) FILTER (WHERE scope = 'targeted')`, &targeted)
	projection.Add("counts", `COALESCE(MAX(published_at), 0)`, &latestPublishedAt)
	query := projection.SQL("t_msg")
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize msg: %w", err)
	}
	value.Counts["notice"] = notices
	value.Counts["system"] = systems
	value.Counts["all"] = broadcasts
	value.Counts["targeted"] = targeted
	value.Counts["latest_published_at"] = latestPublishedAt
	return value, nil
}
