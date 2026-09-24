{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"bytes"
	"github.com/gin-gonic/gin"
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
}

func TestRecovererLogsAbort(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := gin.New()
	handler.Use(Recoverer())
	handler.GET("/*path", func(c *gin.Context) { panic(http.ErrAbortHandler) })
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abort", nil))
	if !strings.Contains(output.String(), "http request aborted") || strings.Contains(output.String(), "stack") {
		t.Fatalf("abort log = %q", output.String())
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
}

func TestRecovererLogsAbort(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	handler := Recoverer()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abort", nil))
	if !strings.Contains(output.String(), "http request aborted") || strings.Contains(output.String(), "stack") {
		t.Fatalf("abort log = %q", output.String())
	}
}
{{ end -}}
