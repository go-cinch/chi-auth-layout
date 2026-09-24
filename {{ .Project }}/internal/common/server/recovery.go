{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recoverer() gin.HandlerFunc {
	logger := slog.Default()
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				c.Abort()
				if recovered == http.ErrAbortHandler {
					logger.WarnContext(c.Request.Context(), "http request aborted")
					return
				}
				logger.ErrorContext(c.Request.Context(), "panic recovered: "+fmt.Sprint(recovered), "stack", string(debug.Stack()))
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
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recoverer() func(http.Handler) http.Handler {
	logger := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writer := wrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						logger.WarnContext(r.Context(), "http request aborted")
						return
					}
					logger.ErrorContext(r.Context(), "panic recovered: "+fmt.Sprint(recovered), "stack", string(debug.Stack()))
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
