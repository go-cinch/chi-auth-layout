package tests

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ else }}
	"github.com/go-chi/chi/v5"
{{ end }}
)

func TestAuditAggregationsAndIsolation(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	ctx := t.Context()
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).UnixMilli()
	for i := 0; i < 8; i++ {
		status := 200
		if i < 5 {
			status = 401
		}
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: "audit_test", Module: "auth", Method: "POST", Path: "/pub/login", Status: status, DurationMS: int64(i * 10), IP: "127.0.0.1", OccurredAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "operation", Username: "audit_test", Module: "user", Method: "PATCH", Path: "/user/1", Status: 200, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
	f := audit.Filter{Kind: "login", From: auditTime(at - 1), To: auditTime(at + 1000), Timezone: "Asia/Shanghai"}
	s, err := m.Summary(ctx, f)
	if err != nil || s.Total != 8 || s.Success != 3 || s.SuccessRate != 37.5 || s.Heatmap[0].Count != 8 || len(s.RepeatedFailures) != 1 {
		t.Fatal(s, err)
	}
	f.Result = "failure"
	p, err := m.List(ctx, f)
	if err != nil || p.Total != 5 || len(p.Items) != 1 {
		t.Fatal(p, err)
	}
	if _, err := m.Get(ctx, "operation", p.Items[0].ID); !errors.Is(err, audit.ErrNotFound) {
		t.Fatal("cross-kind detail", err)
	}
	options, err := m.Options(ctx, f)
	if err != nil || len(options) != 1 {
		t.Fatal(options, err)
	}
	events, err := m.Export(ctx, f)
	if err != nil || len(events) != 5 {
		t.Fatal(events, err)
	}
	f.Query = "%"
	p, err = m.List(ctx, f)
	if err != nil || p.Total != 0 {
		t.Fatal("wildcard escaped", p, err)
	}
}

func TestHistoricalLogoutBelongsToLoginLog(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	ctx := t.Context()
	at := time.Now().UnixMilli()
	e := auditctx.Event{OccurredAt: at, Kind: "operation", Module: "auth", Method: "POST", Path: "/custom/pub/logout", Operation: "logout", Username: "historical_logout", Status: 200}
	if err := m.RecordHTTP(ctx, e); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"login", "operation"} {
		filter := audit.Filter{Kind: kind, Username: e.Username, From: auditTime(at), To: auditTime(at + 1)}
		events, err := m.Export(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "operation" {
			if len(events) != 0 {
				t.Fatal("logout leaked into operation export", events)
			}
			continue
		}
		if len(events) != 1 || events[0].Kind != "login" || events[0].Operation != "logout" {
			t.Fatal("missing historical logout", events)
		}
		id := events[0].ID
		if detail, err := m.Get(ctx, "login", id); err != nil || detail.Kind != "login" || detail.Path != e.Path {
			t.Fatal("login detail", detail, err)
		}
		if _, err := m.Get(ctx, "operation", id); !errors.Is(err, audit.ErrNotFound) {
			t.Fatal("operation detail", err)
		}
	}
}

func TestAuditSelectedCellAndSort(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	ctx := t.Context()
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).UnixMilli()
	for i := 0; i < 4; i++ {
		name := "alice"
		if i == 3 {
			name = "alice-long"
		}
		status := 200
		if i == 1 {
			status = 401
		}
		offset := int64(0)
		if i == 2 {
			offset = 3600000
		}
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: name, Module: "auth", Method: "POST", Path: "/pub/login", Status: status, DurationMS: int64(10 + i), IP: "127.0.0.1", OccurredAt: at + offset}); err != nil {
			t.Fatal(err)
		}
	}
	day, hour, size := int32(0), int32(8), int32(10)
	f := audit.Filter{Kind: "login", From: auditTime(at - 1), To: auditTime(at + 7200000), Timezone: "Asia/Shanghai", Weekday: &day, Hour: &hour, Username: "alice", Size: &size, Sort: "failure"}
	p, err := m.List(ctx, f)
	if err != nil || p.Total != 2 || p.Items[0].Status != 401 {
		t.Fatal(p, err)
	}
	s, err := m.Summary(ctx, f)
	if err != nil || s.Total != 2 || s.Heatmap[0].Failed != 1 || len(s.Accounts) != 1 || len(s.Sources) != 1 || s.BucketMS != 3600000 {
		t.Fatal(s, err)
	}
	f.Sort = "duration"
	events, err := m.Export(ctx, f)
	if err != nil || len(events) != 2 || events[0].DurationMS != 11 {
		t.Fatal(events, err)
	}
	f.Timezone = "UTC"
	p, err = m.List(ctx, f)
	if err != nil || p.Total != 0 {
		t.Fatal("timezone selection mismatch", p, err)
	}
	// The same wall-clock heat cell includes both repeated hours across a DST fold.
	fold := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).UnixMilli()
	for _, offset := range []int64{0, 3600000} {
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: "dst", Module: "auth", Method: "POST", Path: "/pub/login", Status: 200, OccurredAt: fold + offset}); err != nil {
			t.Fatal(err)
		}
	}
	day = 6
	hour = 1
	f = audit.Filter{Kind: "login", From: auditTime(fold - 3600000), To: auditTime(fold + 7200000), Timezone: "America/New_York", Weekday: &day, Hour: &hour, Size: &size}
	p, err = m.List(ctx, f)
	if err != nil || p.Total != 2 {
		t.Fatal("DST fold selection", p, err)
	}
}

func TestAuditReadExportPermissions(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	if _, err := store.DB.ExecContext(t.Context(), `UPDATE t_action SET config=JSONB_SET(config,'{bindings,0,feature_keys}',(config #> '{bindings,0,feature_keys}') - 'admin:audit:login:export-report') WHERE word='admin:read:audit:login'`); err != nil {
		t.Fatal(err)
	}

	limits, _ := pagination.New(100, 100)
	_, err := store.DB.ExecContext(t.Context(), `UPDATE t_user SET role_id=NULL, action=(SELECT code FROM t_action WHERE word='admin:read:audit:login'),login_count=1 WHERE id=2`)
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}}
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	memorySessions := auth.NewMemorySessionStore()
	t.Cleanup(func() { _ = memorySessions.Close() })
	sessions, _ := auth.NewSessions(memorySessions, auth.SessionConfig{
		Issuer:           "test",
		Key:              "session-test-signing-key-0123456789abcdef",
		SessionLifetime:  time.Hour,
		RememberLifetime: 24 * time.Hour,
		RetryWindow:      10 * time.Second,
	})
	manager, err := authn.New("test", strings.Repeat("x", 32), time.Hour, auth.NewIdentityValidator(store, sessions))
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Issue(t.Context(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	handler, err := server.NewRouter(cfg, manager, nil, auth.New(store, manager, nil, sessions, nil, nil, nil, localizer, nil, &auth.Verification{}), audit.New(store, limits, config.DefaultAudit()))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/audit/login", 200}, {"/audit/login/summary", 200}, {"/audit/login/option", 200}, {"/audit/login/detail/999999", 404}, {"/audit/login/export", 403}, {"/audit/operation", 403}, {"/audit/operation/summary", 403},
	} {
		req := httptest.NewRequest("GET", test.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != test.status {
			t.Fatalf("%s = %d %s", test.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/audit/login", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestAuditHeatmapCalendarBuckets(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	location, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 12, 14, 30, 0, 0, location)
	for _, at := range []time.Time{now.Add(-time.Minute), now.AddDate(0, 0, -7), now.AddDate(0, 0, -14)} {
		if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: "login", Username: "calendar", Status: 200, OccurredAt: at.UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, location)
	f := audit.Filter{Kind: "login", From: auditTime(from.UnixMilli()), To: auditTime(now.UnixMilli()), Timezone: location.String()}
	s, err := m.Summary(t.Context(), f)
	if err != nil || len(s.Heatmap) != 30 || s.HeatmapGranularity != "day" {
		t.Fatal(s, err)
	}
	populated := 0
	for _, cell := range s.Heatmap {
		if cell.Count == 0 {
			continue
		}
		populated++
		selected := f
		selected.From = auditTime(cell.StartedAt)
		selected.To = auditTime(cell.EndedAt)
		page, err := m.List(t.Context(), selected)
		if err != nil || cell.Count != 1 || page.Total != cell.Count {
			t.Fatal(cell, page, err)
		}
	}
	if populated != 3 {
		t.Fatal("same weekday from different dates was merged", populated)
	}
	f.From = auditTime(now.Add(-24 * time.Hour).UnixMilli())
	s, err = m.Summary(t.Context(), f)
	if err != nil || len(s.Heatmap) != 24 || s.Heatmap[23].Count != 1 {
		t.Fatal(s, err)
	}
}

func TestAuditEpochStorageAndActorConstraint(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	conn, err := store.DB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, zone := range []string{"Asia/Shanghai", "America/New_York"} {
		if _, err := conn.ExecContext(t.Context(), `SELECT set_config('TimeZone',$1,false)`, zone); err != nil {
			t.Fatal(err)
		}
		before := time.Now().UnixMilli()
		var created, occurred, databaseNow int64
		if err := conn.QueryRowContext(t.Context(), `INSERT INTO t_audit_event(occurred_at,kind,module,method,path,status,duration_ms) VALUES ($1,'operation','user','PATCH','/user/1',200,0) RETURNING created_at,occurred_at,FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint`, before).Scan(&created, &occurred, &databaseNow); err != nil {
			t.Fatal(err)
		}
		if occurred != before || created != databaseNow {
			t.Fatalf("%s: timestamps depend on zone: %d %d", zone, created, occurred)
		}
	}
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO t_audit_event(occurred_at,kind,user_id,module,method,path,status,duration_ms) VALUES (1,'login',-1,'auth','POST','/pub/login',401,0)`); err == nil {
		t.Fatal("negative actor accepted")
	}
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	day, hour := int32(0), int32(8)
	if _, err := m.List(t.Context(), audit.Filter{Kind: "operation", Timezone: "Local", Weekday: &day, Hour: &hour}); !errors.Is(err, audit.ErrFilter) {
		t.Fatal("Local must return a filter error", err)
	}
}

func TestAuditPayloadPersistenceAndHistory(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	at := time.Now().UnixMilli()
	request := json.RawMessage(`{"query":null,"body":{"name":"safe","password":"[redacted]"}}`)
	response := json.RawMessage(`{"id":25,"name":"safe"}`)
	if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: "operation", Module: "user", UserID: 2, Username: "guest", Operation: "create", ResourceType: "user", Method: "POST", Path: "/user", Status: 200, OccurredAt: at, RequestParams: request, ResponseParams: response}); err != nil {
		t.Fatal(err)
	}
	page, err := m.List(t.Context(), audit.Filter{Kind: "operation", From: auditTime(at), To: auditTime(at + 1000)})
	if err != nil || page.Total != 1 {
		t.Fatal(page, err)
	}
	item := page.Items[0]
	if item.ResourceID != "25" || string(item.RequestParams) != "null" || string(item.ResponseParams) != "null" {
		t.Fatal(item)
	}
	detail, err := m.Get(t.Context(), "operation", item.ID)
	if err != nil || !strings.Contains(string(detail.RequestParams), "[redacted]") || !strings.Contains(string(detail.ResponseParams), "safe") {
		t.Fatal(detail, err)
	}
	if _, err := store.DB.ExecContext(t.Context(), `DELETE FROM t_user WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	detail, err = m.Get(t.Context(), "operation", item.ID)
	if err != nil || detail.UserID != 2 || detail.Username != "guest" {
		t.Fatal("audit history must survive actor deletion", detail, err)
	}
}

func TestAuditHeaderPersistenceAndHistory(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	at := time.Now().UnixMilli()
	header := json.RawMessage(`{"Authorization":"[redacted]","X-Verified-Client-Ip":"203.0.113.5","X-Real-Ip":"203.0.113.6","X-Forwarded-For":["192.0.2.99","203.0.113.5"]}`)
	for _, kind := range []string{"login", "operation"} {
		if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: kind, Module: "auth", Method: "POST", Path: "/pub/login", Status: 401, OccurredAt: at, Header: header}); err != nil {
			t.Fatal(err)
		}
		page, err := m.List(t.Context(), audit.Filter{Kind: kind, From: auditTime(at), To: auditTime(at + 1000)})
		if err != nil || page.Total != 1 || string(page.Items[0].Header) != "null" {
			t.Fatalf("list must not load headers: %+v, %v", page, err)
		}
		detail, err := m.Get(t.Context(), kind, page.Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(detail.Header, &got); err != nil {
			t.Fatal(err)
		}
		if got["Authorization"] != "[redacted]" || got["X-Verified-Client-Ip"] != "203.0.113.5" || len(got["X-Forwarded-For"].([]any)) != 2 {
			t.Fatalf("header snapshot lost: %s", detail.Header)
		}
		encoded, err := json.Marshal(detail)
		if err != nil || !strings.Contains(string(encoded), `"header":{`) {
			t.Fatalf("detail header contract: %s, %v", encoded, err)
		}
	}
	var legacyID int64
	if err := store.DB.QueryRowContext(t.Context(), `INSERT INTO t_audit_event(occurred_at,kind,module,method,path,status,duration_ms) VALUES ($1,'login','auth','POST','/pub/login',200,0) RETURNING id`, at).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	legacy, err := m.Get(t.Context(), "login", legacyID)
	if err != nil || string(legacy.Header) != "null" {
		t.Fatalf("legacy header default: %+v, %v", legacy, err)
	}
}

func auditTime(value int64) *int64 { return &value }

func TestAuditOneSidedBoundsAndSelectedPayloadsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	ctx := t.Context()
	for _, at := range []int64{1000, 2000, 3000} {
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: "alice", Module: "auth", Method: "POST", Path: "/pub/login", Status: 200, OccurredAt: at, Header: json.RawMessage(`{"user_agent":"test"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		from, to *int64
		want     int64
	}{
		{auditTime(1500), nil, 2},
		{nil, auditTime(2500), 2},
		{auditTime(1500), auditTime(3000), 1},
	} {
		f := audit.Filter{Kind: "login", From: c.from, To: c.to, Query: "   "}
		page, err := m.List(ctx, f)
		if err != nil || page.Total != c.want {
			t.Fatal(page, err)
		}
		summary, err := m.Summary(ctx, f, "metrics", "trend", "heatmap")
		if err != nil || summary.Total != c.want || (summary.From == nil) != (c.from == nil) || (summary.To == nil) != (c.to == nil) {
			t.Fatal(summary, err)
		}
		detail, err := m.GetSelected(ctx, "login", page.Items[0].ID, []string{"id", "header"})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(detail)
		var fields map[string]any
		if json.Unmarshal(encoded, &fields) != nil || len(fields) != 2 || fields["header"] == nil {
			t.Fatal(string(encoded))
		}
	}
}

func TestLoginRankingTopTenAndTieBreakers(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	at := time.Now().UnixMilli()
	for _, account := range []struct {
		name          string
		total, failed int
	}{
		{"last", 1, 0}, {"echo", 1, 1}, {"charlie", 1, 1}, {"delta", 1, 1},
		{"foxtrot", 1, 1}, {"golf", 1, 1}, {"hotel", 1, 1}, {"first", 3, 3},
		{"bravo", 3, 2}, {"alpha", 3, 2}, {"second", 5, 2},
	} {
		for i := 0; i < account.total; i++ {
			status := 200
			if i < account.failed {
				status = 401
			}
			event := auditctx.Event{Kind: "login", Username: account.name, Module: "auth", Method: "POST", Path: "/pub/login", Status: status, OccurredAt: at}
			if err := m.RecordHTTP(t.Context(), event); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := m.Summary(t.Context(), audit.Filter{Kind: "login", From: auditTime(at), To: auditTime(at + 1)}, "accounts")
	if err != nil {
		t.Fatal(err)
	}
	want := []audit.Breakdown{
		{Name: "first", Total: 3, Failed: 3},
		{Name: "second", Total: 5, Failed: 2},
		{Name: "alpha", Total: 3, Failed: 2},
		{Name: "bravo", Total: 3, Failed: 2},
		{Name: "charlie", Total: 1, Failed: 1},
		{Name: "delta", Total: 1, Failed: 1},
		{Name: "echo", Total: 1, Failed: 1},
		{Name: "foxtrot", Total: 1, Failed: 1},
		{Name: "golf", Total: 1, Failed: 1},
		{Name: "hotel", Total: 1, Failed: 1},
	}
	if !slices.Equal(result.Accounts, want) {
		t.Fatalf("ranked accounts = %+v, want %+v", result.Accounts, want)
	}
}

func TestLoginHistorySurvivesActorDeletion(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits, config.DefaultAudit())
	at := time.Now().UnixMilli()
	for _, status := range []int{200, 401} {
		event := auditctx.Event{Kind: "login", Module: "auth", UserID: 2, Username: "guest", Method: "POST", Path: "/pub/login", Status: status, OccurredAt: at}
		if err := m.RecordHTTP(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	size := int32(10)
	filter := audit.Filter{Kind: "login", Username: "guest", From: auditTime(at), To: auditTime(at + 1), Size: &size}
	before, err := m.Export(t.Context(), filter)
	if err != nil || len(before) != 2 {
		t.Fatal(before, err)
	}
	var handler http.Handler
{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group("/audit"))
	handler = router
{{ else }}
	router := chi.NewRouter()
	router.Mount("/audit", m.HTTP())
	handler = router
{{ end }}
	csvSnapshot := func() [][]string {
		t.Helper()
		response := httptest.NewRecorder()
		target := fmt.Sprintf("/audit/login/export?username=guest&from=%d&to=%d", at, at+1)
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		rows, err := csv.NewReader(response.Body).ReadAll()
		if response.Code != http.StatusOK || err != nil || len(rows) != 3 {
			t.Fatalf("CSV status=%d rows=%v err=%v", response.Code, rows, err)
		}
		return rows
	}
	originalCSV := csvSnapshot()
	if _, err := store.DB.ExecContext(t.Context(), "DELETE FROM t_user WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	for i, row := range csvSnapshot() {
		if !slices.Equal(row, originalCSV[i]) {
			t.Fatalf("CSV row %d changed after deleting the actor: %v", i, row)
		}
	}
	list, err := m.List(t.Context(), filter)
	if err != nil || list.Total != 2 {
		t.Fatal(list, err)
	}
	exported, err := m.Export(t.Context(), filter)
	if err != nil || len(exported) != len(before) {
		t.Fatal(exported, err)
	}
	for i, record := range before {
		detail, err := m.Get(t.Context(), "login", record.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []audit.Event{list.Items[i], exported[i], *detail} {
			if current.ID != record.ID || current.Username != "guest" || current.UserID != 2 || current.Status != record.Status || current.OccurredAt != at {
				t.Fatalf("history was changed after deleting the actor: %+v", current)
			}
		}
	}
}

type auditTestRegistrationCredentials struct{}

func (auditTestRegistrationCredentials) OpenRegistrationPassword(context.Context, auth.EncryptedRegisterInput) (string, error) {
	return "test-password", nil
}

func TestAuditCreatedResourceIDsWithOmittedResponse(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	settings := config.DefaultAudit()
	settings.PayloadLimit = 64
	recorder := audit.New(store, limits, settings)
	manager, err := authn.New("audit-create", strings.Repeat("k", 64), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, localizer, nil, &auth.Verification{})
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	actions := action.New(store, limits, localizer)
	actions.SetDefinitionCatalog(dictionaries)
	actions.SetPermissionAdministration(policy)
	roles := role.New(store, limits, actions)
	users := user.New(store, limits, auditTestRegistrationCredentials{}, nil, actions, roles)
	cfg := &config.Config{Audit: settings}
	cfg.Idempotency.TTL = time.Minute
	router, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), recorder, users, roles, actions, usergroup.New(store, limits, actions, users), dictionaries, msg.New(store, limits))
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}}
	// Keep the actor distinct from the newly allocated IDs in every resource table.
	if err := store.DB.QueryRowContext(t.Context(), `INSERT INTO t_user (id, username, code, password, status, action)
		SELECT 9001, 'audit_creator', 'AUDITUSR', 'unused', 1, STRING_AGG(code, ',') FROM t_action
		WHERE word IN ('admin:create:system:user','admin:create:system:role','admin:create:system:action',
			'admin:create:system:user-group','admin:create:system:dictionary','admin:create:system:msg')
		RETURNING id, username, code, credential_version`).
		Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ module, body string }{
		{"user", `{"username":"audit_target","challenge_id":"fixture","credential":"fixture"}`},
		{"role", `{"name":"Audit target","word":"audit_target"}`},
		{"action", `{"word":"admin:read:custom:audit","group":"admin:system:user","config":{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":[]}],"ui":[],"endpoints":[]}}`},
		{"user-group", `{"name":"Audit target","word":"audit_target"}`},
		{"dictionary", `{"name":"Audit target","key":"AUDIT_TARGET","value":{"enabled":true}}`},
		{"msg", `{"title":"Audit target","content":"Test notice","type":"notice","scope":"all"}`},
	} {
		t.Run(tc.module, func(t *testing.T) {
			path := "/" + tc.module
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("POST %s returned %d: %s", path, res.Code, res.Body.String())
			}
			if res.Body.Len() <= settings.PayloadLimit {
				t.Fatalf("create response is only %d bytes, limit %d", res.Body.Len(), settings.PayloadLimit)
			}
			var created struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil || created.ID <= 0 {
				t.Fatalf("invalid created ID %d, err=%v", created.ID, err)
			}
			list, err := recorder.List(t.Context(), audit.Filter{Kind: "operation", Path: path, From: auditTime(0)})
			if err != nil || list.Total != 1 || len(list.Items) != 1 {
				t.Fatalf("expected one operation for %s: %+v, err=%v", path, list, err)
			}
			event, err := recorder.Get(t.Context(), "operation", list.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if string(event.ResponseParams) != `"[omitted: size limit]"` {
				t.Fatal("oversized response was not marked as omitted")
			}
			if event.Operation != "create" || event.ResourceType != tc.module || event.ResourceID != fmt.Sprint(created.ID) || event.UserID != identity.UserID {
				t.Fatalf("operation=%q type=%q target=%q (want %d) actor=%d", event.Operation, event.ResourceType, event.ResourceID, created.ID, event.UserID)
			}
		})
	}
}
