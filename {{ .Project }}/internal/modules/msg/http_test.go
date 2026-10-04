package msg

import (
{{- if eq .Computed.http_router_final "gin" }}
 "net/http"
{{- end }}
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{- end }}
 "{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestHTTP(t *testing.T) {
 m,mock:=testModule(t)
 if m.Name()!="/msg" || !m.Idempotent() { t.Fatal("module contract") }
{{ if eq .Computed.http_router_final "gin" -}}
 router:=gin.New();m.HTTP(router.Group(""));handler:=http.Handler(router)
{{ else -}}
 handler:=m.HTTP()
{{ end }}
 manager,_:=authn.New("test",strings.Repeat("k",64),time.Hour)
 token,_,err:=manager.Issue(authn.Identity{UserID:2,Username:"guest",Code:"12345678",CredentialVersion:1});if err!=nil { t.Fatal(err) }
 request:=func(method,path,body string,want int,authenticated bool) {
  t.Helper();r:=httptest.NewRequest(method,path,strings.NewReader(body));w:=httptest.NewRecorder()
  if authenticated { r.Header.Set("Authorization","Bearer "+token);var err error;r,err=manager.AuthenticateRequest(r);if err!=nil { t.Fatal(err) } }
  handler.ServeHTTP(w,r)
  if w.Code!=want { t.Fatalf("%s %s: %d %s",method,path,w.Code,w.Body.String()) }
 }
 for _,route:=range []struct{method,path string}{ {"GET","/inbox"},{"GET","/inbox/1"},{"PATCH","/inbox/1"},{"DELETE","/inbox/1"},{"POST","/inbox/read-all"},{"GET","/unread-count"},{"POST","/"}} {request(route.method,route.path,"",401,false)}
 request("GET","/missing/route","",404,true)
 for _,path:=range []string{"/inbox/bad","/sent/0"} {request("GET",path,"",400,true)}
 request("PATCH","/inbox/bad","{}",400,true);request("DELETE","/inbox/bad","",400,true);request("DELETE","/sent/bad","",400,true)
 for _,body:=range []string{`{`, `{} {}`, `{"unknown":1}`, `{}`, `{"read":false}`} {request("PATCH","/inbox/1",body,400,true)}
 request("POST","/","{",400,true);request("POST","/","{}",400,true)
 for _,query:=range []string{"p=","p=1&p=2","p=x","p=2147483648","s=x","read=x","type=","bad=%zz"} {request("GET","/inbox?"+query,"",400,true)}
 request("GET","/recipient-option?q=a&q=b","",400,true)
 mock.ExpectQuery("SELECT .* FROM").WillReturnRows(messageRows("all"));request("GET","/inbox/1","",200,true)
 mock.ExpectQuery("SELECT .* FROM").WillReturnRows(messageRows("all"));request("GET","/sent/1","",200,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0));request("GET","/inbox?p=1&s=10&read=false&type=system&scope=all","",200,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0));request("GET","/sent","",200,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0));request("GET","/sent?read=true","",200,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled);request("GET","/sent","",500,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2));request("GET","/unread-count","",200,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0,1));request("PATCH","/inbox/1",`{"read":true}`,200,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0,1));request("DELETE","/inbox/1","",200,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0,1));request("POST","/inbox/read-all","",200,true)
 mock.ExpectExec("DELETE FROM t_msg").WillReturnResult(sqlmock.NewResult(0,0));request("DELETE","/sent/999","",404,true)
 mock.ExpectExec("DELETE FROM t_msg").WillReturnResult(sqlmock.NewResult(0,1));request("DELETE","/sent/1","",200,true)
 mock.ExpectQuery("SELECT id, username").WillReturnRows(sqlmock.NewRows([]string{"id","username"}).AddRow(2,"guest"));request("GET","/recipient-option?q=guest","",200,true)
 mock.ExpectBegin();mock.ExpectQuery("INSERT INTO t_msg").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1));mock.ExpectQuery("SELECT .* FROM").WillReturnRows(messageRows("all"));mock.ExpectCommit()
 request("POST","/",`{"title":"hello","content":"world","type":"system","scope":"all"}`,200,true)
 for _,path:=range []string{"/inbox/1","/sent/1"} {
  mock.ExpectQuery("SELECT .* FROM").WillReturnError(sqlmock.ErrCancelled);request("GET",path,"",500,true)
 }
 mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled);request("GET","/inbox","",500,true)
 mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled);request("GET","/unread-count","",500,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnError(sqlmock.ErrCancelled);request("PATCH","/inbox/1",`{"read":true}`,500,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnError(sqlmock.ErrCancelled);request("DELETE","/inbox/1","",500,true)
 mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnError(sqlmock.ErrCancelled);request("POST","/inbox/read-all","",500,true)
 mock.ExpectExec("DELETE FROM t_msg").WillReturnError(sqlmock.ErrCancelled);request("DELETE","/sent/1","",500,true)
 mock.ExpectQuery("SELECT id, username").WillReturnError(sqlmock.ErrCancelled);request("GET","/recipient-option","",500,true)
 mock.ExpectBegin();mock.ExpectQuery("INSERT INTO t_msg").WillReturnError(sqlmock.ErrCancelled);mock.ExpectRollback()
 request("POST","/",`{"title":"hello","content":"world","type":"system","scope":"all"}`,500,true)

}
