package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/report"

	"github.com/lib/pq"
)

type Bucket struct {
	MFAChallenge int64 `json:"mfa_challenge,omitempty"`
	At           int64 `json:"at"`
	EndedAt      int64 `json:"ended_at"`
	Success      int64 `json:"success"`
	Failure      int64 `json:"failure"`
}

type HeatCell struct {
	MFAChallenge int64 `json:"mfa_challenge,omitempty"`
	StartedAt int64 `json:"started_at"`
	EndedAt   int64 `json:"ended_at"`
	Weekday   int   `json:"weekday"`
	Hour      int   `json:"hour"`
	Count     int64 `json:"count"`
	Failed    int64 `json:"failed"`
}
type Breakdown struct {
	Name   string `json:"name"`
	Total  int64  `json:"total"`
	Failed int64  `json:"failed"`
}
type Summary struct {
	fields             report.Fields
	HeatmapGranularity string      `json:"heatmap_granularity,omitempty"`
	BucketMS           int64       `json:"bucket_ms,omitempty"`
	Accounts           []Breakdown `json:"accounts,omitempty"`
	Sources            []Breakdown `json:"sources,omitempty"`
	Total              int64       `json:"total,omitempty"`
	Success            int64       `json:"success,omitempty"`
	Failed             int64       `json:"failed,omitempty"`
	UniqueUsers        int64       `json:"unique_users,omitempty"`
	UniqueIPs          int64       `json:"unique_ips,omitempty"`
	AverageMS          float64     `json:"average_ms,omitempty"`
	P95MS              float64     `json:"p95_ms,omitempty"`
	SuccessRate        float64     `json:"success_rate,omitempty"`
	From               *int64      `json:"from"`
	To                 *int64      `json:"to"`
	Trend              []Bucket    `json:"trend,omitempty"`
	Heatmap            []HeatCell  `json:"heatmap,omitempty"`
	Modules            []Breakdown `json:"modules,omitempty"`
	RepeatedFailures   []Breakdown `json:"repeated_failures,omitempty"`
}

func (m *Module) Summary(ctx context.Context, f Filter, fields ...string) (*Summary, error) {
	selection := report.Fields(fields)
	if err := selection.Validate("metrics", "trend", "heatmap", "modules", "accounts", "sources", "repeated_failures"); err != nil {
		return nil, ErrFilter
	}
	f, err := normalize(f, m.settings)
	if err != nil {
		return nil, err
	}
	clause, args := where(f)
	chartFilter := f
	if selection.Has("trend") || selection.Has("heatmap") {
		chartFilter, err = m.summaryChartFilter(ctx, f, clause, args)
		if err != nil {
			return nil, err
		}
	}

	result := &Summary{fields: selection, Accounts: []Breakdown{}, Sources: []Breakdown{}, From: f.From, To: f.To, Trend: []Bucket{}, Heatmap: []HeatCell{}, Modules: []Breakdown{}, RepeatedFailures: []Breakdown{}}
	if selection.Has("metrics") {
		if err := m.loadSummaryMetrics(ctx, clause, args, result); err != nil {
			return nil, err
		}
	}
	if selection.Has("trend") {
		if err := m.loadSummaryTrend(ctx, chartFilter, clause, args, result); err != nil {
			return nil, err
		}
	}
	if selection.Has("heatmap") {
		if err := m.loadSummaryHeatmap(ctx, chartFilter, clause, args, result); err != nil {
			return nil, err
		}
	}
	if selection.Has("modules") {
		result.Modules, err = m.breakdown(ctx, clause, args, "module", 0)
		if err != nil {
			return nil, err
		}
	}
	if selection.Has("repeated_failures") {
		result.RepeatedFailures, err = m.breakdown(ctx, clause, args, "username", 5)
		if err != nil {
			return nil, err
		}
	}
	if selection.Has("accounts") {
		result.Accounts, err = m.breakdown(ctx, clause, args, "username", 0)
		if err != nil {
			return nil, err
		}
	}
	if selection.Has("sources") {
		result.Sources, err = m.breakdown(ctx, clause, args, "ip", 0)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (m *Module) summaryChartFilter(ctx context.Context, f Filter, clause string, args []any) (Filter, error) {
	if f.From != nil && f.To != nil {
		return f, nil
	}
	chartFilter := f
	var first, last sql.NullInt64
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT MIN(occurred_at), MAX(occurred_at) FROM t_audit_event"+clause, args...).Scan(&first, &last); err != nil {
		return f, err
	}
	// Missing public bounds remain absent. Only chart allocation uses data extents.
	if chartFilter.From == nil {
		from := last.Int64
		if first.Valid {
			from = first.Int64
		} else if f.To != nil {
			from = *f.To - 1
		}
		chartFilter.From = &from
	}
	if chartFilter.To == nil {
		to := *chartFilter.From + 1
		if last.Valid {
			to = last.Int64 + 1
		}
		chartFilter.To = &to
	}
	if *chartFilter.To-*chartFilter.From > m.settings.MaxWindow.Milliseconds() {
		return f, ErrFilter
	}
	return chartFilter, nil
}

func (m *Module) loadSummaryMetrics(ctx context.Context, clause string, args []any, result *Summary) error {
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE `+successSQL+`),COUNT(*) FILTER (WHERE `+failureSQL+`),COUNT(DISTINCT NULLIF(username,'')),COUNT(DISTINCT NULLIF(ip,'')),COALESCE(AVG(duration_ms),0),COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),0) FROM t_audit_event`+clause, args...).Scan(&result.Total, &result.Success, &result.Failed, &result.UniqueUsers, &result.UniqueIPs, &result.AverageMS, &result.P95MS)
	if err != nil {
		return err
	}
	if completed := result.Success + result.Failed; completed > 0 {
		result.SuccessRate = float64(result.Success) * 100 / float64(completed)
	}
	return nil
}

func (m *Module) loadSummaryTrend(ctx context.Context, f Filter, clause string, args []any, result *Summary) error {
	// Keep hourly bins epoch-aligned. Longer ranges follow the same local
	// calendar boundaries as the heatmap, including 23- and 25-hour days.
	hour := time.Hour.Milliseconds()
	daily := *f.To-*f.From > 24*hour
	at := *f.From
	result.BucketMS = hour
	if daily {
		result.BucketMS = 24 * hour // nominal granularity, not the interval length
	} else {
		at = (at / hour) * hour
	}
	location, err := time.LoadLocation(f.Timezone)
	if err != nil {
		return err
	}
	var starts []int64
	for at < *f.To {
		end := at + hour
		if daily {
			end = nextCalendarDay(time.UnixMilli(at).In(location)).UnixMilli()
		}
		end = min(end, *f.To)
		starts = append(starts, at)
		result.Trend = append(result.Trend, Bucket{At: at, EndedAt: end})
		at = end
	}

	// A one-sided filter may gain newer rows after we read its matching extents.
	// Use the exclusive end as a sentinel so those rows cannot enter the last bin.
	starts = append(starts, *f.To)
	bucketArgs := append(append([]any{}, args...), pq.Array(starts))
	query := fmt.Sprintf(`SELECT width_bucket(occurred_at,$%d::bigint[]),COUNT(*) FILTER (WHERE `+successSQL+`),COUNT(*) FILTER (WHERE `+failureSQL+`),COUNT(*) FILTER (WHERE `+challengeSQL+`) FROM t_audit_event`, len(bucketArgs))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query+clause+" GROUP BY 1", bucketArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var counts Bucket
		if err := rows.Scan(&index, &counts.Success, &counts.Failure, &counts.MFAChallenge); err != nil {
			return err
		}
		if index > 0 && index <= len(result.Trend) {
			bucket := &result.Trend[index-1]
			bucket.Success, bucket.Failure, bucket.MFAChallenge = counts.Success, counts.Failure, counts.MFAChallenge
		}
	}
	return rows.Err()
}

func (m *Module) loadSummaryHeatmap(ctx context.Context, f Filter, clause string, args []any, result *Summary) error {
	result.Heatmap, result.HeatmapGranularity = heatCells(f)
	thresholds := make([]int64, len(result.Heatmap))
	for i, cell := range result.Heatmap {
		thresholds[i] = cell.StartedAt
	}
	heatArgs := append(append([]any{}, args...), pq.Array(thresholds))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, fmt.Sprintf("SELECT width_bucket(occurred_at,$%d::bigint[]),COUNT(*),COUNT(*) FILTER (WHERE "+failureSQL+"),COUNT(*) FILTER (WHERE "+challengeSQL+") FROM t_audit_event", len(heatArgs))+clause+" GROUP BY 1", heatArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var count, failed, challenges int64
		if err := rows.Scan(&index, &count, &failed, &challenges); err != nil {
			return err
		}
		if index > 0 && index <= len(result.Heatmap) {
			result.Heatmap[index-1].Count = count
			result.Heatmap[index-1].Failed = failed
			result.Heatmap[index-1].MFAChallenge = challenges
		}
	}
	return rows.Err()
}

func (m *Module) breakdown(ctx context.Context, clause string, args []any, field string, minimum int64) ([]Breakdown, error) {
	if field != "module" && field != "username" && field != "ip" {
		return nil, ErrFilter
	}
	clause += " AND " + field + " <> ''"
	extra := ""
	if minimum > 0 {
		args = append(append([]any{}, args...), minimum)
		extra = fmt.Sprintf(" HAVING COUNT(*) FILTER (WHERE "+failureSQL+") >= $%d", len(args))
	}
	ranking := "2 DESC,3 DESC,1"
	if field == "username" {
		ranking = "3 DESC,2 DESC,1"
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT "+field+",COUNT(*),COUNT(*) FILTER (WHERE "+failureSQL+") FROM t_audit_event"+clause+" GROUP BY "+field+extra+" ORDER BY "+ranking+" LIMIT 10", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Breakdown{}
	for rows.Next() {
		var b Breakdown
		if err := rows.Scan(&b.Name, &b.Total, &b.Failed); err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

// Range bounds describe every selected aggregate; chart metadata follows its chart.
func (s Summary) MarshalJSON() ([]byte, error) {
	value := map[string]any{"from": s.From, "to": s.To}
	if s.fields.Has("metrics") {
		value["total"], value["success"], value["failed"] = s.Total, s.Success, s.Failed
		value["unique_users"], value["unique_ips"] = s.UniqueUsers, s.UniqueIPs
		value["average_ms"], value["p95_ms"], value["success_rate"] = s.AverageMS, s.P95MS, s.SuccessRate
	}
	if s.fields.Has("trend") {
		value["trend"], value["bucket_ms"] = s.Trend, s.BucketMS
	}
	if s.fields.Has("heatmap") {
		value["heatmap"], value["heatmap_granularity"] = s.Heatmap, s.HeatmapGranularity
	}
	if s.fields.Has("modules") {
		value["modules"] = s.Modules
	}
	if s.fields.Has("accounts") {
		value["accounts"] = s.Accounts
	}
	if s.fields.Has("sources") {
		value["sources"] = s.Sources
	}
	if s.fields.Has("repeated_failures") {
		value["repeated_failures"] = s.RepeatedFailures
	}
	return json.Marshal(value)
}
