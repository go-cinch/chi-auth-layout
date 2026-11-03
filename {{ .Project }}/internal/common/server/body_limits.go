package server

import (
	"context"
	"net/http"
	"{{ .Computed.module_name_final }}/internal/common/config"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

type BodyLimit string

const (
	BodyAuthChallenge BodyLimit = "authChallenge"
	BodySliderChallenge BodyLimit = "sliderChallenge"
	BodyAuthVerification BodyLimit = "authVerification"
	BodyAuth BodyLimit = "auth"
	BodyBusiness BodyLimit = "business"
	BodyDictionary BodyLimit = "dictionary"
)

type bodyLimitsKey struct{}

// RequestBodyLimits carries one router's immutable limits to its HTTP decoders.
{{ if eq .Computed.http_router_final "gin" -}}
func RequestBodyLimits(limits config.HTTPBodyLimitsConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), bodyLimitsKey{}, limits))
		c.Next()
	}
}
{{ else -}}
func RequestBodyLimits(limits config.HTTPBodyLimitsConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), bodyLimitsKey{}, limits)))
		})
	}
}
{{ end -}}

func requestBodyLimit(r *http.Request, category BodyLimit) int64 {
	limits, ok := r.Context().Value(bodyLimitsKey{}).(config.HTTPBodyLimitsConfig)
	if !ok { limits = config.DefaultHTTPBodyLimits() }
	switch category {
	case BodyAuthChallenge: return int64(limits.AuthChallenge)
	case BodySliderChallenge: return int64(limits.SliderChallenge)
	case BodyAuthVerification: return int64(limits.AuthVerification)
	case BodyAuth: return int64(limits.Auth)
	case BodyBusiness: return int64(limits.Business)
	case BodyDictionary: return int64(limits.Dictionary)
	default: return 0
	}
}
