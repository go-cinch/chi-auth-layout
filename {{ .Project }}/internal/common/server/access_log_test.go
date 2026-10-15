package server

import (
"{{ .Computed.module_name_final }}/internal/common/apperror"
    "bytes"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "net/http/httptest"
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
    r.HandleMethodNotAllowed = true
    r.Use(AccessLog(redact.New()),Recoverer())
    r.NoRoute(func(c *gin.Context){ WriteError(c.Writer,c.Request,404,apperror.NotFound) })
    r.NoMethod(func(c *gin.Context){ WriteError(c.Writer,c.Request,405,apperror.MethodNotAllowed) })
    r.GET("/reset/:token",func(c *gin.Context){ c.Status(204) })
    r.GET("/user/:id",func(c *gin.Context){ WriteOK(c.Writer,map[string]string{"id":c.Param("id")}) })
    r.GET("/plain",func(*gin.Context){})
    r.GET("/panic",func(c *gin.Context){ panic("boom") })
    resetRoute,userRoute := "/reset/:token","/user/:id"
{{ else -}}
    r := chi.NewRouter()
    r.Use(AccessLog(redact.New()),Recoverer())
    r.NotFound(func(w http.ResponseWriter,r *http.Request){ WriteError(w,r,404,apperror.NotFound) })
    r.Get("/reset/{token}",func(w http.ResponseWriter,_ *http.Request){ w.WriteHeader(204) })
    r.Get("/user/{id}",func(w http.ResponseWriter,req *http.Request){ WriteOK(w,map[string]string{"id":chi.URLParam(req,"id")}) })
    r.Get("/plain",func(http.ResponseWriter,*http.Request){})
    r.Get("/files*",func(http.ResponseWriter,*http.Request){})
    r.Get("/assets/*",func(http.ResponseWriter,*http.Request){})
    r.Get("/panic",func(http.ResponseWriter,*http.Request){ panic("boom") })
    resetRoute,userRoute := "/reset/{token}","/user/{id}"
{{ end -}}
    for _, test := range []struct{method, path, route, loggedPath string; status int}{
        {"GET","/reset/abcdef?password=never-log-query",resetRoute,"/reset/abc***",204},
        {"GET","/reset/abc%64efghijk",resetRoute,"/reset/abc***ijk",204},
        {"GET","/reset/ab",resetRoute,"/reset/***",204},
        {"POST","/reset/secret-token-value","-","-",405},
        {"POST","/reset/abc%64efghijk","-","-",405},
        {"GET","/reset/secret-token-value/unknown","-","-",404},
        {"GET","/user/42",userRoute,"/user/42",200},
        {"GET","/user/43",userRoute,"/user/43",200},
        {"GET","/missing","-","-",404},
        {"GET","/plain","/plain","/plain",200},
        {"GET","/panic","/panic","/panic",500},
{{ if ne .Computed.http_router_final "gin" -}}
        {"GET","/files-secret-token-value","/files*","-",200},
        {"GET","/assets/secret-token-value","/assets/*","-",200},
{{ end -}}
    } {
        output.Reset()
        request := httptest.NewRequest(test.method,test.path,nil)
        request.RemoteAddr = "127.0.0.1:12345"
        request.Header.Set("User-Agent","SummaryTest/1")
        response := httptest.NewRecorder()
        r.ServeHTTP(response,request)
        if response.Code != test.status { t.Fatalf("response = %d, want %d", response.Code, test.status) }
        lines := strings.Split(strings.TrimSpace(output.String()),"\n")
        var entry map[string]json.RawMessage
        if err := json.Unmarshal([]byte(lines[len(lines)-1]),&entry); err != nil { t.Fatal(err) }
        var message string
        if err := json.Unmarshal(entry["msg"],&message); err != nil { t.Fatal(err) }
        for _, key := range []string{"status", "duration_ms"} {
            if len(entry[key]) == 0 || string(entry[key]) == "null" { t.Fatalf("missing numeric field %s: %s", key, output.String()) }
        }
        var status int
        if err := json.Unmarshal(entry["status"], &status); err != nil { t.Fatal(err) }
        if status != test.status { t.Errorf("status = %d, want %d", status, test.status) }
        var durationMS int64
        if err := json.Unmarshal(entry["duration_ms"], &durationMS); err != nil { t.Fatal(err) }
        if durationMS < 0 { t.Errorf("negative duration: %d", durationMS) }
        wantMessage := fmt.Sprintf("%s %d %dms %s %s", test.method, status, durationMS, test.route, test.loggedPath)
        if message != wantMessage { t.Fatalf("message: %q, want %q", message, wantMessage) }
        for _, secret := range []string{"abcdef", "secret-token-value", "never-log-query", "password="} {
            if strings.Contains(output.String(), secret) { t.Fatalf("secret leaked: %s", output.String()) }
        }
        for key, want := range map[string]string{"method": test.method, "route": test.route, "path": test.loggedPath, "remote_addr": request.RemoteAddr, "user_agent": request.UserAgent()} {
            var value string
            if err := json.Unmarshal(entry[key], &value); err != nil { t.Fatal(err) }
            if value != want { t.Errorf("%s = %q, want %q", key, value, want) }
        }
    }
    request := httptest.NewRequest(http.MethodGet,"/plain",nil)
{{ if eq .Computed.http_router_final "gin" -}}
    path := redactedPath(request,nil,"",redact.New())
{{ else -}}
    path := redactedPath(request,redact.New())
{{ end -}}
    if path != "-" { t.Fatalf("unmatched path: %s",path) }
}

func TestAccessLogRedactsRejectedChildRoute(t *testing.T) {
    var output bytes.Buffer
    previous := slog.Default()
    slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
    t.Cleanup(func() { slog.SetDefault(previous) })
{{ if eq .Computed.http_router_final "gin" -}}
    router := gin.New()
    router.Use(AccessLog(redact.New()), Recoverer())
    group := router.Group("/private")
    group.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
    group.GET("/reset/:token", func(*gin.Context) { t.Fatal("rejected handler ran") })
    wantPath := "/private/reset/sec***lue"
{{ else -}}
    router := chi.NewRouter()
    router.Use(AccessLog(redact.New()), Recoverer())
    child := chi.NewRouter()
    child.Get("/reset/{token}", func(http.ResponseWriter, *http.Request) { t.Fatal("rejected handler ran") })
    router.Mount("/private", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") == "" {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        child.ServeHTTP(w, r)
    }))
    wantPath := "-"
{{ end -}}
    response := httptest.NewRecorder()
    router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/private/reset/secret-token-value", nil))
    if response.Code != http.StatusUnauthorized || strings.Contains(output.String(), "secret-token-value") {
        t.Fatalf("response = %d, logs = %s", response.Code, output.String())
    }
    var entry map[string]json.RawMessage
    if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
    var path string
    if err := json.Unmarshal(entry["path"], &path); err != nil { t.Fatal(err) }
    if path != wantPath || string(entry["status"]) != "401" { t.Fatalf("log: %s", output.String()) }
}

func TestAccessLogAbortedRequest(t *testing.T) {
    for _, test := range []struct { name string; selectStatus, committed bool }{
        {"before_response", false, false},
{{ if eq .Computed.http_router_final "gin" -}}
        {"pending_status", true, false},
{{ end -}}
        {"after_response", true, true},
    } {
        t.Run(test.name, func(t *testing.T) {
            var output bytes.Buffer
            previous := slog.Default()
            slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
            t.Cleanup(func() { slog.SetDefault(previous) })
{{ if eq .Computed.http_router_final "gin" -}}
            router := gin.New()
            router.Use(AccessLog(redact.New()), Recoverer())
            router.GET("/abort", func(c *gin.Context) {
                if test.selectStatus { c.Status(http.StatusAccepted) }
                if test.committed { _, _ = c.Writer.Write([]byte("partial")) }
                panic(http.ErrAbortHandler)
            })
{{ else -}}
            router := chi.NewRouter()
            router.Use(AccessLog(redact.New()), Recoverer())
            router.Get("/abort", func(w http.ResponseWriter, _ *http.Request) {
                if test.committed {
                    w.WriteHeader(http.StatusAccepted)
                    _, _ = w.Write([]byte("partial"))
                }
                panic(http.ErrAbortHandler)
            })
{{ end -}}
            response := httptest.NewRecorder()
            response.Code = 0
            var propagated any
            func() {
                defer func() { propagated = recover() }()
                router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/abort", nil))
            }()
            if propagated != http.ErrAbortHandler { t.Fatalf("panic = %v", propagated) }
            wantStatus, wantBody := 0, ""
            if test.committed { wantStatus, wantBody = http.StatusAccepted, "partial" }
            if response.Code != wantStatus || response.Body.String() != wantBody {
                t.Fatalf("response = %d %q", response.Code, response.Body.String())
            }
            lines := strings.Split(strings.TrimSpace(output.String()), "\n")
            if len(lines) != 2 || strings.Contains(output.String(), "stack") { t.Fatalf("logs: %s", output.String()) }
            var entry map[string]json.RawMessage
            if err := json.Unmarshal([]byte(lines[1]), &entry); err != nil { t.Fatal(err) }
            if string(entry["status"]) != fmt.Sprint(wantStatus) || string(entry["outcome"]) != `"aborted"` || string(entry["level"]) != `"WARN"` {
                t.Fatalf("access log: %s", lines[1])
            }
            var duration int64
            if err := json.Unmarshal(entry["duration_ms"], &duration); err != nil { t.Fatal(err) }
            if duration < 0 { t.Fatalf("negative duration: %d", duration) }
            var message string
            if err := json.Unmarshal(entry["msg"], &message); err != nil { t.Fatal(err) }
            if want := fmt.Sprintf("GET %d %dms /abort /abort aborted", wantStatus, duration); message != want {
                t.Fatalf("message = %q, want %q", message, want)
            }
        })
    }
}

func TestAccessLogPropagatesUnhandledPanic(t *testing.T) {
{{ if eq .Computed.http_router_final "gin" -}}
    router := gin.New()
    router.Use(AccessLog(redact.New()))
    router.GET("/panic", func(*gin.Context) { panic("outside recoverer") })
{{ else -}}
    router := chi.NewRouter()
    router.Use(AccessLog(redact.New()))
    router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("outside recoverer") })
{{ end -}}
    defer func() {
        if got := recover(); got != "outside recoverer" { t.Fatalf("panic = %v", got) }
    }()
    router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
}
