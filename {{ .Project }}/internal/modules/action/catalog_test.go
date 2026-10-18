package action

import (
 "{{ .Computed.module_name_final }}/internal/modules/localization"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
	{{ end }}
	"{{ .Computed.module_name_final }}/internal/common/authn"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

type catalogPolicy struct {
	actionPolicy
	calls   int
	failure error
}

func (p *catalogPolicy) PermissionPaths(_ context.Context, scope string, _ report.Fields) (PermissionPaths, error) {
	p.calls++
	return PermissionPaths{Btn: []string{scope + ":btn:system:user:query"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user"}}, GRPC: []string{}}, p.failure
}

func TestCatalogFieldSelectionAndEffectivePermissions(t *testing.T) {
	m, mock := newTestModule(t)
	policy := &catalogPolicy{actionPolicy: actionPolicy{allowed: true}}
	m.SetPermissionAdministration(policy)
	value, err := m.Catalog(t.Context(), report.Fields{"scopes", "operations"})
	if err != nil || value.Scopes == nil || value.Operations == nil || value.ResourceCatalog.Resources != nil {
		t.Fatalf("%v %v", value, err)
	}
	if !reflect.DeepEqual(value.Operations, []string{"read", "create", "update", "delete"}) {
		t.Fatal(value)
	}
	value, err = m.Catalog(t.Context(), report.Fields{"resource_catalog"})
	if err != nil || value.ResourceCatalog.Resources == nil {
		t.Fatal(value, err)
	}
	mock.ExpectQuery("SELECT id, word FROM t_action ORDER BY id").WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(4, "admin:read:system:user"))
	value, err = m.Catalog(t.Context(), report.Fields{"defined_actions"})
	if err != nil || value.DefinedActions == nil {
		t.Fatal(value, err)
	}
	effective, err := m.Effective(t.Context(), "admin", report.Fields{"http"})
	if err != nil || effective.Scope != "admin" || effective.Btn != nil || effective.HTTP == nil {
		t.Fatal(value, err)
	}
	calls := policy.calls
	for _, fields := range []report.Fields{ {}, {"unknown"}, {"btn", "btn"}} {
		if _, err := m.Effective(t.Context(), "admin", fields); err == nil {
			t.Fatal(fields)
		}
	}
	if _, err := m.Effective(t.Context(), "h5", report.Fields{"btn"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if calls != policy.calls {
		t.Fatal("invalid requests loaded permissions")
	}
	policy.failure = sqlmock.ErrCancelled
	if _, err := m.Effective(t.Context(), "admin", report.Fields{"btn"}); !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatal(err)
	}
	policy.allowed = false
	if _, err := m.Catalog(t.Context(), report.Fields{"scopes"}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatal(err)
	}
}

func TestCatalogHTTP(t *testing.T) {
	m, _ := newTestModule(t)
	policy := &catalogPolicy{actionPolicy: actionPolicy{allowed: true}}
	m.SetPermissionAdministration(policy)
	{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
	{{ else }}
	handler := m.HTTP()
	{{ end }}
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/catalog", 200}, {"/catalog?f=resource_catalog", 200}, {"/effective?scope=admin&f=btn,http,grpc", 200},
		{"/catalog?f=", 400}, {"/catalog?f=unknown", 400}, {"/catalog?f=scopes,scopes", 400}, {"/catalog?f=scopes&f=operations", 400}, {"/catalog?x=%zz", 400},
		{"/effective", 400}, {"/effective?scope=admin&scope=mini", 400}, {"/effective?scope=unknown", 400}, {"/effective?scope=admin&f=", 400}, {"/effective?scope=admin&f=Btn", 400},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s: %d %s", test.path, response.Code, response.Body.String())
		}
		if test.path == "/catalog" {
			var v map[string]any
			_ = json.Unmarshal(response.Body.Bytes(), &v)
			if len(v) != 2 || v["resource_catalog"] != nil {
				t.Fatal(v)
			}
		}
	}
	policy.allowed = false
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	if response.Code != 403 {
		t.Fatal(response.Code)
	}
}

func TestEffectiveFullSelectionPreservesEmptyCollections(t *testing.T) {
 value:=EffectiveResult{Scope:"admin",Btn:[]string{},HTTP:[]localization.HTTPPath{},GRPC:[]string{}}
 raw,err:=json.Marshal(value)
 if err!=nil {t.Fatal(err)}
 var fields map[string]json.RawMessage
 if err:=json.Unmarshal(raw,&fields);err!=nil {t.Fatal(err)}
 if len(fields)!=4 || string(fields["scope"])!=`"admin"` {t.Fatal(string(raw))}
 for _,name:=range []string{"btn","http","grpc"} {if string(fields[name])!="[]" {t.Fatal(string(raw))}}
}

type narrowCatalogSource struct { source *localization.Module; fields report.Fields }

func (s *narrowCatalogSource) Current() localization.Catalog { panic("full catalog requested") }

func (s *narrowCatalogSource) Select(fields []string) localization.Catalog {
 s.fields = append(report.Fields{},fields...)
 return s.source.Select(fields)
}

func TestCatalogLoadsOnlySelectedMetadata(t *testing.T) {
 m,_:=newTestModule(t)
 source:=&narrowCatalogSource{source:m.catalogSource.(*localization.Module)}
 m.catalogSource=source
 value,err:=m.Catalog(t.Context(),report.Fields{"scopes"})
 if err!=nil || len(value.Scopes)==0 || !reflect.DeepEqual(source.fields,report.Fields{"scopes"}) {t.Fatal(value,source.fields,err)}
}
