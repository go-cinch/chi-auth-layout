{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeout cancels the request context; handlers must observe cancellation.
func RequestTimeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if timeout <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			WriteError(c.Writer, c.Request, http.StatusGatewayTimeout, apperror.GatewayTimeout)
		}
	}
}
{{ else -}}
package server

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"context"
	"net/http"
	"time"
)

func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if timeout <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writer := wrapResponseWriter(w, r.ProtoMajor)
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(writer, r.WithContext(ctx))
			if ctx.Err() == context.DeadlineExceeded && !responseWritten(writer) {
				WriteError(writer, r, http.StatusGatewayTimeout, apperror.GatewayTimeout)
			}
		})
	}
}
{{ end -}}
