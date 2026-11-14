package mailtemplate

import (
 "database/sql"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{- end }}
)

func testRouter(m *Module) http.Handler {
{{- if eq .Computed.http_router_final "gin" }}
 gin.SetMode(gin.TestMode);r:=gin.New();m.HTTP(r.Group(""));return r
{{- else }}
 return m.HTTP()
{{- end }}
}

func TestHTTPValidationAndErrors(t *testing.T) {
 m,mock:=testModule(t);h:=testRouter(m)
 if m.Name()!="/mail-template"||!m.Idempotent(){t.Fatal("module contract")}
 cases:=[]struct{method,path,body string;status int}{
  {"GET","/bad","",400},{"GET","/0","",400},{"GET","/1/version/bad","",400},
  {"GET","/?p=","",400},{"GET","/?p=1&p=2","",400},{"GET","/?s=no","",400},{"GET","/?q=a&q=b","",400},{"GET","/?category=a&category=b","",400},{"GET","/?q=%zz","",400},
  {"POST","/","null",400},{"POST","/","{}",400},{"PATCH","/1","{",400},{"POST","/1/publish","{",400},
  {"PATCH","/bad","{}",400},{"POST","/bad/publish","{}",400},{"DELETE","/bad","",400},{"GET","/bad/version","",400},{"GET","/1/missing","",404},
 }
 for _,c:=range cases {t.Run(c.method+c.path,func(t *testing.T){w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest(c.method,c.path,strings.NewReader(c.body)));if w.Code!=c.status{t.Fatalf("%d %s",w.Code,w.Body.String())}})}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnError(sql.ErrNoRows);w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","/1",nil));if w.Code!=404{t.Fatal(w.Code)}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnError(sql.ErrConnDone);w=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","/1",nil));if w.Code!=500{t.Fatal(w.Code)}
 for _,c:=range []struct{err error;code int}{ {ErrConflict,409},{ErrDuplicate,409},{ErrInvalid,400},{ErrNotFound,404},{sql.ErrConnDone,500}}{w=httptest.NewRecorder();writeFailure(w,httptest.NewRequest("GET","/",nil),c.err);if w.Code!=c.code{t.Fatal(w.Code)}}
}

func TestHTTPSuccess(t *testing.T) {
 m,mock:=testModule(t);h:=testRouter(m)
 call:=func(method,path,body string){t.Helper();w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest(method,path,strings.NewReader(body)));if w.Code!=200{t.Fatalf("%s %s: %d %s",method,path,w.Code,w.Body.String())}}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnRows(recordRow(true,1,0,true));call("GET","/1","")
 mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0));mock.ExpectQuery("SELECT id, created_at").WillReturnRows(recordRow(false,1,0,true));call("GET","/?p=1&s=20&q=x&category=notification","")
 raw,_:=json.Marshal(CreateInput{TemplateID:"test",Name:"Test",Category:"notification",Content:sampleContent()});mock.ExpectBegin();mock.ExpectQuery("INSERT INTO t_mail_template").WillReturnRows(recordRow(true,1,0,true));mock.ExpectCommit();call("POST","/",string(raw))
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WillReturnRows(recordRow(true,1,0,true));mock.ExpectQuery("UPDATE t_mail_template SET name").WillReturnRows(recordRow(true,2,0,true));mock.ExpectCommit();call("PATCH","/1",`{"revision":1,"name":"Other"}`)
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WillReturnRows(recordRow(true,2,0,true));mock.ExpectExec("INSERT INTO t_mail_template_version").WillReturnResult(sqlmock.NewResult(1,1));mock.ExpectQuery("UPDATE t_mail_template SET published_version").WillReturnRows(recordRow(true,3,1,false));mock.ExpectCommit();call("POST","/1/publish",`{"revision":2}`)
 mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true));mock.ExpectQuery("SELECT id,created_at,version").WillReturnRows(sqlmock.NewRows([]string{"id","created_at","version","name","category","published_by"}));call("GET","/1/version","")
 data,_:=json.Marshal(sampleContent());mock.ExpectQuery("SELECT id,created_at,version").WillReturnRows(sqlmock.NewRows([]string{"id","created_at","version","name","category","published_by","content"}).AddRow(1,100,1,"Test","notification",0,data));call("GET","/1/version/1","")
 mock.ExpectBegin();mock.ExpectExec("DELETE FROM t_mail_template").WillReturnResult(sqlmock.NewResult(0,1));mock.ExpectCommit();call("DELETE","/1","")
}

func TestHTTPTestSend(t *testing.T) {
 for _,scenario:=range []struct{name string;databaseErr error;sender *testSender;status int}{
  {"sent",nil,&testSender{ready:true},200},
  {"missing",sql.ErrNoRows,&testSender{ready:true},404},
  {"database unavailable",sql.ErrConnDone,&testSender{ready:true},500},
  {"smtp disabled",nil,&testSender{},503},
  {"relay rejected",nil,&testSender{ready:true,err:sql.ErrConnDone},500},
 } {t.Run(scenario.name,func(t *testing.T){
  m,mock:=testModule(t);m.SetTestSender(scenario.sender)
  query:=mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id")
  if scenario.databaseErr!=nil {query.WillReturnError(scenario.databaseErr)} else {query.WillReturnRows(recordRow(true,1,0,true));expectRecipient(mock,"test@example.com")}
  raw,_:=json.Marshal(testInput());w:=httptest.NewRecorder()
  testRouter(m).ServeHTTP(w,testRequest(t,httptest.NewRequest("POST","/1/test-send",strings.NewReader(string(raw)))))
  if w.Code!=scenario.status {t.Fatalf("got %d want %d",w.Code,scenario.status)}
 })}
 for _,request:=range []struct{path,body string}{ {"/bad/test-send","{}"},{"/1/test-send","{"},{"/1/test-send","{}"}} {
  m,_:=testModule(t);w:=httptest.NewRecorder();testRouter(m).ServeHTTP(w,httptest.NewRequest("POST",request.path,strings.NewReader(request.body)))
  if w.Code!=400 {t.Fatal(w.Code)}
 }
}

func TestHTTPTestSendRejectsRecipientOverride(t *testing.T) {
 m, _ := testModule(t); sender := &testSender{ready:true}; m.SetTestSender(sender)
 raw,_ := json.Marshal(testInput())
 body := strings.TrimSuffix(string(raw), "}") + `,"recipient":"victim@example.com"}`
 w := httptest.NewRecorder()
 testRouter(m).ServeHTTP(w,testRequest(t,httptest.NewRequest("POST","/1/test-send",strings.NewReader(body))))
 if w.Code != 400 || sender.calls != 0 { t.Fatalf("client recipient accepted: %d", w.Code) }
}
