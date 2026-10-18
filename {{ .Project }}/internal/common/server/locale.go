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
func Locale(translators ...i18n.Translator) gin.HandlerFunc {
 return func(c *gin.Context) {
  ctx:=c.Request.Context(); if len(translators)>0 {ctx=i18n.WithTranslator(ctx,translators[0])}; locale := i18n.Locale(ctx,c.Request.Header.Get("Accept-Language"))
  c.Request = c.Request.WithContext(i18n.WithLocale(ctx, locale))
  c.Header("Content-Language", locale)
  c.Writer.Header().Add("Vary", "Accept-Language")
  c.Next()
 }
}
{{ else -}}
func Locale(translators ...i18n.Translator) func(http.Handler) http.Handler {
 return func(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   ctx:=r.Context(); if len(translators)>0 {ctx=i18n.WithTranslator(ctx,translators[0])}; locale := i18n.Locale(ctx,r.Header.Get("Accept-Language"))
   w.Header().Set("Content-Language", locale)
   w.Header().Add("Vary", "Accept-Language")
   next.ServeHTTP(w, r.WithContext(i18n.WithLocale(ctx, locale)))
  })
 }
}
{{ end -}}
