package tests

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestAuditAggregationsAndIsolation(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
	ctx := t.Context()
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).UnixMilli()
	for i := 0; i < 8; i++ {
		status := 200
		if i < 5 {
			status = 401
		}
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: "audit_test", Module: "auth", Method: "POST", Path: "/auth/pub/login", Status: status, DurationMS: int64(i * 10), IP: "127.0.0.1", OccurredAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "operation", Username: "audit_test", Module: "user", Method: "PATCH", Path: "/user/1", Status: 200, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
	f := audit.Filter{Kind: "login", From: at - 1, To: at + 1000, Timezone: "Asia/Shanghai"}
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

func TestAuditSelectedCellAndSort(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
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
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: name, Module: "auth", Method: "POST", Path: "/auth/pub/login", Status: status, DurationMS: int64(10 + i), IP: "127.0.0.1", OccurredAt: at + offset}); err != nil {
			t.Fatal(err)
		}
	}
	day, hour, size := int32(0), int32(8), int32(10)
	f := audit.Filter{Kind: "login", From: at - 1, To: at + 7200000, Timezone: "Asia/Shanghai", Weekday: &day, Hour: &hour, Username: "alice", Size: &size, Sort: "failure"}
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
		if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: "dst", Module: "auth", Method: "POST", Path: "/auth/pub/login", Status: 200, OccurredAt: fold + offset}); err != nil {
			t.Fatal(err)
		}
	}
	day = 6
	hour = 1
	f = audit.Filter{Kind: "login", From: fold - 3600000, To: fold + 7200000, Timezone: "America/New_York", Weekday: &day, Hour: &hour, Size: &size}
	p, err = m.List(ctx, f)
	if err != nil || p.Total != 2 {
		t.Fatal("DST fold selection", p, err)
	}
}

func TestAuditReadExportPermissions(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	_, err := store.DB.ExecContext(t.Context(), `UPDATE t_user SET role_id=NULL, action=(SELECT code FROM t_action WHERE word='audit.login.read'),login_count=1 WHERE id=2`)
	if err != nil {
		t.Fatal(err)
	}
	var identity authn.Identity
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	sessions, _ := auth.NewSessions(auth.NewMemorySessionStore(), time.Hour, 24*time.Hour)
	manager, err := authn.New("test", strings.Repeat("x", 32), time.Hour, auth.NewIdentityValidator(store, sessions, auth.Switches{}))
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
	handler, err := server.NewRouter(cfg, manager, nil, auth.New(store, manager, nil, sessions, nil, nil, nil, auth.Switches{}), audit.New(store, limits))
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
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
	location, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 12, 14, 30, 0, 0, location)
	for _, at := range []time.Time{now.Add(-time.Minute), now.AddDate(0, 0, -7), now.AddDate(0, 0, -14)} {
		if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: "login", Username: "calendar", Status: 200, OccurredAt: at.UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, location)
	f := audit.Filter{Kind: "login", From: from.UnixMilli(), To: now.UnixMilli(), Timezone: location.String()}
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
		selected.From = cell.StartedAt
		selected.To = cell.EndedAt
		page, err := m.List(t.Context(), selected)
		if err != nil || cell.Count != 1 || page.Total != cell.Count {
			t.Fatal(cell, page, err)
		}
	}
	if populated != 3 {
		t.Fatal("same weekday from different dates was merged", populated)
	}
	f.From = now.Add(-24 * time.Hour).UnixMilli()
	s, err = m.Summary(t.Context(), f)
	if err != nil || len(s.Heatmap) != 24 || s.Heatmap[23].Count != 1 {
		t.Fatal(s, err)
	}
}

func TestAuditEpochStorageAndActorConstraint(t *testing.T) {
	store := permissionDatabase(t)
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
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO t_audit_event(occurred_at,kind,user_id,module,method,path,status,duration_ms) VALUES (1,'login',-1,'auth','POST','/auth/pub/login',401,0)`); err == nil {
		t.Fatal("negative actor accepted")
	}
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
	day, hour := int32(0), int32(8)
	if _, err := m.List(t.Context(), audit.Filter{Kind: "operation", Timezone: "Local", Weekday: &day, Hour: &hour}); !errors.Is(err, audit.ErrFilter) {
		t.Fatal("Local must return a filter error", err)
	}
}

func TestAuditPayloadPersistenceAndHistory(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
	at := time.Now().UnixMilli()
	request := json.RawMessage(`{"query":null,"body":{"name":"safe","password":"[redacted]"}}`)
	response := json.RawMessage(`{"id":25,"name":"safe"}`)
	if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: "operation", Module: "user", UserID: 2, Username: "guest", Operation: "create", ResourceType: "user", Method: "POST", Path: "/user", Status: 200, OccurredAt: at, RequestParams: request, ResponseParams: response}); err != nil {
		t.Fatal(err)
	}
	page, err := m.List(t.Context(), audit.Filter{Kind: "operation", From: at, To: at + 1000})
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
	limits, _ := pagination.New(100, 100)
	m := audit.New(store, limits)
	at := time.Now().UnixMilli()
	header := json.RawMessage(`{"Authorization":"[redacted]","X-Verified-Client-Ip":"203.0.113.5","X-Real-Ip":"203.0.113.6","X-Forwarded-For":["192.0.2.99","203.0.113.5"]}`)
	for _, kind := range []string{"login", "operation"} {
		if err := m.RecordHTTP(t.Context(), auditctx.Event{Kind: kind, Module: "auth", Method: "POST", Path: "/auth/pub/login", Status: 401, OccurredAt: at, Header: header}); err != nil {
			t.Fatal(err)
		}
		page, err := m.List(t.Context(), audit.Filter{Kind: kind, From: at, To: at + 1000})
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
	if err := store.DB.QueryRowContext(t.Context(), `INSERT INTO t_audit_event(occurred_at,kind,module,method,path,status,duration_ms) VALUES ($1,'login','auth','POST','/auth/pub/login',200,0) RETURNING id`, at).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	legacy, err := m.Get(t.Context(), "login", legacyID)
	if err != nil || string(legacy.Header) != "null" {
		t.Fatalf("legacy header default: %+v, %v", legacy, err)
	}
}
