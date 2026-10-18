package modules

import (
 "{{ .Computed.module_name_final }}/internal/common/i18n"
	"context"
 "{{ .Computed.module_name_final }}/internal/common/auditctx"
	"net/http"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
{{- if .Computed.enable_grpc_final }}
	"google.golang.org/grpc"
{{- end }}
)

type HTTPModule interface {
 Name() string
{{- if eq .Computed.http_router_final "gin" }}
 HTTP(*gin.RouterGroup)
{{- else }}
 HTTP() http.Handler
{{- end }}
}

// Module aliases HTTPModule.
type Module = HTTPModule

// IdempotentHTTPModule opts a module's POST handlers into idempotency-key
// protection. Other HTTP methods pass through unchanged.
type IdempotentHTTPModule interface {
	HTTPModule
	Idempotent() bool
}

// PublicHTTPModule marks infrastructure modules that are public in their
// entirety. Business modules remain authenticated by default and expose any
// unauthenticated handlers below their module-local /pub prefix.
type PublicHTTPModule interface {
	HTTPModule
	Public() bool
}

// HTTPPermissionAuthorizer supplies resource authorization for every protected
// HTTP module. PermissionEndpoint is authenticated but skips authorization so
// gateways can call it without recursively authorizing the check itself.
type HTTPPermissionAuthorizer interface {
	HTTPModule
	AuthorizeHTTP(context.Context, string, string) (bool, error)
	PermissionEndpoint() string
}

// HTTPPermissionRouteCatalog receives registered routes before serving requests.
// Authorizers use this to distinguish static routes from sibling parameters.
type HTTPPermissionRoute struct { Method string; Path string }
type HTTPPermissionRouteCatalog interface { SetHTTPPermissionRoutes([]HTTPPermissionRoute) }

// AnonymousHTTPModule handles explicit exceptions at its HTTP boundary.
type AnonymousHTTPModule interface {
	HTTPModule
	AllowAnonymousHTTP(*http.Request) bool
}

{{ if .Computed.enable_grpc_final -}}
// GRPCModule exposes RPC services without requiring an HTTP route or Name.
type GRPCModule interface {
 GRPC(grpc.ServiceRegistrar)
}
{{ end -}}

// HTTPAuditRecorder classifies business targets and persists sanitized audit events.
type HTTPAuditRecorder interface {
 AuditTarget(method, path string) auditctx.Target
 RecordHTTP(context.Context, auditctx.Event) error
}

// HTTPLocalizer supplies runtime translations without coupling HTTP to a business package.
type HTTPLocalizer interface { HTTPModule; Translator() i18n.Translator }
