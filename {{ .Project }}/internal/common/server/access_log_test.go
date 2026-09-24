package server

import (
"{{ .Computed.module_name_final }}/internal/common/apperror"
    "bytes"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "regexp"
    "strings"
    "testing"

    "{{ .Computed.module_name_final }}/internal/common/redact"
{{ if eq .Computed.http_router_final "gin" -}}
    "github.com/gin-gonic/gin"
{{ else -}}
    "github.com/go-chi/chi/v5"
{{ end -}}
)

func TestAccessLogRedactsPath(t *testing.T) {
    var output bytes.Buffer
    previous := slog.Default()
    slog.SetDefault(slog.New(slog.NewJSONHandler(&output,nil)))
    t.Cleanup(func(){ slog.SetDefault(previous) })
{{ if eq .Computed.http_router_final "gin" -}}
    r := gin.New()
    r.Use(AccessLog(redact.New()),Recoverer())
    r.NoRoute(func(c *gin.Context){ WriteError(c.Writer,c.Request,404,apperror.NotFound) })
    r.GET("/reset/:token",func(c *gin.Context){ c.Status(204) })
    r.GET("/user/:id",func(c *gin.Context){ WriteOK(c.Writer,map[string]string{"id":c.Param("id")}) })
    r.GET("/panic",func(c *gin.Context){ panic("boom") })
    resetRoute,userRoute := "/reset/:token","/user/:id"
{{ else -}}
    r := chi.NewRouter()
    r.Use(AccessLog(redact.New()),Recoverer())
    r.NotFound(func(w http.ResponseWriter,r *http.Request){ WriteError(w,r,404,apperror.NotFound) })
    r.Get("/reset/{token}",func(w http.ResponseWriter,_ *http.Request){ w.WriteHeader(204) })
    r.Get("/user/{id}",func(w http.ResponseWriter,req *http.Request){ WriteOK(w,map[string]string{"id":chi.URLParam(req,"id")}) })
    r.Get("/panic",func(http.ResponseWriter,*http.Request){ panic("boom") })
    resetRoute,userRoute := "/reset/{token}","/user/{id}"
{{ end -}}
    for _, test := range []struct{path, route, loggedPath string; status int}{
        {"/reset/abcdef",resetRoute,"/reset/abc***",204},
        {"/user/42",userRoute,"/user/42",200},
        {"/missing","-","/missing",404},
        {"/panic","/panic","/panic",500},
    } {
        output.Reset()
        request := httptest.NewRequest(http.MethodGet,test.path,nil)
        request.RemoteAddr = "127.0.0.1:12345"
        request.Header.Set("User-Agent","SummaryTest/1")
        r.ServeHTTP(httptest.NewRecorder(),request)
        lines := strings.Split(strings.TrimSpace(output.String()),"\n")
        var entry map[string]json.RawMessage
        if err := json.Unmarshal([]byte(lines[len(lines)-1]),&entry); err != nil { t.Fatal(err) }
        var message string
        if err := json.Unmarshal(entry["msg"],&message); err != nil { t.Fatal(err) }
        pattern := fmt.Sprintf(`^GET %d [0-9]+ms %s %s$`,test.status,regexp.QuoteMeta(test.route),regexp.QuoteMeta(test.loggedPath))
        if !regexp.MustCompile(pattern).MatchString(message) { t.Fatalf("message: %q, want %q",message,pattern) }
        if strings.Contains(output.String(),"abcdef") { t.Fatalf("secret leaked: %s",output.String()) }
        for _, key := range []string{"method","status","duration_ms","route","path","bytes","client"} {
            if _, exists := entry[key]; exists { t.Errorf("unexpected top-level attribute %q: %s",key,output.String()) }
        }
        for key, want := range map[string]string{"remote_addr": request.RemoteAddr, "user_agent": request.UserAgent()} {
            var value string
            if err := json.Unmarshal(entry[key], &value); err != nil { t.Fatal(err) }
            if value != want { t.Errorf("%s = %q, want %q", key, value, want) }
        }
    }
    request := httptest.NewRequest(http.MethodGet,"/plain",nil)
{{ if eq .Computed.http_router_final "gin" -}}
    path := redactedPath(request,nil,redact.New())
{{ else -}}
    path := redactedPath(request,redact.New())
{{ end -}}
    if path != "/plain" { t.Fatalf("plain path: %s",path) }
}
