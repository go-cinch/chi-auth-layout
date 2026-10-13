package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/redact"
)

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
