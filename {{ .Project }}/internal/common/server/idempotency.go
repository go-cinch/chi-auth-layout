package server

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/idempotency"
{{- if eq .Computed.http_router_final "gin" }}

	"github.com/gin-gonic/gin"
{{- end }}
)

const IdempotencyHeader = "X-Idempotent"

func Idempotency(store idempotency.Store, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !claimIdempotency(w, r, store, ttl) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

{{ if eq .Computed.http_router_final "gin" -}}
func GinIdempotency(store idempotency.Store, ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !claimIdempotency(c.Writer, c.Request, store, ttl) {
			c.Abort()
			return
		}
		c.Next()
	}
}
{{ end -}}

func claimIdempotency(w http.ResponseWriter, r *http.Request, store idempotency.Store, ttl time.Duration) bool {
	if r.Method != http.MethodPost {
		return true
	}
	key := strings.TrimSpace(r.Header.Get(IdempotencyHeader))
	if key == "" {
		return true
	}
	if !validIdempotencyKey(key) {
		WriteError(w, r, http.StatusBadRequest, apperror.IdempotencyKey)
		return false
	}
	if store == nil {
		slog.ErrorContext(r.Context(), "claim idempotency key failed: store is unavailable")
		WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return false
	}
	claimed, err := store.Claim(r.Context(), key, ttl)
	if err != nil {
		slog.ErrorContext(r.Context(), "claim idempotency key failed: "+err.Error())
		WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return false
	}
	if !claimed {
		WriteError(w, r, http.StatusConflict, apperror.DuplicateRequest)
		return false
	}
	return true
}

func validIdempotencyKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, character := range key {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("-_.:", character) {
			continue
		}
		return false
	}
	return true
}
