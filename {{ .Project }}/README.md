# {{ .Computed.service_name_final }}

A PostgreSQL authentication service using **{{ .Computed.http_router_final }}** {{ if .Computed.enable_grpc_final }}with optional gRPC{{ else }}for HTTP{{ end }}.

Authentication, business modules and embedded SQL migrations are always included.
Set `database.migrate: false` in `conf/database.yml` when migrations are managed
externally; this only skips startup migration execution. PostgreSQL remains a
required dependency.

## Run

```bash
make gen
make tidy
make test
make run
```

## Authentication endpoint path

`conf/auth.yml` controls only the authentication module's mount:

```yaml
auth:
  path: ""
```

The empty default exposes `/info`, `/pub/login`, `/challenge`, `/reset/pwd`
and `/permission` at the service root. Set `auth.path: "/auth"` to expose
`/auth/info`, `/auth/pub/login` and the other authentication endpoints under
`/auth`. `SERVICE_AUTH_PATH` overrides this scalar, including an explicitly
empty environment value. Other modules retain `/user`, `/role`, `/action`, etc.
Restart the service after changing the mount; regenerate OpenAPI with `make api`.
Permissions, first-login password restrictions and audit classification use the
same configured mount. Existing stored permission endpoint identities are retained;
runtime catalog reads project them to the configured mount.

Gateway service prefixes are independent. With an external `/api/auth` prefix
and the default empty mount, the user-info URL is `/api/auth/info`.
Frontend auth endpoint paths must use the same mount.

## Nginx configuration

Add these locations inside your Nginx `server` block. Replace `order-service`
and `auth-service` with your upstream addresses and `/order/` with your service
path. The default permission check uses `GET /permission` and forwards the verified
user code through `X-Code`.

Overwrite the target headers from the original request and keep the permission
subrequest location `internal`. If `auth.path` is `/auth`, use
`http://auth-service/auth/permission` instead:

```nginx
location ^~ /order/ {
  auth_request /_permission;
  auth_request_set $authenticated_code $upstream_http_x_code;
  proxy_set_header X-Code $authenticated_code;
  proxy_set_header X-Verified-Client-IP $remote_addr;
  proxy_pass http://order-service;
}

location = /_permission {
  internal;
  proxy_method GET;
  proxy_pass_request_body off;
  proxy_set_header Content-Length "";
  proxy_set_header Authorization $http_authorization;
  proxy_set_header X-Original-Method $request_method;
  proxy_set_header X-Permission-URI $request_uri;
  proxy_set_header X-Original-Resource "";
  proxy_set_header X-Verified-Client-IP $remote_addr;
  proxy_pass http://auth-service/permission;
}
```
