package role

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "regexp"
 "testing"

 "github.com/DATA-DOG/go-sqlmock"
{{ if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{ end }}
 "{{ .Computed.module_name_final }}/internal/common/report"
 "{{ .Computed.module_name_final }}/internal/common/pagination"
 "{{ .Computed.module_name_final }}/internal/infra/db"
)

func TestListFieldProjection(t *testing.T) {
 database, mock, err := sqlmock.New()
 if err != nil { t.Fatal(err) }
 defer database.Close()
 // Nil association modules make accidental enrichment fail immediately.
 m := &Module{store: &db.Store{DB: database}, limits: pagination.Limits{MaxP:10000,MaxS:10000}}
 input := ListRolesInput{Fields: report.Fields{"name"}, }
 mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
 mock.ExpectQuery(regexp.QuoteMeta("SELECT name FROM t_role")).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow(""))
 value, err := m.List(t.Context(), input)
 if err != nil { t.Fatal(err) }
 if len(value.Items) != 1 { t.Fatalf("items: %+v", value) }
 encoded, err := json.Marshal(value.Items[0])
 if err != nil || string(encoded) != `{"name":""}` { t.Fatalf("projection: %s %v", encoded, err) }
 if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
 for _, fields := range []report.Fields{
 {}, {"unknown"}, {"name", "name"}} {
  input.Fields = fields
  if _, err := m.List(t.Context(), input); err == nil { t.Fatal("accepted invalid selector") }
 }
{{ if eq .Computed.http_router_final "gin" }}
 router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else }}
 handler := m.HTTP()
{{ end }}
 for _, query := range []string{"f=", "f=unknown", "f=name,name", "f=name,", "f=name&f=name", "f=Key"} {
  response := httptest.NewRecorder()
  handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,"/?"+query,nil))
  if response.Code != 400 { t.Fatalf("%s: %d %s",query,response.Code,response.Body.String()) }
 }
 // An empty selected page retains the normal pagination envelope.
 input.Fields = report.Fields{"name"}
 mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))

 value, err = m.List(t.Context(), input)
 if err != nil { t.Fatal(err) }
 encoded, err = json.Marshal(value)
 if err != nil || string(encoded) != `{"items":[],"t":0,"p":1,"s":1}` { t.Fatalf("empty page: %s %v",encoded,err) }
 if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}
