package tests

import (
	"slices"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
)

func TestAuditDailyTrendAndDrilldown(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	recorder := audit.New(store, limits, config.DefaultAudit())
	epoch := func(timestamp string) int64 {
		t.Helper()
		at, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			t.Fatal(err)
		}
		return at.UnixMilli()
	}
	for _, kind := range []string{"login", "operation"} {
		t.Run(kind, func(t *testing.T) {
			for _, timestamp := range []string{
				"2026-11-20T23:59:59.999+08:00",
				"2026-11-21T00:00:00+08:00",
				"2026-11-21T02:00:00+08:00",
				"2026-11-21T23:59:59.999+08:00",
				"2026-11-22T00:00:00+08:00",
			} {
				event := auditctx.Event{Kind: kind, Username: "daily-trend", Status: 200, OccurredAt: epoch(timestamp)}
				if err := recorder.RecordHTTP(t.Context(), event); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				zone, from, to string
				want           []string // newest first, as displayed by the list
			}{
				{"Asia/Shanghai", "2026-11-21T00:00:00+08:00", "2026-11-22T00:00:00+08:00", []string{
					"2026-11-21T23:59:59.999+08:00", "2026-11-21T02:00:00+08:00", "2026-11-21T00:00:00+08:00",
				}},
				{"UTC", "2026-11-21T00:00:00Z", "2026-11-22T00:00:00Z", []string{
					"2026-11-22T00:00:00+08:00", "2026-11-21T23:59:59.999+08:00",
				}},
			} {
				t.Run(tc.zone, func(t *testing.T) {
					f := audit.Filter{Kind: kind, Username: "daily-trend", Timezone: tc.zone,
						From: auditTime(epoch("2026-11-20T12:00:00+08:00")), To: auditTime(epoch("2026-11-22T12:00:00+08:00"))}
					s, err := recorder.Summary(t.Context(), f, "trend")
					if err != nil {
						t.Fatal(err)
					}
					i := slices.IndexFunc(s.Trend, func(b audit.Bucket) bool { return b.At == epoch(tc.from) })
					if i < 0 {
						t.Fatalf("no Nov 21 bucket starting at %s: %+v", tc.from, s.Trend)
					}
					b := s.Trend[i]
					if b.EndedAt != epoch(tc.to) || b.Success != int64(len(tc.want)) {
						t.Fatalf("Nov 21 bucket = %+v, want end %s and %d events", b, tc.to, len(tc.want))
					}
					f.From, f.To = &b.At, &b.EndedAt
					f.Size = new(int32(10))
					page, err := recorder.List(t.Context(), f)
					if err != nil {
						t.Fatal(err)
					}
					if page.Total != b.Success || len(page.Items) != len(tc.want) {
						t.Fatalf("drilldown returned %d rows, total %d", len(page.Items), page.Total)
					}
					for i, ts := range tc.want {
						if page.Items[i].OccurredAt != epoch(ts) {
							t.Errorf("drilldown row %d = %d, want %s", i, page.Items[i].OccurredAt, ts)
						}
					}
				})
			}
		})
	}
}
