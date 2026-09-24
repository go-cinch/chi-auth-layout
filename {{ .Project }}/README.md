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

## Nginx configuration

Add these locations inside your Nginx `server` block. Replace `order-service`
and `auth-service` with your upstream addresses and `/order/` with your service
path. The permission check uses `GET /auth/permission` and forwards the verified
user code through `X-Code`.

Overwrite the target headers from the original request and keep the permission
subrequest location `internal`:

```nginx
location ^~ /order/ {
  auth_request /_permission;
  auth_request_set $authenticated_code $upstream_http_x_code;
  proxy_set_header X-Code $authenticated_code;
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
  proxy_pass http://auth-service/auth/permission;
}
```
