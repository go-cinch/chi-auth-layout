{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
 "{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"errors"
	
	"log/slog"
 "{{ .Computed.module_name_final }}/internal/common/auditctx"
	
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/modules"
)

func NewRouter(cfg *config.Config, authenticator *authn.Manager, idempotencyKeys idempotency.Store, mounted ...modules.Module) (http.Handler, error) {
	
	permissionAuthorizer, permissionEndpoint, err := newHTTPPermissionAuthorizer(cfg, mounted)
	if err != nil {
		return nil, err
	}
	
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.HandleMethodNotAllowed = true
	// ClientIP handles the verified header; disable Gin's X-Forwarded-For/X-Real-IP resolver.
	if err := r.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	policy := redact.New(cfg.Redact.Keys...)
{{- if .Computed.enable_trace_final }}
	r.Use(TraceID(cfg.Server.Name, policy))
{{- end }}
	r.Use(ClientIP(), Locale(httpTranslator(mounted)), AccessLog(policy), Audit(auditRecorder(mounted), policy), Recoverer(), RequestTimeout(cfg.HTTP.Timeout))
	r.NoRoute(func(c *gin.Context) {
		WriteError(c.Writer, c.Request, http.StatusNotFound, apperror.NotFound)
	})
	r.NoMethod(func(c *gin.Context) {
		WriteError(c.Writer, c.Request, http.StatusMethodNotAllowed, apperror.MethodNotAllowed)
	})
	seen := make(map[string]struct{}, len(mounted))
	for _, module := range mounted {
		if module == nil {
			return nil, errors.New("http module is required")
		}
		pattern := strings.TrimSpace(module.Name())
        if _, configured := module.(modules.ConfigurableHTTPModule); configured {
            normalized, err := config.NormalizeHTTPPath(module.Name())
            if err != nil { return nil, err }
            if _, authorizes := module.(modules.HTTPPermissionAuthorizer); authorizes && authenticator != nil {
                if err := authenticator.SetHTTPPath(normalized); err != nil { return nil, err }
            }
            pattern = normalized
            if pattern == "" { pattern = "/" }
        }
		if pattern == "" || !strings.HasPrefix(pattern, "/") {
			return nil, errors.New("http module pattern must start with /")
		}
		if _, exists := seen[pattern]; exists {
			return nil, errors.New("http module pattern is duplicated")
		}
		seen[pattern] = struct{}{}
		group := r.Group(pattern)

		if !isPublicModule(module) {
			if authenticator == nil {
				return nil, errors.New("http authenticator is required")
			}
			group.Use(authenticationMiddleware(authenticator, permissionAuthorizer, permissionEndpoint, pattern, module))
		}
		if isIdempotentModule(module) {
			if idempotencyKeys == nil || cfg.Idempotency.TTL <= 0 {
				return nil, errors.New("idempotency store and positive ttl are required")
			}
			group.Use(GinIdempotency(idempotencyKeys, cfg.Idempotency.TTL))
		}

		module.HTTP(group)
	}
	// Gateway checks need route precedence even when local authorization is disabled.
	catalog := make([]modules.HTTPPermissionRoute, 0)
	for _, route := range r.Routes() {
		catalog = append(catalog, modules.HTTPPermissionRoute{Method: route.Method, Path: route.Path})
	}
	for _, module := range mounted {
		if receiver, ok := module.(modules.HTTPPermissionRouteCatalog); ok {
			receiver.SetHTTPPermissionRoutes(catalog)
		}
	}
 return r,nil
}

func authenticationMiddleware(authenticator *authn.Manager, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint, modulePath string, module modules.Module) gin.HandlerFunc {
	return func(c *gin.Context) {
		if anonymous, ok := module.(modules.AnonymousHTTPModule); ok && anonymous.AllowAnonymousHTTP(c.Request) {
			c.Next()
			return
		}
		if isPublicEndpoint(c.Request.URL.Path, modulePath) {
			c.Next()
			return
		}
		request, err := authenticator.AuthenticateRequest(c.Request)
		if err != nil {
			c.Abort()
			if errors.Is(err, authn.ErrPasswordResetRequired) {
                WriteError(c.Writer, c.Request, http.StatusForbidden, err)
            } else { WriteError(c.Writer, c.Request, http.StatusUnauthorized, apperror.Unauthorized) }
			return
		}
		request, err = authn.ScopeRequest(request)
 if err != nil { c.Abort(); WriteError(c.Writer,c.Request,http.StatusBadRequest,err); return }
 c.Request = request
        identity, _ := authn.FromContext(request.Context()); auditctx.Identity(request.Context(),identity.UserID,identity.Username)
		if authorizer != nil && c.Request.URL.Path != permissionEndpoint {
			allowed, err := authorizer.AuthorizeHTTP(c.Request.Context(), c.Request.Method, c.Request.URL.Path)
			if err != nil {
				slog.ErrorContext(c.Request.Context(), "authorize http request failed: "+err.Error(),
					"operation", "authorize_http_request", "outcome", "failure", "method", c.Request.Method, "error", err)
				c.Abort()
				WriteError(c.Writer, c.Request, http.StatusInternalServerError, apperror.Internal)
				return
			}
			if !allowed {
				c.Abort()
				WriteError(c.Writer, c.Request, http.StatusForbidden, apperror.Forbidden)
				return
			}
		}
		c.Next()
	}
}

func newHTTPPermissionAuthorizer(cfg *config.Config, mounted []modules.Module) (modules.HTTPPermissionAuthorizer, string, error) {
	if !cfg.Auth.Authorization.Enabled {
		return nil, "", nil
	}
	var found modules.HTTPPermissionAuthorizer
	for _, module := range mounted {
		candidate, ok := module.(modules.HTTPPermissionAuthorizer)
		if !ok {
			continue
		}
		if found != nil {
			return nil, "", errors.New("http permission authorizer is duplicated")
		}
		found = candidate
	}
	if found == nil {
		return nil, "", errors.New("http permission authorizer is required")
	}
	endpoint := strings.TrimSpace(found.PermissionEndpoint())
	if endpoint == "" || !strings.HasPrefix(endpoint, "/") {
		return nil, "", errors.New("http permission endpoint must start with /")
	}
	return found, strings.TrimSuffix(found.Name(), "/") + endpoint, nil
}

func isPublicModule(module modules.Module) bool {
	public, ok := module.(modules.PublicHTTPModule)
	return ok && public.Public()
}

func isIdempotentModule(module modules.Module) bool {
	idempotent, ok := module.(modules.IdempotentHTTPModule)
	return ok && idempotent.Idempotent()
}

func isPublicEndpoint(path, modulePath string) bool {
	publicPath := strings.TrimSuffix(modulePath, "/") + "/pub"
	return path == publicPath || strings.HasPrefix(path, publicPath+"/")
}

{{ else -}}
package server

import (
 "{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"errors"
	
	"log/slog"
 "{{ .Computed.module_name_final }}/internal/common/auditctx"
	
	"net/http"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/modules"

	"github.com/go-chi/chi/v5"
)

func NewRouter(cfg *config.Config, authenticator *authn.Manager, idempotencyKeys idempotency.Store, mounted ...modules.Module) (http.Handler, error) {
	
	permissionAuthorizer, permissionEndpoint, err := newHTTPPermissionAuthorizer(cfg, mounted)
	if err != nil {
		return nil, err
	}
	
	r := chi.NewRouter()
	policy := redact.New(cfg.Redact.Keys...)
{{- if .Computed.enable_trace_final }}
	r.Use(TraceID(cfg.Server.Name, policy))
{{- end }}
	r.Use(ClientIP(), Locale(httpTranslator(mounted)), AccessLog(policy), Audit(auditRecorder(mounted), policy), Recoverer())

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, apperror.NotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, apperror.MethodNotAllowed)
	})
	routes := r.With(RequestTimeout(cfg.HTTP.Timeout))
	catalog:=make([]modules.HTTPPermissionRoute,0)
 seen := make(map[string]struct{}, len(mounted))
	for _, module := range mounted {
		if module == nil {
			return nil, errors.New("http module is required")
		}
		pattern := strings.TrimSpace(module.Name())
        if _, configured := module.(modules.ConfigurableHTTPModule); configured {
            normalized, err := config.NormalizeHTTPPath(module.Name())
            if err != nil { return nil, err }
            if _, authorizes := module.(modules.HTTPPermissionAuthorizer); authorizes && authenticator != nil {
                if err := authenticator.SetHTTPPath(normalized); err != nil { return nil, err }
            }
            pattern = normalized
            if pattern == "" { pattern = "/" }
        }
		if pattern == "" || !strings.HasPrefix(pattern, "/") {
			return nil, errors.New("http module pattern must start with /")
		}
		if _, exists := seen[pattern]; exists {
			return nil, errors.New("http module pattern is duplicated")
		}
		seen[pattern] = struct{}{}
		handler := module.HTTP()
		if handler == nil {
			return nil, errors.New("http module handler is required")
		}

        moduleRoutes := make([]modules.HTTPPermissionRoute, 0)
        rootHandlers := make(map[string]http.Handler)
        if registered, ok := handler.(chi.Routes); ok {
            if err := chi.Walk(registered, func(method, path string, endpoint http.Handler, middleware ...func(http.Handler) http.Handler) error {
                route := modules.HTTPPermissionRoute{Method: method, Path: strings.TrimSuffix(pattern, "/") + path}
                catalog = append(catalog, route)
                moduleRoutes = append(moduleRoutes, route)
                rootHandlers[method+" "+route.Path] = chi.Chain(middleware...).Handler(endpoint)
                return nil
            }); err != nil { return nil, err }
        }

        if isIdempotentModule(module) && (idempotencyKeys == nil || cfg.Idempotency.TTL <= 0) {
            return nil, errors.New("idempotency store and positive ttl are required")
        }
        if !isPublicModule(module) && authenticator == nil {
            return nil, errors.New("http authenticator is required")
        }
        wrapModule := func(next http.Handler) http.Handler {
            if isIdempotentModule(module) { next = Idempotency(idempotencyKeys, cfg.Idempotency.TTL)(next) }
            if !isPublicModule(module) { next = authenticationMiddleware(authenticator, permissionAuthorizer, permissionEndpoint, pattern, module, next) }
            return next
        }
        if _, configured := module.(modules.ConfigurableHTTPModule); configured && pattern == "/" {
            if len(moduleRoutes) == 0 { return nil, errors.New("root module must expose discoverable http routes") }
            for _, route := range moduleRoutes { routes.Method(route.Method, route.Path, wrapModule(rootHandlers[route.Method+" "+route.Path])) }
        } else {
            routes.Mount(pattern, wrapModule(handler))
        }
	}

	// Gateway checks need route precedence even when local authorization is disabled.
	for _, module := range mounted {
		if receiver, ok := module.(modules.HTTPPermissionRouteCatalog); ok {
			receiver.SetHTTPPermissionRoutes(catalog)
		}
	}
 return r,nil
}

func authenticationMiddleware(authenticator *authn.Manager, authorizer modules.HTTPPermissionAuthorizer, permissionEndpoint, modulePath string, module modules.Module, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if anonymous, ok := module.(modules.AnonymousHTTPModule); ok && anonymous.AllowAnonymousHTTP(r) {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicEndpoint(r.URL.Path, modulePath) {
			next.ServeHTTP(w, r)
			return
		}
		request, err := authenticator.AuthenticateRequest(r)
		if err != nil {
			if errors.Is(err, authn.ErrPasswordResetRequired) {
                WriteError(w, r, http.StatusForbidden, err)
            } else { WriteError(w, r, http.StatusUnauthorized, apperror.Unauthorized) }
			return
		}
		request, err = authn.ScopeRequest(request)
 if err != nil { WriteError(w,r,http.StatusBadRequest,err); return }
 identity, _ := authn.FromContext(request.Context()); auditctx.Identity(request.Context(),identity.UserID,identity.Username)
        if authorizer != nil && request.URL.Path != permissionEndpoint {
			allowed, err := authorizer.AuthorizeHTTP(request.Context(), request.Method, request.URL.Path)
			if err != nil {
				slog.ErrorContext(request.Context(), "authorize http request failed: "+err.Error(),
					"operation", "authorize_http_request", "outcome", "failure", "method", request.Method, "error", err)
				WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
				return
			}
			if !allowed {
				WriteError(w, r, http.StatusForbidden, apperror.Forbidden)
				return
			}
		}
		next.ServeHTTP(w, request)
	})
}

func newHTTPPermissionAuthorizer(cfg *config.Config, mounted []modules.Module) (modules.HTTPPermissionAuthorizer, string, error) {
	if !cfg.Auth.Authorization.Enabled {
		return nil, "", nil
	}
	var found modules.HTTPPermissionAuthorizer
	for _, module := range mounted {
		candidate, ok := module.(modules.HTTPPermissionAuthorizer)
		if !ok {
			continue
		}
		if found != nil {
			return nil, "", errors.New("http permission authorizer is duplicated")
		}
		found = candidate
	}
	if found == nil {
		return nil, "", errors.New("http permission authorizer is required")
	}
	endpoint := strings.TrimSpace(found.PermissionEndpoint())
	if endpoint == "" || !strings.HasPrefix(endpoint, "/") {
		return nil, "", errors.New("http permission endpoint must start with /")
	}
	return found, strings.TrimSuffix(found.Name(), "/") + endpoint, nil
}

func isPublicModule(module modules.Module) bool {
	public, ok := module.(modules.PublicHTTPModule)
	return ok && public.Public()
}

func isIdempotentModule(module modules.Module) bool {
	idempotent, ok := module.(modules.IdempotentHTTPModule)
	return ok && idempotent.Idempotent()
}

func isPublicEndpoint(path, modulePath string) bool {
	publicPath := strings.TrimSuffix(modulePath, "/") + "/pub"
	return path == publicPath || strings.HasPrefix(path, publicPath+"/")
}

{{ end -}}

func httpTranslator(mounted []modules.Module) i18n.Translator { for _, module := range mounted { if source,ok:=module.(modules.HTTPLocalizer);ok {return source.Translator()} }; return nil }
