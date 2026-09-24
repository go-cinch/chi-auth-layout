package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"

{{- if .Computed.enable_grpc_final }}
"{{ .Computed.module_name_final }}/internal/docs"
"{{ .Computed.module_name_final }}/internal/modules"
"google.golang.org/grpc"
{{- end }}

	"{{ .Computed.module_name_final }}/internal/infra/db"

)

func TestNewRouter(t *testing.T) {
	application := &Application{

		authenticator: testAuthenticator(t),
		idempotencyKeys: idempotency.NewMemoryStore(),

		db: &db.Store{},

{{- if .Computed.enable_redis_final }}
		rds: nil,
{{- end }}
	}
	var cfg config.Config
	application.wireBusinessModules()
	
	cfg.Idempotency.TTL = time.Hour
	
	cfg.HTTP.Docs.Enabled = true
	handler, err := application.NewRouter(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := "/missing"
	want := http.StatusNotFound
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d", recorder.Code, want)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("docs status = %d", recorder.Code)
	}
}

func testAuthenticator(t *testing.T) *authn.Manager {
	t.Helper()
	authenticator, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

{{ if .Computed.enable_grpc_final -}}
type rpcOnlyModule struct{}

func (rpcOnlyModule) GRPC(grpc.ServiceRegistrar) {}

func TestConfigureOptionalTransports(t *testing.T) {
 for _,test:=range []struct{http,grpc bool}{
  {true,false},{false,true},{true,true},
 } {
  a:=&Application{

   authenticator:testAuthenticator(t),

   db:&db.Store{},

  }
  var httpModules []modules.HTTPModule
  var grpcModules []modules.GRPCModule
  if test.http {h,err:=docs.New(nil);if err!=nil {t.Fatal(err)};httpModules=append(httpModules,h)}
  if test.grpc {grpcModules=append(grpcModules,rpcOnlyModule{})}
  if err:=a.configureTransports(&config.Config{},httpModules,grpcModules);err!=nil {t.Fatal(err)}
  if (a.server!=nil)!=test.http || (a.grpcServer!=nil)!=test.grpc {t.Fatalf("servers: http=%v grpc=%v",a.server!=nil,a.grpcServer!=nil)}
  if a.grpcServer!=nil {a.grpcServer.Stop()}
 }
}
{{ end -}}
