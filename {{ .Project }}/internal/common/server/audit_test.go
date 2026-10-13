package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/redact"
{{ if eq .Computed.http_router_final "gin" }}

	"github.com/gin-gonic/gin"
{{ end }}
)

type recordedAudit struct {
	events []auditctx.Event
	fail   bool
	target auditctx.Target
}

func (a *recordedAudit) AuditTarget(method, path string) auditctx.Target { return a.target }

func (a *recordedAudit) RecordHTTP(ctx context.Context, e auditctx.Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.events = append(a.events, e)
	if a.fail {
		return errors.New("offline")
	}
	return nil
}

func auditHandler(recorder *recordedAudit, handler http.HandlerFunc) http.Handler {
	policy := redact.New("tenantSecret")

{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	router.Use(Audit(recorder, policy), Recoverer())
	router.POST("/test", func(c *gin.Context) { handler(c.Writer, c.Request) })
	return router
{{ else }}
	return Audit(recorder, policy)(Recoverer()(handler))
{{ end }}

}

func TestAuditCapture(t *testing.T) {
	recorder := &recordedAudit{target: auditctx.Target{Kind: "operation", Module: "test", Operation: "update", ResourceType: "test", ResourceID: "1"}}
	body := `{"name":"visible","password":"never-store","nested":[{"private_key":"never-store","tenant_secret":"never-store"}],"credential":"never-store"}`
	handler := auditHandler(recorder, func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil || string(got) != body {
			t.Fatal("request altered", err)
		}
		auditctx.Identity(r.Context(), 1, "alice")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"access_token":"never-store","ok":true}`)
	})
	req := httptest.NewRequest("POST", "/test?password=never-store&filter=visible", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	req.RemoteAddr = "127.0.0.1:12"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "never-store") {
		t.Fatal("response altered")
	}
	if len(recorder.events) != 1 {
		t.Fatal(recorder.events)
	}
	event := recorder.events[0]
	if event.UserID != 1 || event.IP != "127.0.0.1" || event.Status != 200 || event.ResourceID != "1" {
		t.Fatal(event)
	}
	for _, payload := range []string{string(event.RequestParams), string(event.ResponseParams)} {
		if strings.Contains(payload, "never-store") || !strings.Contains(payload, "[redacted]") {
			t.Fatal("unsafe audit payload", payload)
		}
	}
	if !strings.Contains(string(event.RequestParams), "visible") {
		t.Fatal("safe parameters lost")
	}
}

func TestAuditLoginAndIgnoredTarget(t *testing.T) {
	recorder := &recordedAudit{target: auditctx.Target{Kind: "login", Module: "test"}}
	handler := auditHandler(recorder, func(w http.ResponseWriter, r *http.Request) {
		auditctx.Identity(r.Context(), 0, "alice")
		auditctx.Error(r.Context(), "AUTH_LOGIN_FAILED")
		w.WriteHeader(401)
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", nil))
	event := recorder.events[0]
	if event.Kind != "login" || event.Status != 401 || event.ErrorCode != "AUTH_LOGIN_FAILED" || event.RequestParams != nil || event.ResponseParams != nil {
		t.Fatal(event)
	}
	recorder.target = auditctx.Target{}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", nil))
	if len(recorder.events) != 1 {
		t.Fatal("ignored request captured")
	}
	if auditRecorder(nil) != nil {
		t.Fatal("unexpected recorder")
	}
	ctx, state := auditctx.WithState(context.Background())
	recorder.fail = true
	request := httptest.NewRequest("POST", "/test", nil).WithContext(ctx)
	request.RemoteAddr = "local"
	finishAudit(recorder, request, state, auditctx.Target{Kind: "operation"}, 0, time.Now(), http.Header{}, &auditBuffer{}, &auditBuffer{}, redact.New())
	if recorder.events[1].Status != 200 || recorder.events[1].IP != "local" {
		t.Fatal(recorder.events)
	}
}

func TestAuditFilesAndRecovery(t *testing.T) {
	for _, panicHandler := range []bool{false, true} {
		recorder := &recordedAudit{target: auditctx.Target{Kind: "operation"}}
		handler := auditHandler(recorder, func(w http.ResponseWriter, r *http.Request) {
			if panicHandler {
				panic("test failure")
			}
			w.Header().Set("Content-Disposition", `attachment; filename="test.csv"`)
			_, _ = io.WriteString(w, "file bytes")
		})
		req := httptest.NewRequest("POST", "/test", strings.NewReader("private file bytes"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		e := recorder.events[0]
		if string(e.RequestParams) != `"file"` {
			t.Fatal(string(e.RequestParams))
		}
		if panicHandler {
			if e.Status != 500 {
				t.Fatal(e)
			}
		} else if string(e.ResponseParams) != `"file"` || w.Body.String() != "file bytes" {
			t.Fatal(e)
		}
	}
}
