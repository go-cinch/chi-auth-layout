package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/infra/rds"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestAuditStreamDelivery(t *testing.T) {
	store := migrationDatabase(t)
	client := sessionRedisClient(t)
	ctx := t.Context()
	settings := config.DefaultAudit()
	settings.Stream = "audit:integration"
	limits, _ := pagination.New(100, 100)
	module := audit.New(store, limits, settings)
	module.UseRedisStream(client)
	queue := rds.NewStream(client, settings.Stream, "audit-writers")

	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auditctx.Identity(r.Context(), 42, "audited-account")
		if r.URL.Path == "/pub/login" {
			auditctx.AlsoRecordUserOperation(r.Context(), "recover_totp")
		}
		w.WriteHeader(http.StatusOK)
	})
{{- if eq .Computed.http_router_final "gin" }}
	handler := gin.New()
	handler.Use(server.Audit(module, redact.New(), settings))
	handler.Any("/*path", func(c *gin.Context) { record.ServeHTTP(c.Writer, c.Request) })
{{- else }}
	handler := server.Audit(module, redact.New(), settings)(record)
{{- end }}
	for _, path := range []string{"/pub/login", "/role"} {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Authorization", "Bearer test-private-value")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatal(response.Code)
		}
	}
	var count int
	rows := func() int {
		t.Helper()
		if err := store.DB.QueryRowContext(ctx, "SELECT count(*) FROM t_audit_event").Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if rows() != 0 {
		t.Fatal("middleware wrote to SQL before consumption")
	}
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	// Simulate a stopped worker and duplicate publication with the same delivery ID.
	pending, err := queue.Read(ctx, "old-worker")
	if err != nil || len(pending) != 1 {
		t.Fatalf("read queued event: %v (%d messages)", err, len(pending))
	}
	first := pending[0]
	payload := fmt.Sprint(first.Values["event"])
	if strings.Contains(payload, "test-private-value") || !strings.Contains(payload, "[redacted]") {
		t.Fatal("stream must contain the sanitized HTTP snapshot")
	}
	if err := client.Eval(ctx, `return redis.call('XCLAIM',KEYS[1],ARGV[1],ARGV[2],0,ARGV[3],'IDLE',120000,'JUSTID')`,
		[]string{settings.Stream}, "audit-writers", "old-worker", first.ID).Err(); err != nil {
		t.Fatal(err)
	}
	if err := queue.Add(ctx, first.Values); err != nil {
		t.Fatal(err)
	}
	// A valid login plus an invalid operation tests rollback of the *first* insert.
	_, err = store.DB.ExecContext(ctx, "ALTER TABLE t_audit_event ADD CONSTRAINT ck_test_operation CHECK (operation <> 'test_poison')")
	if err != nil {
		t.Fatal(err)
	}
	bad := auditctx.Event{Kind: "login", Status: 200, Operation: "login", AdditionalUserOperation: "test_poison"}
	if err := module.RecordHTTP(ctx, bad); err != nil {
		t.Fatal(err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); module.ConsumeStream(workerCtx) }()
	t.Cleanup(func() { cancel(); <-done })

	await := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal("stream failed to drain")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	length := func(key string) int64 {
		t.Helper()
		n, err := client.Eval(ctx, "return redis.call('XLEN',KEYS[1])", []string{key}).Int64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	failedKey := settings.Stream + ":failed"
	await(func() bool { return length(settings.Stream) == 0 && length(failedKey) == 1 })
	if rows() != 3 {
		t.Fatalf("want one login, its paired operation, and the independent operation; got %d", count)
	}
	var kinds, operations, deliveries int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT kind), COUNT(DISTINCT operation), COUNT(DISTINCT delivery_id)
		FROM t_audit_event WHERE username='audited-account'`).Scan(&kinds, &operations, &deliveries); err != nil || kinds != 2 || operations != 3 || deliveries != 2 {
		t.Fatalf("audit identities: %d/%d/%d: %v", kinds, operations, deliveries, err)
	}
	// Replay the dead letter with its original delivery identity after fixing the cause.
	if _, err := store.DB.ExecContext(ctx, "ALTER TABLE t_audit_event DROP CONSTRAINT ck_test_operation"); err != nil {
		t.Fatal(err)
	}
	if err := client.Eval(ctx, `local entry = redis.call('XRANGE',KEYS[2],'-','+','COUNT',1)
		return redis.call('XADD',KEYS[1],'*',unpack(entry[1][2]))`, []string{settings.Stream, failedKey}).Err(); err != nil {
		t.Fatal(err)
	}
	await(func() bool { return length(settings.Stream) == 0 && rows() == 5 })

	// A Redis admission failure must neither block on SQL nor insert a fallback row.
	other := audit.New(store, limits, config.AuditConfig{Stream: "audit:wrong-type"})
	other.UseRedisStream(client)
	if err := client.Set(ctx, "audit:wrong-type", "value", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := other.RecordHTTP(ctx, bad); err == nil || rows() != 5 {
		t.Fatal("expected admission error and no synchronous fallback")
	}
}
