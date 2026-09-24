package server

import (
{{- if ne .Computed.http_router_final "gin" }}
 "net/http"
{{- end }}
 "{{ .Computed.module_name_final }}/internal/common/i18n"
{{- if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{- end }}
)

{{ if eq .Computed.http_router_final "gin" -}}
func Locale() gin.HandlerFunc {
 return func(c *gin.Context) {
  locale := i18n.Match(c.Request.Header.Get("Accept-Language"))
  c.Request = c.Request.WithContext(i18n.WithLocale(c.Request.Context(), locale))
  c.Header("Content-Language", locale)
  c.Writer.Header().Add("Vary", "Accept-Language")
  c.Next()
 }
}
{{ else -}}
func Locale() func(http.Handler) http.Handler {
 return func(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   locale := i18n.Match(r.Header.Get("Accept-Language"))
   w.Header().Set("Content-Language", locale)
   w.Header().Add("Vary", "Accept-Language")
   next.ServeHTTP(w, r.WithContext(i18n.WithLocale(r.Context(), locale)))
  })
 }
}
{{ end -}}
