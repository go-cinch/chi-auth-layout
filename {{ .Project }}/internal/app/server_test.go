package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

{{- if .Computed.enable_grpc_final }}
	"google.golang.org/grpc"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
{{- if .Computed.enable_grpc_final }}
	"{{ .Computed.module_name_final }}/internal/docs"
	"{{ .Computed.module_name_final }}/internal/modules"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/infra/db"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestNewRouter(t *testing.T) {
	application := &Application{
		localization: localization.New(),
		verification:          &authmodule.Verification{},

		authenticator:   testAuthenticator(t),
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
	for _, test := range []struct{ http, grpc bool }{
		{true, false}, {false, true}, {true, true},
	} {
		a := &Application{

			authenticator: testAuthenticator(t),

			db: &db.Store{},
		}
		var httpModules []modules.HTTPModule
		var grpcModules []modules.GRPCModule
		if test.http {
			h, err := docs.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			httpModules = append(httpModules, h)
		}
		if test.grpc {
			grpcModules = append(grpcModules, rpcOnlyModule{})
		}
		if err := a.configureTransports(&config.Config{}, httpModules, grpcModules); err != nil {
			t.Fatal(err)
		}
		if (a.server != nil) != test.http || (a.grpcServer != nil) != test.grpc {
			t.Fatalf("servers: http=%v grpc=%v", a.server != nil, a.grpcServer != nil)
		}
		if a.grpcServer != nil {
			a.grpcServer.Stop()
		}
	}
}

{{ end -}}
func TestBusinessHTTPAlwaysRequiresAuthentication(t *testing.T) {
	a := &Application{
		localization: localization.New(),
		verification:          &authmodule.Verification{},

		db:              &db.Store{},
		authenticator:   testAuthenticator(t),
		idempotencyKeys: idempotency.NewMemoryStore(),
	}
	a.auditSettings = config.DefaultAudit()
	a.wireBusinessModules()
	httpModules {{ if .Computed.enable_grpc_final }}, _ {{ end }} := a.generatedModules()
	cfg := &config.Config{}
	cfg.HTTP.Addr = "127.0.0.1:0"
	cfg.HTTP.Profiler.Enabled = true
	cfg.HTTP.Profiler.Addr = "127.0.0.1:0"
	if err := a.configureTransports(cfg, httpModules {{ if .Computed.enable_grpc_final }}, nil {{ end }}); err == nil || !strings.Contains(err.Error(), "initialize http router: idempotency store and positive ttl are required") {
		t.Fatalf("invalid idempotency configuration error = %v", err)
	}
	if a.server != nil || a.profilerServer != nil {
		t.Fatal("servers configured after failed module registration")
	}
	cfg.Idempotency.TTL = time.Minute
	if err := a.configureTransports(cfg, httpModules {{ if .Computed.enable_grpc_final }}, nil {{ end }}); err != nil {
		t.Fatal(err)
	}
	if a.server == nil {
		t.Fatal("required business HTTP server was not configured")
	}
	if a.profilerServer == nil || a.profilerServer.Addr != cfg.HTTP.Profiler.Addr {
		t.Fatal("enabled profiler server was not configured")
	}
	w := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/user/7", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated user route = %d, want 401", w.Code)
	}
}
