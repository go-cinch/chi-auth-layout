package server

import (
	"context"
 "fmt"
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

func parseTrustedProxies(value string) ([]*net.IPNet, error) {
 var networks []*net.IPNet
 for _, cidr := range strings.Split(value, ",") {
  cidr = strings.TrimSpace(cidr)
  if cidr == "" { continue }
  _, network, err := net.ParseCIDR(cidr)
  if err != nil { return nil, fmt.Errorf("http.clientIP.trustedProxies: invalid CIDR %q", cidr) }
  networks = append(networks, network)
 }
 return networks, nil
}

func resolveClientIP(r *http.Request, trusted ...*net.IPNet) string {
 peer := peerIP(r.RemoteAddr)
 ip := net.ParseIP(peer)
 for _, network := range trusted {
  if network.Contains(ip) {
   if values := r.Header.Values("X-Verified-Client-IP"); len(values) == 1 {
    if forwarded := net.ParseIP(strings.TrimSpace(values[0])); forwarded != nil { return forwarded.String() }
   }
   break
  }
 }
 return peer
}

func withClientIP(r *http.Request, trusted ...*net.IPNet) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPKey{}, resolveClientIP(r, trusted...)))
}

// RequestClientIP returns the identity resolved by the router, or the TCP peer.
// Callers never independently trust forwarding headers.
func RequestClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r.RemoteAddr)
}

func clientIP(r *http.Request) string { return RequestClientIP(r) }

// ClientIP accepts the verified header only from configured gateways.
// RemoteAddr stays unchanged for access logging.
{{ if eq .Computed.http_router_final "gin" -}}
func ClientIP(trusted ...*net.IPNet) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = withClientIP(c.Request, trusted...)
		c.Next()
	}
}
{{ else -}}
func ClientIP(trusted ...*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, withClientIP(r, trusted...))
		})
	}
}
{{ end -}}
