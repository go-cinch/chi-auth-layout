package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/config"
{{- if eq .Computed.http_router_final "gin" }}

	"github.com/gin-gonic/gin"
{{- end }}
)

func TestClientIPResolver(t *testing.T) {
	for _, test := range []struct {
		name, proxies, remote, verified, forwarded, real, want string
	}{
		{name: "local IPv4", remote: "127.0.0.1:8080", want: "127.0.0.1"},
		{name: "local IPv6", remote: "[::1]:8080", want: "::1"},
		{name: "localhost", remote: "localhost:8080", want: "localhost"},
		{name: "bare localhost", remote: "localhost", want: "localhost"},
		{name: "bare IP", remote: "127.0.0.1", want: "127.0.0.1"},
		{name: "direct IPv6", remote: "[2001:db8::5]:8080", want: "2001:db8::5"},
		{name: "default ignores spoofed headers", remote: "127.0.0.1:8080", verified: "203.0.113.4", forwarded: "203.0.113.5", real: "203.0.113.6", want: "127.0.0.1"},
		{name: "untrusted peer", proxies: "10.62.32.0/24", remote: "198.51.100.7:8080", verified: "203.0.113.4", forwarded: "203.0.113.5", real: "203.0.113.6", want: "198.51.100.7"},
		{name: "APISIX verified IP", proxies: "10.62.32.0/24", remote: "10.62.32.142:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "ignore real IP", proxies: "10.62.32.0/24", remote: "10.62.32.142:8080", real: "203.0.113.5", want: "10.62.32.142"},
		{name: "ignore forwarded chain", proxies: "10.62.32.0/24", remote: "10.62.32.142:8080", forwarded: "192.0.2.99, 203.0.113.5", want: "10.62.32.142"},
		{name: "verified IP overrides forwarding headers", proxies: "10.62.32.0/24", remote: "10.62.32.142:8080", verified: "203.0.113.4", forwarded: "192.0.2.99", real: "203.0.113.6", want: "203.0.113.4"},
		{name: "multiple trusted proxies", proxies: "10.62.32.0/24, 192.0.2.10", remote: "192.0.2.10:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "IPv6 client", proxies: "10.62.32.142", remote: "10.62.32.142:8080", verified: "2001:db8::5", want: "2001:db8::5"},
		{name: "IPv6 proxy CIDR", proxies: "2001:db8:1::/64", remote: "[2001:db8:1::2]:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "IPv6 proxy IP", proxies: "2001:db8:1::2", remote: "[2001:db8:1::2]:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "local IPv4 proxy", proxies: " localhost ", remote: "127.0.0.1:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "local IPv6 proxy", proxies: "LOCALHOST", remote: "[::1]:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "IPv4 mapped proxy", proxies: "127.0.0.1", remote: "[::ffff:127.0.0.1]:8080", verified: "203.0.113.5", want: "203.0.113.5"},
		{name: "IPv4 mapped client", proxies: "localhost", remote: "127.0.0.1:8080", verified: "::ffff:203.0.113.5", want: "203.0.113.5"},
		{name: "trim verified IP", proxies: "localhost", remote: "127.0.0.1:8080", verified: " 203.0.113.5 ", want: "203.0.113.5"},
		{name: "trusted peer without header", proxies: "localhost", remote: "127.0.0.1:8080", want: "127.0.0.1"},
		{name: "empty verified IP ignores forwarding headers", proxies: "localhost", remote: "127.0.0.1:8080", forwarded: "203.0.113.5", real: "203.0.113.6", want: "127.0.0.1"},
		{name: "invalid verified IP ignores forwarding headers", proxies: "localhost", remote: "127.0.0.1:8080", verified: "invalid", forwarded: "203.0.113.5", real: "203.0.113.6", want: "127.0.0.1"},
		{name: "verified IP list rejected", proxies: "localhost", remote: "127.0.0.1:8080", verified: "203.0.113.4, 203.0.113.5", want: "127.0.0.1"},
		{name: "verified hostname rejected", proxies: "localhost", remote: "127.0.0.1:8080", verified: "localhost", want: "127.0.0.1"},
		{name: "verified port rejected", proxies: "localhost", remote: "127.0.0.1:8080", verified: "203.0.113.5:8080", want: "127.0.0.1"},
		{name: "malformed peer", proxies: "localhost", remote: "local", verified: "203.0.113.5", want: "local"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver, err := newClientIPResolver(test.proxies)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote
			request.Header.Set("X-Verified-Client-IP", test.verified)
			request.Header.Set("X-Forwarded-For", test.forwarded)
			request.Header.Set("X-Real-IP", test.real)
			if got := resolver.resolve(request); got != test.want {
				t.Fatalf("client IP = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClientIPRepeatedHeaders(t *testing.T) {
	resolver, err := newClientIPResolver("10.62.32.0/24")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.62.32.142:8080"
	request.Header.Add("X-Verified-Client-IP", "203.0.113.5")
	request.Header.Add("X-Verified-Client-IP", "203.0.113.6")
	request.Header.Set("X-Forwarded-For", "192.0.2.99")
	request.Header.Set("X-Real-IP", "192.0.2.99")
	if got := resolver.resolve(request); got != "10.62.32.142" {
		t.Fatalf("ambiguous verified IP accepted: %q", got)
	}
}

func TestClientIPInvalidProxyConfig(t *testing.T) {
	for _, value := range []string{"*", "proxy.example", "10.0.0.0/99", "127.0.0.1,", ",localhost"} {
		if _, err := ClientIP(value); err == nil {
			t.Fatalf("invalid proxy configuration accepted: %q", value)
		}
	}
	var cfg config.Config
	cfg.HTTP.TrustedProxies = "invalid"
	if _, err := NewRouter(&cfg, nil, nil); err == nil || !strings.Contains(err.Error(), "http.trustedProxies") {
		t.Fatalf("invalid router proxy configuration: %v", err)
	}
}

type clientIPAuditModule struct {
	*recordedAudit
}

func (clientIPAuditModule) Name() string { return "/client-ip" }

func (clientIPAuditModule) Public() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (clientIPAuditModule) HTTP(r *gin.RouterGroup) {
	r.GET("", func(c *gin.Context) { _, _ = io.WriteString(c.Writer, clientIP(c.Request)+"|"+c.Request.RemoteAddr) })
}
{{ else -}}
func (clientIPAuditModule) HTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, clientIP(r)+"|"+r.RemoteAddr) })
}
{{ end }}

func TestClientIPRouterAudit(t *testing.T) {
	for _, test := range []struct {
		name, proxies, remote, header, want string
	}{
		{"local", "", "127.0.0.1:51234", "", "127.0.0.1"},
		{"localhost", "", "localhost:51234", "", "localhost"},
		{"IPv6 localhost", "", "[::1]:51234", "", "::1"},
		{"spoofed header", "", "127.0.0.1:51234", "203.0.113.5", "127.0.0.1"},
		{"local proxy", "localhost", "127.0.0.1:51234", "203.0.113.5", "203.0.113.5"},
		{"APISIX", "10.62.32.0/24", "10.62.32.142:51234", "203.0.113.5", "203.0.113.5"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.Config
			cfg.HTTP.TrustedProxies = test.proxies
			recorder := &recordedAudit{target: auditctx.Target{Kind: "login", Module: "test"}}
			router, err := NewRouter(&cfg, nil, nil, clientIPAuditModule{recorder})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
			request.RemoteAddr = test.remote
			request.Header.Set("X-Verified-Client-IP", test.header)
			request.Header.Set("X-Forwarded-For", "192.0.2.99")
			request.Header.Set("X-Real-IP", "192.0.2.99")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != test.want+"|"+test.remote {
				t.Fatalf("client IP/peer response: %d %q", response.Code, response.Body.String())
			}
			if len(recorder.events) != 1 || recorder.events[0].IP != test.want {
				t.Fatalf("audit client IP: %+v", recorder.events)
			}
		})
	}
}

func TestClientIPLocalHTTP(t *testing.T) {
	serverRouter, err := NewRouter(&config.Config{}, nil, nil, clientIPAuditModule{&recordedAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(serverRouter)
	defer local.Close()
	for _, address := range []string{local.URL, strings.Replace(local.URL, "127.0.0.1", "localhost", 1)} {
		response, err := local.Client().Get(address + "/client-ip")
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "127.0.0.1|127.0.0.1:") {
			t.Fatalf("local HTTP %s: status %d, body %q, error %v", address, response.StatusCode, body, readErr)
		}
	}
}
