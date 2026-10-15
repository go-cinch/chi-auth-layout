{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverer(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := gin.New()
	handler.Use(Recoverer())
	handler.GET("/*path", func(c *gin.Context) { panic("boom") })
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(output.String(), "panic recovered") {
		t.Fatalf("response = %d, log = %q", recorder.Code, output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
	if entry["msg"] != "panic recovered: boom" || entry["error"] != "boom" || entry["outcome"] != "failure" || entry["method"] != "GET" {
		t.Fatalf("panic fields: %v", entry)
	}
}

func TestRecovererLogsAbort(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := gin.New()
	handler.Use(Recoverer())
	handler.GET("/*path", func(c *gin.Context) { panic(http.ErrAbortHandler) })
	recorder := httptest.NewRecorder()
	recorder.Code = 0
	var propagated any
	func() {
		defer func() { propagated = recover() }()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/abort", nil))
	}()
	if propagated != http.ErrAbortHandler {
		t.Fatalf("propagated panic = %v, want http.ErrAbortHandler", propagated)
	}
	if recorder.Code != 0 || recorder.Body.Len() != 0 {
		t.Fatalf("abort wrote response: %d %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(output.String(), "http request aborted") || strings.Contains(output.String(), "stack") {
		t.Fatalf("abort log = %q", output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
	if entry["operation"] != "handle_http_request" || entry["outcome"] != "aborted" || entry["method"] != "GET" {
		t.Fatalf("abort fields: %v", entry)
	}
}

func TestRecovererAbortsConnection(t *testing.T) {
	handler := gin.New()
	handler.Use(Recoverer())
	handler.GET("/abort", func(c *gin.Context) { panic(http.ErrAbortHandler) })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	response, err := server.Client().Get(server.URL + "/abort")
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("abort returned HTTP response: %d", response.StatusCode)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("request error = %v, want connection closed before response", err)
	}
}

func TestRecovererKeepsWrittenResponse(t *testing.T) {
	r := gin.New()
	r.Use(Recoverer())
	r.GET("/", func(c *gin.Context) { c.String(202, "accepted"); panic("after write") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 202 || w.Body.String() != "accepted" {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
}
{{ else -}}
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverer(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := Recoverer()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(output.String(), "panic recovered") {
		t.Fatalf("response = %d, log = %q", recorder.Code, output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
	if entry["msg"] != "panic recovered: boom" || entry["error"] != "boom" || entry["outcome"] != "failure" || entry["method"] != "GET" {
		t.Fatalf("panic fields: %v", entry)
	}
}

func TestRecovererLogsAbort(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := Recoverer()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	recorder := httptest.NewRecorder()
	recorder.Code = 0
	var propagated any
	func() {
		defer func() { propagated = recover() }()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/abort", nil))
	}()
	if propagated != http.ErrAbortHandler {
		t.Fatalf("propagated panic = %v, want http.ErrAbortHandler", propagated)
	}
	if recorder.Code != 0 || recorder.Body.Len() != 0 {
		t.Fatalf("abort wrote response: %d %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(output.String(), "http request aborted") || strings.Contains(output.String(), "stack") {
		t.Fatalf("abort log = %q", output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
	if entry["operation"] != "handle_http_request" || entry["outcome"] != "aborted" || entry["method"] != "GET" {
		t.Fatalf("abort fields: %v", entry)
	}
}

func TestRecovererAbortsConnection(t *testing.T) {
	handler := Recoverer()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	response, err := server.Client().Get(server.URL + "/abort")
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("abort returned HTTP response: %d", response.StatusCode)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("request error = %v, want connection closed before response", err)
	}
}
{{ end -}}
