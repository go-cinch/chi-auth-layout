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

type clientIPResolver struct {
	trusted []*net.IPNet
}

func newClientIPResolver(proxies string) (clientIPResolver, error) {
	var resolver clientIPResolver
	if strings.TrimSpace(proxies) == "" {
		return resolver, nil
	}
	for _, value := range strings.Split(proxies, ",") {
		value = strings.TrimSpace(value)
		if strings.EqualFold(value, "localhost") {
			resolver.trusted = append(resolver.trusted,
				&net.IPNet{IP: net.IPv4(127, 0, 0, 1), Mask: net.CIDRMask(32, 32)},
				&net.IPNet{IP: net.IPv6loopback, Mask: net.CIDRMask(128, 128)})
			continue
		}
		if _, network, err := net.ParseCIDR(value); err == nil {
			resolver.trusted = append(resolver.trusted, network)
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return clientIPResolver{}, fmt.Errorf("http.trustedProxies contains invalid proxy %q: expected an IP, CIDR, or localhost", value)
		}
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		resolver.trusted = append(resolver.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return resolver, nil
}

func (resolver clientIPResolver) isTrusted(ip net.IP) bool {
	for _, network := range resolver.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

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

func (resolver clientIPResolver) resolve(r *http.Request) string {
	peer := peerIP(r.RemoteAddr)
	if !resolver.isTrusted(net.ParseIP(peer)) {
		return peer
	}
	// The trusted gateway must overwrite this header with its observed client IP.
	// Accept exactly one IP; never fall back to client-supplied forwarding headers.
	if values := r.Header.Values("X-Verified-Client-IP"); len(values) == 1 {
		if ip := net.ParseIP(strings.TrimSpace(values[0])); ip != nil {
			return ip.String()
		}
	}
	return peer
}

func (resolver clientIPResolver) request(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPKey{}, resolver.resolve(r)))
}

func clientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r.RemoteAddr)
}

// ClientIP accepts X-Verified-Client-IP only from configured proxies. Missing or
// invalid values use the transport peer. RemoteAddr stays unchanged for logging.
{{ if eq .Computed.http_router_final "gin" -}}
func ClientIP(trustedProxies string) (gin.HandlerFunc, error) {
	resolver, err := newClientIPResolver(trustedProxies)
	if err != nil {
		return nil, err
	}
	return func(c *gin.Context) {
		c.Request = resolver.request(c.Request)
		c.Next()
	}, nil
}
{{ else -}}
func ClientIP(trustedProxies string) (func(http.Handler) http.Handler, error) {
	resolver, err := newClientIPResolver(trustedProxies)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, resolver.request(r))
		})
	}, nil
}
{{ end -}}
