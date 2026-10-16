package server

import (
	"context"
	"net"
	"net/http"
	"strings"
{{- if eq .Computed.http_router_final "gin" }}

	"github.com/gin-gonic/gin"
{{- end }}
)

type clientIPKey struct{}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}

func resolveClientIP(r *http.Request) string {
	// The deployment controls trusted sources; the gateway overwrites this header.
	// Accept exactly one valid IP and ignore other forwarding headers.
	if values := r.Header.Values("X-Verified-Client-IP"); len(values) == 1 {
		if ip := net.ParseIP(strings.TrimSpace(values[0])); ip != nil {
			return ip.String()
		}
	}
	return peerIP(r.RemoteAddr)
}

func withClientIP(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPKey{}, resolveClientIP(r)))
}

func clientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r.RemoteAddr)
}

// ClientIP accepts X-Verified-Client-IP from any connection. Missing or invalid
// values use the transport peer. RemoteAddr stays unchanged for access logging.
{{ if eq .Computed.http_router_final "gin" -}}
func ClientIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = withClientIP(c.Request)
		c.Next()
	}
}
{{ else -}}
func ClientIP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, withClientIP(r))
		})
	}
}
{{ end -}}
