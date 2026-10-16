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

## Client IPs

Set `http.trustedProxies` (or `SERVICE_HTTP_TRUSTEDPROXIES`) to the IPs/CIDRs of
gateways allowed to supply `X-Verified-Client-IP`. The default is empty and trusts
no forwarded headers. Audit events accept exactly one valid IPv4/IPv6 address in
this header from a trusted connection; otherwise they record the transport peer.
`X-Real-IP` and `X-Forwarded-For` are ignored when resolving the client IP. Access
logs retain `RemoteAddr`. Login and operation audit details expose a separate
`header` snapshot of all received request headers, with sensitive values fully
redacted using `redact.keys` and the built-in policy. See [audit storage](internal/modules/audit/README.md).

The public gateway must overwrite `X-Verified-Client-IP` with its observed source
address on every request, including permission subrequests. For APISIX, use
`proxy-rewrite.headers.set` with `X-Verified-Client-IP: $remote_addr`. For a direct
Kubernetes LoadBalancer/NodePort entry, verify source IP preservation against the
actual CNI/LB path. `externalTrafficPolicy: Local` can preserve it when needed, but
only serves ready endpoints on the receiving node; do not change it blindly.
If another proxy precedes the gateway, configure
source IP recovery only for that trusted proxy; never trust arbitrary incoming
forwarding headers. Restrict backend access and keep trusted proxy ranges narrow.

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
  proxy_pass http://auth-service/auth/permission;
}
```
