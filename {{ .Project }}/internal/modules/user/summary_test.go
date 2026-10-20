package user

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/report"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

const zeroCreationCounts = `[0,0,0,0,0,0,0,0,0,0,0,0,0,0]`

func TestCreationBuckets(t *testing.T) {
	now := time.Date(2026, 10, 13, 3, 12, 30, 123000000, time.UTC)
	points, err := creationBuckets(SummaryInput{}, now)
	if err != nil || len(points) != 14 || points[0].Date != "2026-09-30" || points[13].To != time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("default buckets: %#v %v", points, err)
	}
	for _, tc := range []struct {
		zone  string
		at    time.Time
		hours int64
		date  string
	}{
		{"America/New_York", time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC), 23, "2026-03-08"},
		{"America/New_York", time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), 25, "2026-11-01"},
		{"Asia/Shanghai", now, 24, "2026-10-12"},
		{"America/Havana", time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC), 23, "2026-03-08"},
		{"America/Santiago", time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), 23, "2026-09-06"},
	} {
		points, err = creationBuckets(SummaryInput{Days: 30, TimeZone: tc.zone}, tc.at)
		if err != nil || len(points) != 30 {
			t.Fatalf("%s: %v", tc.zone, err)
		}
		found := false
		for i, p := range points {
			if i > 0 && points[i-1].To != p.From {
				t.Fatal("calendar buckets are not contiguous")
			}
			if p.To <= p.From {
				t.Fatalf("invalid bucket: %#v", p)
			}
			if p.Date == tc.date {
				found = true
				if p.To-p.From != tc.hours*int64(time.Hour/time.Millisecond) {
					t.Fatalf("DST duration: %#v", p)
				}
			}
		}
		if !found {
			t.Fatal("missing expected calendar date")
		}
		if last := points[len(points)-1]; last.From > tc.at.UnixMilli() || last.To <= tc.at.UnixMilli() {
			t.Fatalf("today must expose full containing day: %#v", last)
		}
	}
	for _, input := range []SummaryInput{ {Days: 1}, {Days: -1}, {Days: 31}, {TimeZone: "Local"}, {TimeZone: "Not/A_Zone"}} {
		if _, err := creationBuckets(input, now); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %#v: %v", input, err)
		}
	}
}

type captureSummaryInstant struct{ target *int64 }

func (a captureSummaryInstant) Match(value driver.Value) bool {
	instant, ok := value.(int64)
	if ok {
		*a.target = instant
	}
	return ok
}

type summaryCutoffMatchesInstant struct{ instant *int64 }

func (a summaryCutoffMatchesInstant) Match(value driver.Value) bool {
	cutoff, ok := value.(int64)
	return ok && cutoff == *a.instant+1
}

func TestUserSummary(t *testing.T) {
	m, mock := newTestModule(t)
	row := func(trend string) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"total", "pending", "active", "locked", "recent_logged_in_users", "trend", "login_trend"}).AddRow(4, 1, 2, 1, 3, trend, `[1,0,0,0,0,0,0,0,0,0,0,0,0,2]`)
	}
	var capturedInstant int64
	mock.ExpectQuery("WITH effective AS").WithArgs(captureSummaryInstant{&capturedInstant}, sqlmock.AnyArg(), summaryCutoffMatchesInstant{&capturedInstant}).WillReturnRows(row(`[0,0,0,0,0,0,0,0,0,0,0,0,0,4]`))
	value, err := m.Summary(t.Context(), SummaryInput{TimeZone: "Asia/Shanghai"})
	if err != nil || value.Total != 4 || value.Counts["active"] != 2 || value.Counts["recent_logged_in_users"] != 3 || len(value.Trend) != 14 || value.Trend[13].Count != 4 || value.Trend[0].RecentLoggedInUsers != 1 || value.Trend[13].RecentLoggedInUsers != 2 || value.Groups == nil {
		t.Fatalf("%#v %v", value, err)
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	localNow := time.UnixMilli(capturedInstant).In(location)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, location).UnixMilli()
	if value.Trend[len(value.Trend)-1].To != end {
		t.Fatalf("public day end is clipped to request time: %#v", value.Trend)
	}
	if _, err := m.Summary(t.Context(), SummaryInput{Days: 2}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("WITH effective AS").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Summary(t.Context(), SummaryInput{}); !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatal(err)
	}
	for _, broken := range []string{`invalid`, `[]`} {
		mock.ExpectQuery("WITH effective AS").WillReturnRows(row(broken))
		if _, err := m.Summary(t.Context(), SummaryInput{}); err == nil {
			t.Fatalf("accepted %s", broken)
		}
	}
}

func TestUserSummaryHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(target string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != want {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, q := range []string{"f=", "f=unknown", "f=total&f=counts", "f=total,total", "days=0", "days=15", "days=a", "days=14&days=30", "time_zone=", "time_zone=Local", "time_zone=Not/A_Zone", "x=y", "days=%zz"} {
		request("/summary?"+q, 400)
	}
	mock.ExpectQuery("WITH effective AS").WillReturnRows(sqlmock.NewRows([]string{"total", "pending", "active", "locked", "recent_logged_in_users", "trend", "login_trend"}).AddRow(0, 0, 0, 0, 0, zeroCreationCounts, zeroCreationCounts))
	if body := request("/summary?days=14&time_zone=Asia%2FShanghai", 200); !strings.Contains(body, `"pending":0`) || strings.Count(body, `"recent_logged_in_users":0`) != 15 || !strings.Contains(body, `"groups":[]`) {
		t.Fatal(body)
	}
	mock.ExpectQuery("WITH effective AS").WillReturnError(sqlmock.ErrCancelled)
	request("/summary", 500)
	for _, q := range []string{"created_from=", "created_from=-1", "created_to=9007199254740992", "created_to=no", "created_from=1&created_from=2", "created_from=2&created_to=2"} {
		request("/?"+q, 400)
	}
	mock.ExpectQuery("SELECT COUNT.*created_at >=.*created_at <").WithArgs(int64(100), int64(200)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request("/?created_from=100&created_to=200", 200)
}

func TestUserCreationRange(t *testing.T) {
	m, mock := newTestModule(t)
	start, end := int64(100), int64(200)
	mock.ExpectQuery("SELECT COUNT.*created_at >=.*created_at <").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	if value, err := m.List(t.Context(), ListUsersInput{CreatedFrom: &start, CreatedTo: &end}); err != nil || value.Total != 0 {
		t.Fatalf("%#v %v", value, err)
	}
	for _, bounds := range [][2]int64{ {-1, 1}, {1, -1}, {2, 1}, {1, 1}, {9007199254740992, 9007199254740993}} {
		if _, err := m.List(t.Context(), ListUsersInput{CreatedFrom: &bounds[0], CreatedTo: &bounds[1]}); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
}

// Match the actual aggregate predicate, including the exclusive snapshot end.
// This also ensures recent users are counted once per row, never by login_count.
func TestRecentLoggedInUsersSummarySQL(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery(`COUNT\(\*\) FILTER \(WHERE last_logged_in_at > 0 AND last_logged_in_at >= \(\$2::bigint\[\]\)\[1\] AND last_logged_in_at < \$3\)`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"total", "pending", "active", "locked", "recent_logged_in_users", "trend", "login_trend"}).AddRow(12, 0, 12, 0, 5, `[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]`, `[1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,4]`))
	value, err := m.Summary(t.Context(), SummaryInput{Days: 30, TimeZone: "America/New_York"})
	if err != nil || value.Counts["recent_logged_in_users"] != 5 || len(value.Trend) != 30 || value.Trend[0].RecentLoggedInUsers != 1 || value.Trend[29].RecentLoggedInUsers != 4 {
		t.Fatalf("recent logged-in users: %#v %v", value, err)
	}
}

func TestRecentLoggedInUsersSummaryHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	mock.ExpectQuery("WITH effective AS.*last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"total", "pending", "active", "locked", "recent_logged_in_users", "trend", "login_trend"}).AddRow(8, 1, 6, 1, 3, zeroCreationCounts, `[1,0,0,0,0,0,0,0,0,0,0,0,0,2]`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/summary?days=14&time_zone=Asia%2FShanghai", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"recent_logged_in_users":3`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestUserSummaryRejectsInvalidLoginBuckets(t *testing.T) {
	m, mock := newTestModule(t)
	for _, broken := range []string{`invalid`, `[]`} {
		mock.ExpectQuery("WITH effective AS").WillReturnRows(sqlmock.NewRows([]string{"total", "pending", "active", "locked", "recent_logged_in_users", "trend", "login_trend"}).AddRow(0, 0, 0, 0, 0, zeroCreationCounts, broken))
		if _, err := m.Summary(t.Context(), SummaryInput{}); err == nil {
			t.Fatalf("accepted latest-login buckets %s", broken)
		}
	}
}

func TestSelectedUserSummary(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("^" + regexp.QuoteMeta("SELECT COUNT(*) FROM t_user") + "$").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	value, err := m.Summary(t.Context(), SummaryInput{Fields: report.Fields{"total"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) != `{"total":0}` {
		t.Fatal(string(data), err)
	}
	if _, err := m.Summary(t.Context(), SummaryInput{Fields: report.Fields{"unknown"}}); err == nil {
		t.Fatal("accepted unknown field")
	}
	mock.ExpectQuery(`WITH effective AS \(SELECT .* FROM t_user\) SELECT COUNT\(\*\) FILTER`).WillReturnRows(sqlmock.NewRows([]string{"pending", "active", "locked", "recent"}).AddRow(0, 2, 0, 1))
	value, err = m.Summary(t.Context(), SummaryInput{Fields: report.Fields{"counts"}})
	if err != nil || value.Counts["active"] != 2 {
		t.Fatal(value, err)
	}
}
