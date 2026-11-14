package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestPublicLocalizationHTTP(t *testing.T) {
	m, _, _ := newAuthTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(method, target, body string, want int) string {
		t.Helper()
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest(method, target, strings.NewReader(body)))
		if out.Code != want { t.Fatalf("%s %s: got %d: %s", method, target, out.Code, out.Body.String()) }
		if want == http.StatusOK && out.Header().Get("Cache-Control") != "no-cache" { t.Fatal("catalog must revalidate") }
		if strings.Contains(out.Body.String(), `"resource_catalog"`) { t.Fatal("public response exposed management mappings") }
		return out.Body.String()
	}
	if body := request(http.MethodGet, "/pub/i18n", "", http.StatusOK); !strings.Contains(body, `"default_locale"`) || !strings.Contains(body, `"locales"`) || strings.Contains(body, `"messages"`) {
		t.Fatalf("public localization response: %s", body)
	}
	request(http.MethodGet, "/pub/i18n?unknown=1", "", http.StatusBadRequest)
	for _, query := range []string{"f=", "f=unknown", "f=locales,locales", "f=locales&f=messages", "f=messages,frontend_messages", "f=Locales", "f=locales,"} {
		request(http.MethodGet, "/pub/i18n?"+query, "", http.StatusBadRequest)
	}
	localizationBody := request(http.MethodGet, "/pub/i18n?f=scopes", "", http.StatusOK)
	if !strings.Contains(localizationBody, `"scopes"`) || strings.Contains(localizationBody, `"messages"`) || strings.Contains(localizationBody, `"locales"`) {
		t.Fatal(localizationBody)
	}
	localizationBody = request(http.MethodGet, "/pub/i18n?f=frontend_messages", "", http.StatusOK)
	if !strings.Contains(localizationBody, `"messages"`) || strings.Contains(localizationBody, `"backend"`) {
		t.Fatal(localizationBody)
	}
	request(http.MethodGet, "/pub/i18n?bad=%zz", "", http.StatusBadRequest)
}
