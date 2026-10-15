package msg

import (
 "errors"
 "encoding/json"
 "regexp"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{- end }}
 "{{ .Computed.module_name_final }}/internal/infra/db"
)

func newSummaryTestModule(t *testing.T) (*Module,sqlmock.Sqlmock) {
 t.Helper()
 database,mock,err := sqlmock.New()
 if err != nil { t.Fatal(err) }
 t.Cleanup(func() { if err := mock.ExpectationsWereMet(); err != nil { t.Error(err) }; _ = database.Close() })
 return &Module{store:&db.Store{DB:database}},mock
}

func TestManagementSummary(t *testing.T) {
 m,mock := newSummaryTestModule(t)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "notice", "system", "all", "targeted", "latest_published_at"}).AddRow(int64(3), int64(1), int64(1), int64(1), int64(1), int64(1)))
 value,err := m.SentSummary(t.Context())
 if err != nil || value.Total != 3 || len(value.Counts) != 5 || value.Counts["notice"] != 1 || value.Trend == nil || value.Groups == nil { t.Fatalf("summary: %#v %v",value,err) }
 mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
 if _,err := m.SentSummary(t.Context()); !errors.Is(err,sqlmock.ErrCancelled) { t.Fatalf("query error: %v",err) }
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("invalid"))
 if _,err := m.SentSummary(t.Context()); err == nil { t.Fatal("accepted malformed SQL result") }
}

func TestManagementSummaryHTTP(t *testing.T) {
 m,mock := newSummaryTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
 router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else -}}
 handler := m.HTTP()
{{ end -}}
 request := func(target string,want int) string {
  t.Helper(); w := httptest.NewRecorder(); handler.ServeHTTP(w,httptest.NewRequest(http.MethodGet,target,nil))
  if w.Code != want { t.Fatalf("%s: %d %s",target,w.Code,w.Body.String()) }
  return w.Body.String()
 }
 request("/sent/summary?q=x",http.StatusBadRequest)
 for _,q:=range []string{"f=","f=total&f=counts","f=unknown","f=total,total"}{request("/sent/summary?"+q,http.StatusBadRequest)}
 request("/sent/summary?q=%zz",http.StatusBadRequest)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "notice", "system", "all", "targeted", "latest_published_at"}).AddRow(int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)))
 body := request("/sent/summary",http.StatusOK)
 if !strings.Contains(body,`"total":0`) || !strings.Contains(body,`"groups":[]`) || !strings.Contains(body,`"trend":[]`) { t.Fatal(body) }
 mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
 request("/sent/summary",http.StatusInternalServerError)
}

func TestSentMessageQuery(t *testing.T) {
 m,mock:=testModule(t)
 mock.ExpectQuery("SELECT COUNT.*m.title ILIKE.*OR m.content ILIKE").WithArgs("%100!%!_!!%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
 if value,err:=m.ListSent(t.Context(),ListInput{Query:" 100%_! "});err!=nil||value.Total!=0{t.Fatalf("%#v %v",value,err)}
{{ if eq .Computed.http_router_final "gin" -}}
 router:=gin.New();m.HTTP(router.Group(""));handler:=http.Handler(router)
{{ else -}}
 handler:=m.HTTP()
{{ end -}}
 w:=httptest.NewRecorder();handler.ServeHTTP(w,httptest.NewRequest("GET","/sent?q=one&q=two",nil));if w.Code!=400{t.Fatal(w.Code)}
 mock.ExpectQuery("SELECT COUNT.*m.title ILIKE").WithArgs("%upgrade%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
 w=httptest.NewRecorder();handler.ServeHTTP(w,httptest.NewRequest("GET","/sent?q=upgrade",nil));if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
}

func TestSelectedSummaryTotalOnly(t *testing.T) {
 m,mock:=newSummaryTestModule(t)
 mock.ExpectQuery("^"+regexp.QuoteMeta("SELECT COUNT(*) FROM t_msg")+"$").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
 value,err:=m.SentSummary(t.Context(),"total")
 if err!=nil{t.Fatal(err)}
 data,err:=json.Marshal(value)
 if err!=nil || string(data)!=`{"total":0}` {t.Fatal(string(data),err)}
 if _,err:=m.SentSummary(t.Context(),"unknown");err==nil{t.Fatal("accepted unknown field")}
}
