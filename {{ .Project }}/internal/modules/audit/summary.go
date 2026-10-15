package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/report"

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
	From               int64       `json:"from"`
	To                 int64       `json:"to"`
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
	f, err := normalize(f)
	if err != nil {
		return nil, err
	}
	clause, args := where(f)
	result := &Summary{fields: selection, Accounts: []Breakdown{}, Sources: []Breakdown{}, From: f.From, To: f.To, Trend: []Bucket{}, Heatmap: []HeatCell{}, Modules: []Breakdown{}, RepeatedFailures: []Breakdown{}}
	if selection.Has("metrics") {
		err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE status >= 200 AND status < 400),COUNT(DISTINCT NULLIF(username,'')),COUNT(DISTINCT NULLIF(ip,'')),COALESCE(AVG(duration_ms),0),COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),0) FROM t_audit_event`+clause, args...).Scan(&result.Total, &result.Success, &result.UniqueUsers, &result.UniqueIPs, &result.AverageMS, &result.P95MS)
		if err != nil {
			return nil, err
		}
		result.Failed = result.Total - result.Success
		if result.Total > 0 {
			result.SuccessRate = float64(result.Success) * 100 / float64(result.Total)
		}
	}
	if selection.Has("trend") {
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
	}
	if selection.Has("heatmap") {
		result.Heatmap, result.HeatmapGranularity = heatCells(f)
		thresholds := make([]int64, len(result.Heatmap))
		for i, cell := range result.Heatmap {
			thresholds[i] = cell.StartedAt
		}
		heatArgs := append(append([]any{}, args...), pq.Array(thresholds))
		rows, err := m.store.SQL(ctx).QueryContext(ctx, fmt.Sprintf("SELECT width_bucket(occurred_at,$%d::bigint[]),COUNT(*),COUNT(*) FILTER (WHERE status < 200 OR status >= 400) FROM t_audit_event", len(heatArgs))+clause+" GROUP BY 1", heatArgs...)
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
