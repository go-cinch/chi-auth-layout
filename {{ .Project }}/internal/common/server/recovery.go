{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/gin-gonic/gin"
)

func Recoverer() gin.HandlerFunc {
	logger := slog.Default()
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				c.Abort()
				if recovered == http.ErrAbortHandler {
					logger.WarnContext(c.Request.Context(), "http request aborted",
						"operation", "handle_http_request", "outcome", "aborted", "method", c.Request.Method)
					// Let net/http abort the connection without writing a response.
					panic(recovered)
				}
				panicMessage := fmt.Sprint(recovered)
				logger.ErrorContext(c.Request.Context(), "panic recovered: "+panicMessage,
					"operation", "recover_http_panic", "outcome", "failure", "method", c.Request.Method, "error", panicMessage, "stack", string(debug.Stack()))
				if !c.Writer.Written() {
					WriteError(c.Writer, c.Request, http.StatusInternalServerError, apperror.Internal)
				}
			}
		}()
		c.Next()
	}
}
{{ else -}}
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

func Recoverer() func(http.Handler) http.Handler {
	logger := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writer := wrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						logger.WarnContext(r.Context(), "http request aborted",
							"operation", "handle_http_request", "outcome", "aborted", "method", r.Method)
						// Let net/http abort the connection without writing a response.
						panic(recovered)
					}
					panicMessage := fmt.Sprint(recovered)
					logger.ErrorContext(r.Context(), "panic recovered: "+panicMessage,
						"operation", "recover_http_panic", "outcome", "failure", "method", r.Method, "error", panicMessage, "stack", string(debug.Stack()))
					if !responseWritten(writer) {
						WriteError(writer, r, http.StatusInternalServerError, apperror.Internal)
					}
				}
			}()
			next.ServeHTTP(writer, r)
		})
	}
}
{{ end -}}
