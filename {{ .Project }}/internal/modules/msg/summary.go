package msg

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

// SentSummary counts all surviving publications, including expired ones, like ListSent.
// Personal deletion and read receipts do not change management statistics.
func (m *Module) SentSummary(ctx context.Context) (*report.ManagementSummary, error) {
	value := report.New("notice", "system", "all", "targeted", "latest_published_at")
	var notices, systems, broadcasts, targeted, latestPublishedAt int64

	const query = `SELECT COUNT(*), COUNT(*) FILTER (WHERE type = 'notice'), COUNT(*) FILTER (WHERE type = 'system'),
 COUNT(*) FILTER (WHERE scope = 'all'), COUNT(*) FILTER (WHERE scope = 'targeted'), COALESCE(MAX(published_at), 0) FROM t_msg`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query).Scan(&value.Total, &notices, &systems, &broadcasts, &targeted, &latestPublishedAt); err != nil {
		return nil, fmt.Errorf("summarize msg: %w", err)
	}
	value.Counts["notice"] = notices
	value.Counts["system"] = systems
	value.Counts["all"] = broadcasts
	value.Counts["targeted"] = targeted
	value.Counts["latest_published_at"] = latestPublishedAt
	return value, nil
}
