package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/redact"
)

func TestAuditHeaderRedactionAndLimits(t *testing.T) {
	policy := redact.New("X-Custom-Value")
	r := httptest.NewRequest(http.MethodPost, "https://auth.example/login", nil)
	for _, name := range []string{"authorization", "PROXY-AUTHORIZATION", "Cookie", "Set-Cookie", "X-Api-Key", "X-Access-Token", "X-JWT", "X-CSRF", "X-XSRF", "X-Signature", "Baggage", "X-Custom-Value"} {
		r.Header.Set(name, "never-store")
	}
	r.Header.Set("Referer", "https://user:never-store@example.com/login?token=never-store&view=logs#never-store")
	r.Header.Set("X-Original-URI", "/login?password=never-store&q=safe")
	r.Header.Set("X-Debug", `{"password":"never-store","value":"safe"}`)
	encoded := auditHeaders(r, policy)
	if strings.Contains(string(encoded), "never-store") || !strings.Contains(string(encoded), "safe") {
		t.Fatalf("unsafe header snapshot: %s", encoded)
	}
	if r.Header.Get("Authorization") != "never-store" {
		t.Fatal("request headers mutated")
	}
	for _, value := range []string{"%ZZ", "/login?token=%ZZ"} {
		if got := auditHeaderURL("Referer", value, policy); got != "[redacted]" {
			t.Fatalf("invalid URL captured: %q", got)
		}
	}
	if got := auditHeaderURL("Referer", "https://example.com/?q=safe", policy); got != "https://example.com/?q=safe" {
		t.Fatalf("safe URL changed: %s", got)
	}
	for _, value := range []string{strings.Repeat("x", auditPayloadLimit+1), strings.Repeat("\x01", auditPayloadLimit/2)} {
		r.Header.Set("X-Large", value)
		encoded = auditHeaders(r, policy)
		if len(encoded) > auditPayloadLimit || !json.Valid(encoded) || !strings.Contains(string(encoded), "size limit") {
			t.Fatalf("unbounded header snapshot: %d bytes", len(encoded))
		}
	}
}

func TestAuditPayloadBoundaries(t *testing.T) {
	policy := redact.New()
	cases := []struct{ media, body, want string }{
		{"application/json", `{"password":"hidden","array":[{"token":"hidden"}],"number":9007199254740991}`, `{"array":[{"token":"[redacted]"}],"number":9007199254740991,"password":"[redacted]"}`},
		{"application/json", "", `null`},
		{"application/json", "{", `"[omitted: unstructured body]"`},
		{"application/json", "{} {}", `"[omitted: unstructured body]"`},
		{"text/plain", "secret plaintext", `"[omitted: unstructured body]"`},
		{"application/octet-stream", "secret bytes", `"file"`},
		{"application/x-www-form-urlencoded", "password=hidden&q=one&q=two", `{"password":"[redacted]","q":["one","two"]}`},
		{"application/x-www-form-urlencoded", "q=%ZZ", `"[omitted: invalid form]"`},
	}
	for _, c := range cases {
		header := http.Header{"Content-Type": []string{c.media}}
		buffer := &auditBuffer{}
		_, _ = buffer.Write([]byte(c.body))
		if got := string(auditResponse(header, buffer, policy)); got != c.want {
			t.Fatalf("%s: %s", c.media, got)
		}
	}
	buffer := &auditBuffer{}
	data := []byte(strings.Repeat("x", auditPayloadLimit+1))
	n, err := buffer.Write(data)
	if err != nil || n != len(data) || len(buffer.data) != auditPayloadLimit || string(auditResponse(http.Header{}, buffer, policy)) != `"[omitted: size limit]"` {
		t.Fatal("capture not bounded")
	}
	for _, query := range []string{"bad=%ZZ", strings.Repeat("q", auditPayloadLimit+1)} {
		req := httptest.NewRequest("POST", "/test?"+query, nil)
		if !strings.Contains(string(auditRequest(req, &auditBuffer{}, policy)), "omitted") {
			t.Fatal("unsafe query")
		}
	}
}

func TestAuditInlineFiles(t *testing.T) {
	header := http.Header{"Content-Type": []string{"text/plain"}, "Content-Disposition": []string{`inline; filename="private.txt"`}}
	if !auditFile(header) {
		t.Fatal("inline file must be omitted")
	}
	buffer := &auditBuffer{}
	_, _ = buffer.Write([]byte("file contents"))
	if string(auditResponse(header, buffer, redact.New())) != `"file"` {
		t.Fatal("file captured")
	}
}
