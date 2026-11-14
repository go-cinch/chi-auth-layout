package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- else }}
	"github.com/go-chi/chi/v5"
{{- end }}
)

type authenticationFailureValidator struct{ err error }

func (v authenticationFailureValidator) ValidateIdentity(context.Context, authn.Identity) error {
	return v.err
}

func TestHTTPAuthenticationFailureLogging(t *testing.T) {
	failure := errors.New("authentication storage unavailable")
	manager, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour, authenticationFailureValidator{failure})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewRouter(&config.Config{}, manager, nil, protectedTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error_code":"HTTP_INTERNAL","msg":"internal server error"}` {
		t.Fatalf("authentication failure response = %d %s", response.Code, response.Body.String())
	}
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if record["operation"] != "authenticate_http_request" {
			continue
		}
		found = true
		if record["level"] != "ERROR" || record["outcome"] != "failure" || record["method"] != "GET" || record["error"] != failure.Error() || record["msg"] != "authenticate http request failed: "+failure.Error() {
			t.Fatalf("authentication failure log = %#v", record)
		}
	}
	if !found {
		t.Fatal("authentication storage failure was not logged")
	}
	if bytes.Contains(logs.Bytes(), []byte(token)) {
		t.Fatal("authentication log contains the bearer token")
	}
}

// guardOrderModule observes the full router chain, including downstream effects.
type guardOrderModule struct {
	events        *[]string
	allowed       bool
	permissionErr error
}

func (*guardOrderModule) Name() string { return "/guard" }

func (*guardOrderModule) PermissionEndpoint() string { return "/permission" }

func (*guardOrderModule) Idempotent() bool { return true }

func (m *guardOrderModule) AuthorizeHTTP(ctx context.Context, _, _ string) (bool, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID != 7 || authn.PermissionScope(ctx) != "admin" {
		return false, errors.New("authorization lost identity or scope")
	}
	*m.events = append(*m.events, "authorize")
	return m.allowed, m.permissionErr
}

func (m *guardOrderModule) handle(w http.ResponseWriter, _ *http.Request) {
	*m.events = append(*m.events, "handler")
	w.WriteHeader(http.StatusNoContent)
}
{{ if eq .Computed.http_router_final "gin" -}}
func (m *guardOrderModule) HTTP(r *gin.RouterGroup) { r.POST("", gin.WrapF(m.handle)) }
{{ else -}}
func (m *guardOrderModule) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Post("/", m.handle)
	return r
}
{{ end -}}
type guardOrderValidator struct{ events *[]string }

func (v guardOrderValidator) ValidateIdentity(context.Context, authn.Identity) error {
	*v.events = append(*v.events, "identity")
	return nil
}

type guardOrderStore struct{ events *[]string }

func (s guardOrderStore) Claim(context.Context, string, time.Duration) (bool, error) {
	*s.events = append(*s.events, "claim")
	return true, nil
}

func TestAuthenticationGuardsPrecedeSideEffects(t *testing.T) {
	for _, tc := range []struct {
		name          string
		token         bool
		scope         string
		allowed       bool
		permissionErr error
		securityErr   error
		status        int
		want          []string
	}{
		{name: "missing token", status: 401},
		{name: "invalid scope", token: true, scope: "invalid scope", status: 400, want: []string{"identity"}},
		{name: "permission denied", token: true, status: 403, want: []string{"identity", "authorize"}},
		{name: "permission unavailable", token: true, permissionErr: errors.New("storage unavailable"), status: 500, want: []string{"identity", "authorize"}},
		{name: "Verification required", token: true, allowed: true, securityErr: apperror.New("AUTH_STEP_UP_REQUIRED", "verify"), status: 403, want: []string{"identity", "authorize", "security"}},
		{name: "allowed", token: true, allowed: true, status: 204, want: []string{"identity", "authorize", "security", "claim", "handler"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			manager, err := authn.New("test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour, guardOrderValidator{&events})
			if err != nil {
				t.Fatal(err)
			}
			token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1", CredentialVersion: 1})
			if err != nil {
				t.Fatal(err)
			}
			ctx, audit := auditctx.WithState(t.Context())
			manager.SetHTTPSecurityPolicy(func(r *http.Request) error {
				if audit.Snapshot().UserID != 7 {
					t.Fatal("security guard ran before audit identity")
				}
				events = append(events, "security")
				return tc.securityErr
			})
			cfg := &config.Config{}
			cfg.Auth.Authorization.Enabled = true
			cfg.Idempotency.TTL = time.Minute
			module := &guardOrderModule{events: &events, allowed: tc.allowed, permissionErr: tc.permissionErr}
			handler, err := NewRouter(cfg, manager, guardOrderStore{&events}, module)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/guard", nil).WithContext(ctx)
			request.Header.Set(IdempotencyHeader, "request-1")
			if tc.token {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			if tc.scope != "" {
				request.Header.Set(authn.PermissionScopeHeader, tc.scope)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || !reflect.DeepEqual(events, tc.want) {
				t.Fatalf("response = %d %s; steps = %v, want status %d steps %v", response.Code, response.Body.String(), events, tc.status, tc.want)
			}
		})
	}
}
