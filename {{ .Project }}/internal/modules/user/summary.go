package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

type SummaryInput struct {
	Fields   report.Fields
	Days     int
	TimeZone string
}

// Summary uses one request instant and one SQL statement for the status and
// creation/login aggregates. Reading an expired temporary lock never writes the user.
func (m *Module) Summary(ctx context.Context, input SummaryInput) (*report.ManagementSummary, error) {
	if err := input.Fields.Validate("total", "counts", "trend"); err != nil {
		return nil, ErrInvalid
	}
	now := time.Now()
	buckets, err := creationBuckets(input, now)
	if err != nil {
		return nil, err
	}
	starts := make([]int64, len(buckets))
	for i, bucket := range buckets {
		starts[i] = bucket.From
	}
	// Public day bounds remain complete for drilldown. Aggregates separately
	// exclude future instants while sharing the same captured request time.
	cutoff := min(buckets[len(buckets)-1].To, now.UnixMilli()+1)
	value := report.New("pending", "active", "locked", "recent_logged_in_users")
	value.Fields = input.Fields
	if !input.Fields.Has("counts") && !input.Fields.Has("trend") {
		err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_user").Scan(&value.Total)
		return value, err
	}
	var pending, active, locked, recentLoggedInUsers int64
	var countsJSON, loginCountsJSON []byte
	projection := report.Projection{Fields: input.Fields}
	projection.Add("total", "COUNT(*)", &value.Total)
	projection.Add("counts", "COUNT(*) FILTER (WHERE status = 0)", &pending)
	projection.Add("counts", "COUNT(*) FILTER (WHERE status = 1)", &active)
	projection.Add("counts", "COUNT(*) FILTER (WHERE status = 2)", &locked)
	projection.Add("counts", "COUNT(*) FILTER (WHERE last_logged_in_at > 0 AND last_logged_in_at >= ($2::bigint[])[1] AND last_logged_in_at < $3)", &recentLoggedInUsers)
	projection.Add("trend", "COALESCE((SELECT json_agg(COALESCE(c.count, 0) ORDER BY b.bucket) FROM buckets b LEFT JOIN bucket_counts c ON c.bucket = b.bucket), '[]'::json)", &countsJSON)
	projection.Add("trend", "COALESCE((SELECT json_agg(COALESCE(l.count, 0) ORDER BY b.bucket) FROM buckets b LEFT JOIN login_counts l ON l.bucket = b.bucket), '[]'::json)", &loginCountsJSON)
	prefix := `WITH effective AS (SELECT CASE WHEN ` + expiredLockSQL("$1") + ` THEN 1 ELSE status END AS status, created_at, last_logged_in_at FROM t_user)`
	if input.Fields.Has("trend") {
		prefix += `,
 bucket_counts AS (SELECT width_bucket(created_at, $2::bigint[]) AS bucket, COUNT(*) AS count FROM effective
 WHERE created_at >= ($2::bigint[])[1] AND created_at < $3 GROUP BY bucket),
 login_counts AS (SELECT width_bucket(last_logged_in_at, $2::bigint[]) AS bucket, COUNT(*) AS count FROM effective
 WHERE last_logged_in_at > 0 AND last_logged_in_at >= ($2::bigint[])[1] AND last_logged_in_at < $3 GROUP BY bucket),
 buckets AS (SELECT * FROM unnest($2::bigint[]) WITH ORDINALITY AS b(started_at, bucket))`
	}
	from := "effective"
	if !input.Fields.Has("total") && !input.Fields.Has("counts") {
		from = ""
	}
	query := prefix + " " + projection.SQL(from)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query, now.UnixMilli(), pq.Array(starts), cutoff).Scan(projection.Targets...); err != nil {
		return nil, fmt.Errorf("summarize users: %w", err)
	}
	value.Counts["pending"], value.Counts["active"], value.Counts["locked"] = pending, active, locked
	value.Counts["recent_logged_in_users"] = recentLoggedInUsers
	if !input.Fields.Has("trend") {
		return value, nil
	}
	var counts, loginCounts []int64
	if err := json.Unmarshal(countsJSON, &counts); err != nil {
		return nil, fmt.Errorf("decode creation counts: %w", err)
	}
	if len(counts) != len(buckets) {
		return nil, fmt.Errorf("creation bucket count mismatch")
	}
	if err := json.Unmarshal(loginCountsJSON, &loginCounts); err != nil {
		return nil, fmt.Errorf("decode latest-login counts: %w", err)
	}
	if len(loginCounts) != len(buckets) {
		return nil, fmt.Errorf("latest-login bucket count mismatch")
	}
	for i := range buckets {
		buckets[i].Count = counts[i]
		buckets[i].RecentLoggedInUsers = loginCounts[i]
	}
	value.Trend = buckets
	return value, nil
}

func creationBuckets(input SummaryInput, now time.Time) ([]report.ManagementPoint, error) {
	if input.Days == 0 {
		input.Days = 14
	}
	if input.Days != 14 && input.Days != 30 {
		return nil, ErrInvalid
	}
	if input.TimeZone == "" {
		input.TimeZone = "UTC"
	}
	location, err := time.LoadLocation(input.TimeZone)
	if err != nil || input.TimeZone == "Local" {
		return nil, ErrInvalid
	}
	local := now.In(location)
	// Advance nominal calendar dates independently of location transitions. Some
	// zones skip midnight; adding 24 hours or AddDate on a normalized midnight
	// would shift all later buckets away from their actual calendar boundaries.
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-input.Days)
	result := make([]report.ManagementPoint, 0, input.Days)
	for i := 0; i < input.Days; i++ {
		next := date.AddDate(0, 0, 1)
		result = append(result, report.ManagementPoint{Date: date.Format("2006-01-02"), From: calendarStart(date, location), To: calendarStart(next, location)})
		date = next
	}
	return result, nil
}

// calendarStart finds the first instant of a local date, including a skipped
// or repeated midnight. Noon anchors a bounded search; tzdb offsets are seconds.
func calendarStart(date time.Time, location *time.Location) int64 {
	noon := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location)
	lo, hi := noon.Add(-36*time.Hour).Unix(), noon.Unix()
	label := date.Format("2006-01-02")
	for lo < hi {
		mid := lo + (hi-lo)/2
		if time.Unix(mid, 0).In(location).Format("2006-01-02") < label {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo * 1000
}
