package action

import (
 "{{ .Computed.module_name_final }}/internal/modules/localization"
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
 localizer := localization.New()
 database, mock, err := sqlmock.New()
 if err != nil { t.Fatal(err) }
 defer database.Close()
 // Nil association modules make accidental enrichment fail immediately.
 m := &Module{store: &db.Store{DB: database}, catalogSource: localizer, limits: pagination.Limits{MaxP:10000,MaxS:10000}}
 input := ListActionsInput{Fields: report.Fields{"resource"}, }
 mock.ExpectQuery("SELECT COUNT.*FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
 mock.ExpectQuery(regexp.QuoteMeta("SELECT word, config FROM t_action")).WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{"bindings":[],"ui":[],"endpoints":[]}`))
 value, err := m.List(t.Context(), input)
 if err != nil { t.Fatal(err) }
 if len(value.Items) != 1 { t.Fatalf("items: %+v", value) }
 encoded, err := json.Marshal(value.Items[0])
 if err != nil || string(encoded) != `{"resource":""}` { t.Fatalf("projection: %s %v", encoded, err) }
 if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
 for _, fields := range []report.Fields{
 {}, {"unknown"}, {"resource", "resource"}} {
  input.Fields = fields
  if _, err := m.List(t.Context(), input); err == nil { t.Fatal("accepted invalid selector") }
 }
{{ if eq .Computed.http_router_final "gin" }}
 router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else }}
 handler := m.HTTP()
{{ end }}
 for _, query := range []string{"f=", "f=unknown", "f=resource,resource", "f=resource,", "f=resource&f=resource", "f=Key"} {
  response := httptest.NewRecorder()
  handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,"/?"+query,nil))
  if response.Code != 400 { t.Fatalf("%s: %d %s",query,response.Code,response.Body.String()) }
 }
 // An empty selected page retains the normal pagination envelope.
 input.Fields = report.Fields{"resource"}
 mock.ExpectQuery("SELECT COUNT.*FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))

 value, err = m.List(t.Context(), input)
 if err != nil { t.Fatal(err) }
 encoded, err = json.Marshal(value)
 if err != nil || string(encoded) != `{"items":[],"t":0,"p":1,"s":1}` { t.Fatalf("empty page: %s %v",encoded,err) }
 if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestListFieldSelectionUsesCurrentCatalog(t *testing.T) {
 m,mock:=newTestModule(t)
 localizer:=m.catalogSource.(*localization.Module)
 word:="admin:read:system:user"
 stored,err:=ResolveConfig(localizer.Current(),word,Config{ResourceKey:"admin:system:user",Operation:"read",FeatureKeys:[]string{"admin:system:user:query"}})
 if err!=nil {t.Fatal(err)}
 rawStored,_:=json.Marshal(stored)
 catalog:=localizer.Current()
 feature:=catalog.ResourceCatalog.Buttons["admin:system:user:query"]
 feature.HTTP=[]localization.HTTPPath{ {Method:"GET",Path:"/new-user"}}
 feature.Btn=[]string{"admin:btn:system:user:current"}
 catalog.ResourceCatalog.Buttons["admin:system:user:query"]=feature
 node:=catalog.ResourceCatalog.Resources["admin:system:user"]
 entry:=node.Entries["admin:menu:system:user"];entry.Path="/current-user";node.Entries["admin:menu:system:user"]=entry
 catalog.ResourceCatalog.Resources["admin:system:user"]=node
 rawCatalog,_:=json.Marshal(catalog)
 if err:=localizer.Install(rawCatalog);err!=nil {t.Fatal(err)}
 mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(4)).WillReturnRows(sqlmock.NewRows([]string{"id","created_at","updated_at","code","action_group","word","scope","config"}).AddRow(4,actionTestTime,actionTestTime,"ABCDEFGH","admin:system:user",word,"admin",string(rawStored)))
 detail,err:=m.Get(t.Context(),4);if err!=nil {t.Fatal(err)}
 for _,field:=range []string{"config","resource","menu","button"} {
  mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
  mock.ExpectQuery("SELECT word, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word","config"}).AddRow(word,string(rawStored)))
  result,err:=m.List(t.Context(),ListActionsInput{Fields:report.Fields{field}});if err!=nil {t.Fatal(err)}
  expected,_:=json.Marshal(detail);actual,_:=json.Marshal(result.Items[0])
  var full,selected map[string]json.RawMessage
  if err:=json.Unmarshal(expected,&full);err!=nil {t.Fatal(err)}
  if err:=json.Unmarshal(actual,&selected);err!=nil {t.Fatal(err)}
  if len(selected)!=1 || string(full[field])!=string(selected[field]) {t.Fatalf("%s differs: detail=%s selected=%s",field,full[field],actual)}
 }
}

type unneededCatalogSource struct{}

func (unneededCatalogSource) Select([]string) localization.Catalog { panic("unselected catalog was loaded") }

func (unneededCatalogSource) Current() localization.Catalog { panic("unselected catalog was loaded") }

func TestIdentityProjectionSkipsCatalogAndMappingDependencies(t *testing.T) {
 m,mock:=newTestModule(t)
 m.catalogSource=unneededCatalogSource{}
 mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
 mock.ExpectQuery("SELECT id FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
 result,err:=m.List(t.Context(),ListActionsInput{Fields:report.Fields{"id"}})
 if err!=nil {t.Fatal(err)}
 raw,err:=json.Marshal(result.Items[0]);if err!=nil || string(raw)!=`{"id":4}` {t.Fatal(string(raw),err)}
}
