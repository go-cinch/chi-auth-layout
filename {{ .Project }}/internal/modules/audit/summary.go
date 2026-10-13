package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type Bucket struct {
	At      int64 `json:"at"`
	Success int64 `json:"success"`
	Failure int64 `json:"failure"`
}
type HeatCell struct {
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
	HeatmapGranularity string      `json:"heatmap_granularity"`
	BucketMS           int64       `json:"bucket_ms"`
	Accounts           []Breakdown `json:"accounts"`
	Sources            []Breakdown `json:"sources"`
	Total              int64       `json:"total"`
	Success            int64       `json:"success"`
	Failed             int64       `json:"failed"`
	UniqueUsers        int64       `json:"unique_users"`
	UniqueIPs          int64       `json:"unique_ips"`
	AverageMS          float64     `json:"average_ms"`
	P95MS              float64     `json:"p95_ms"`
	SuccessRate        float64     `json:"success_rate"`
	From               int64       `json:"from"`
	To                 int64       `json:"to"`
	Trend              []Bucket    `json:"trend"`
	Heatmap            []HeatCell  `json:"heatmap"`
	Modules            []Breakdown `json:"modules"`
	RepeatedFailures   []Breakdown `json:"repeated_failures"`
}

func (m *Module) Summary(ctx context.Context, f Filter) (*Summary, error) {
	f, err := normalize(f)
	if err != nil {
		return nil, err
	}
	clause, args := where(f)
	result := &Summary{From: f.From, To: f.To, Trend: []Bucket{}, Heatmap: []HeatCell{}, Modules: []Breakdown{}, RepeatedFailures: []Breakdown{}}
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE status >= 200 AND status < 400),COUNT(DISTINCT NULLIF(username,'')),COUNT(DISTINCT NULLIF(ip,'')),COALESCE(AVG(duration_ms),0),COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),0) FROM t_audit_event`+clause, args...).Scan(&result.Total, &result.Success, &result.UniqueUsers, &result.UniqueIPs, &result.AverageMS, &result.P95MS)
	if err != nil {
		return nil, err
	}
	result.Failed = result.Total - result.Success
	if result.Total > 0 {
		result.SuccessRate = float64(result.Success) * 100 / float64(result.Total)
	}
	// UTC buckets are fixed durations. Client renders their instants in its selected timezone.
	seconds := int64(86400)
	if f.To-f.From <= int64(24*time.Hour/time.Millisecond) {
		seconds = 3600
	}
	bucketArgs := append(append([]any{}, args...), seconds)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, fmt.Sprintf(`SELECT (occurred_at / ($%d::bigint * 1000)) * $%d * 1000,COUNT(*) FILTER (WHERE status >= 200 AND status < 400),COUNT(*) FILTER (WHERE status < 200 OR status >= 400) FROM t_audit_event`, len(bucketArgs), len(bucketArgs))+clause+" GROUP BY 1 ORDER BY 1", bucketArgs...)
	if err != nil {
		return nil, err
	}
	buckets := map[int64]Bucket{}
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.At, &b.Success, &b.Failure); err != nil {
			rows.Close()
			return nil, err
		}
		buckets[b.At] = b
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	step := seconds * 1000
	result.BucketMS = step
	for at := (f.From / step) * step; at < f.To; at += step {
		b := buckets[at]
		b.At = at
		result.Trend = append(result.Trend, b)
	}
	result.Heatmap, result.HeatmapGranularity = heatCells(f)
	thresholds := make([]int64, len(result.Heatmap))
	for i, cell := range result.Heatmap {
		thresholds[i] = cell.StartedAt
	}
	heatArgs := append(append([]any{}, args...), pq.Array(thresholds))
	rows, err = m.store.SQL(ctx).QueryContext(ctx, fmt.Sprintf("SELECT width_bucket(occurred_at,$%d::bigint[]),COUNT(*),COUNT(*) FILTER (WHERE status < 200 OR status >= 400) FROM t_audit_event", len(heatArgs))+clause+" GROUP BY 1", heatArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var index int
		var count, failed int64
		if err := rows.Scan(&index, &count, &failed); err != nil {
			rows.Close()
			return nil, err
		}
		if index > 0 && index <= len(result.Heatmap) {
			result.Heatmap[index-1].Count = count
			result.Heatmap[index-1].Failed = failed
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result.Modules, err = m.breakdown(ctx, clause, args, "module", 0)
	if err != nil {
		return nil, err
	}
	result.RepeatedFailures, err = m.breakdown(ctx, clause, args, "username", 5)
	if err != nil {
		return nil, err
	}
	result.Accounts, err = m.breakdown(ctx, clause, args, "username", 0)
	if err != nil {
		return nil, err
	}
	result.Sources, err = m.breakdown(ctx, clause, args, "ip", 0)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (m *Module) breakdown(ctx context.Context, clause string, args []any, field string, minimum int64) ([]Breakdown, error) {
	if field != "module" && field != "username" && field != "ip" {
		return nil, ErrFilter
	}
	clause += " AND " + field + " <> ''"
	extra := ""
	if minimum > 0 {
		args = append(append([]any{}, args...), minimum)
		extra = fmt.Sprintf(" HAVING COUNT(*) FILTER (WHERE status < 200 OR status >= 400) >= $%d", len(args))
	}
	ranking := "2 DESC,3 DESC,1"
	if field == "username" {
		ranking = "3 DESC,2 DESC,1"
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT "+field+",COUNT(*),COUNT(*) FILTER (WHERE status < 200 OR status >= 400) FROM t_audit_event"+clause+" GROUP BY "+field+extra+" ORDER BY "+ranking+" LIMIT 10", args...)
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
