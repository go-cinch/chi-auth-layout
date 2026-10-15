package usergroup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestUserGroupHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/user-group" { t.Fatal(m.Name()) }
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(method, target, body string, want int) string {
		t.Helper(); recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
		if recorder.Code != want { t.Fatalf("%s %s: got %d body %s", method, target, recorder.Code, recorder.Body.String()) }
		return recorder.Body.String()
	}
	request(http.MethodGet, "/bad", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	expectGroupGet(mock, 1, "Read Only", "readonly")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_user_group").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WillReturnResult(sqlmock.NewResult(0, 0))
	expectGroupGet(mock, 4, "Auditors", "auditor")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"name":"Auditors","word":"auditor","action_codes":[],"user_ids":[]}`, http.StatusOK)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group SET name").WillReturnResult(sqlmock.NewResult(0, 1))
	expectGroupGet(mock, 1, "Updated", "readonly")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"name":"Updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=1&s=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?broken=%zz", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user_group").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user_group").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	request(http.MethodPost, "/", `{"name":"Missing","word":"missing","user_ids":[99]}`, http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	request(http.MethodPatch, "/99", `{"name":"Missing"}`, http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}

func TestMemberAndSelectionHTTP(t *testing.T) {
 m,mock:=newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
 router:=gin.New();m.HTTP(router.Group(""));handler:=http.Handler(router)
{{ else -}}
 handler:=m.HTTP()
{{ end -}}
 request:=func(method,target,body string,want int) string {
  t.Helper();w:=httptest.NewRecorder();handler.ServeHTTP(w,httptest.NewRequest(method,target,strings.NewReader(body)))
  if w.Code!=want{t.Fatalf("%s %s: %d %s",method,target,w.Code,w.Body.String())};return w.Body.String()
 }
 for _,path:=range []string{"/","/1"} {
  for _,q:=range []string{"f=","f=unknown","f=users,users","f=users,,actions","f=users&f=actions","f=%zz"} {request("GET",path+"?"+q,"",400)}
 }
 for _,method:=range []string{"POST","PATCH"}{
  path:="/";if method=="PATCH"{path="/1"}
  for _,q:=range []string{"f=unknown","f=%zz"}{request(method,path+"?"+q,`{}`,400)}
 }
 for _,target:=range []string{"/bad/member","/0/member","/1/member?q=a&q=b","/1/member?p=","/1/member?p=1&p=2","/1/member?p=2147483648","/1/member?s=x","/1/member?s=1&s=2","/1/member?x=%zz"}{request("GET",target,"",400)}
 request("GET","/1/member?p=0","",200)
 request("GET","/1/member?s=-1","",200)
 mock.ExpectQuery("SELECT id FROM").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
 mock.ExpectQuery("SELECT u.id").WillReturnRows(sqlmock.NewRows([]string{"id","username","code"}).AddRow(3,"reader","USER0003"))
 body:=request("GET","/1/member?p=1&s=20&q=reader","",200)
 if !strings.Contains(body,`"t":1`)||!strings.Contains(body,`"username":"reader"`){t.Fatal(body)}
 mock.ExpectQuery("SELECT id FROM").WillReturnRows(sqlmock.NewRows([]string{"id"}))
 request("GET","/99/member","",404)
 mock.ExpectQuery("SELECT id FROM").WillReturnError(sqlmock.ErrCancelled)
 request("GET","/1/member","",500)
 mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WillReturnRows(groupRows(1,"Group","group"))
 mock.ExpectQuery("SELECT g.id").WillReturnRows(sqlmock.NewRows([]string{"id","count"}).AddRow(1,1000))
 body=request("GET","/1?f=member_count","",200)
 if strings.Contains(body,`"users"`)||strings.Contains(body,`"actions"`)||!strings.Contains(body,`"member_count":1000`){t.Fatal(body)}
 mock.ExpectBegin()
 mock.ExpectExec("UPDATE t_user_group SET updated_at").WillReturnResult(sqlmock.NewResult(0,1))
 mock.ExpectExec("DELETE FROM t_user_user_group_relation WHERE user_group_id = .* AND user_id = ANY").WillReturnResult(sqlmock.NewResult(0,1))
 mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WillReturnRows(groupRows(1,"Group","group"))
 mock.ExpectQuery("SELECT g.id").WillReturnRows(sqlmock.NewRows([]string{"id","count"}).AddRow(1,999))
 mock.ExpectCommit()
 body=request("PATCH","/1?f=member_count",`{"remove_user_ids":[3]}`,200)
 if strings.Contains(body,`"users"`)||!strings.Contains(body,`"member_count":999`){t.Fatal(body)}
}
