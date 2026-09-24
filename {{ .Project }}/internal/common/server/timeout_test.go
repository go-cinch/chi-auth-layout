{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeout(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout time.Duration
		write   bool
		status  int
	}{
		{"deadline", time.Millisecond, false, 504},
		{"already written", time.Millisecond, true, 202},
		{"disabled", 0, false, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequestTimeout(test.timeout))
			r.GET("/", func(c *gin.Context) {
				if test.write {
					c.String(202, "accepted")
				}
				if test.timeout > 0 {
					<-c.Request.Context().Done()
				}
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != test.status {
				t.Fatalf("status: %d", w.Code)
			}
			if test.write && w.Body.String() != "accepted" {
				t.Fatalf("body: %s", w.Body.String())
			}
		})
	}
}

func TestRequestTimeoutPreservesCancellation(t *testing.T) {
	r := gin.New()
	r.Use(RequestTimeout(time.Second))
	r.GET("/", func(c *gin.Context) {
		if c.Request.Context().Err() != context.Canceled {
			t.Error("parent cancellation lost")
		}
		c.Status(204)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	if w.Code != 204 {
		t.Fatalf("status: %d", w.Code)
	}
}
{{ else -}}
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeout(t *testing.T) {
	handler := RequestTimeout(time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d", recorder.Code)
	}
}
{{ end -}}
