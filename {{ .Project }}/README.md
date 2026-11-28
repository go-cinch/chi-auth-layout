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

## Email setup

`conf/email.yml` uses `smtp.resend.com`, username `resend`, and sender
`no-reply@{{ .Computed.brand_domain_final }}`. The SMTP password is intentionally
empty. Before starting with email enabled, set `SERVICE_EMAIL_PASSWORD` or fill
in `email.password`, and configure the sender domain with your mail provider.
The sender display name is `{{ (fromJSON .Computed.brand).title }}`.

For local development without sending email, run:

```bash
SERVICE_EMAIL_SKIPDELIVERY=true make run
```

This completes queued deliveries without SMTP. Set `SERVICE_EMAIL_ENABLED=false`
instead to disable email. Configure `auth.email.unlinkURL` and
`auth.email.auditURL` with the actual frontend URLs for email action links.

### Sent email for end-to-end tests

With `server.env: dev` and `email.enabled: true`, enable
`auth.testing.emailQueueEndpoint`. An empty `auth.testing.emailQueueToken`
allows anonymous access to verification codes. Use this mode in isolated test
environments.

For a local run with anonymous access and simulated SMTP delivery:

```bash
SERVICE_AUTH_TESTING_EMAILQUEUETOKEN='' \
  SERVICE_AUTH_TESTING_EMAILQUEUEENDPOINT=true SERVICE_EMAIL_SKIPDELIVERY=true make run
```

Call `GET /pub/testing/email-queue` from the test runner and prepend `auth.path`
when set.

```bash
curl --get 'http://127.0.0.1:{{ or .Scaffold.http_port "8080" }}/pub/testing/email-queue' \
  --data-urlencode "verification_id=$verification_id"
```

To require authentication, set `SERVICE_AUTH_TESTING_EMAILQUEUETOKEN` to a
dedicated token of at least 32 bytes and send it as `Authorization: Bearer <token>`.
Use HTTPS when calling across a network.

Poll using the `verification_id` returned by the code request or filter by
`recipient`. The `items` array contains successful deliveries that match the
filters, with the recipient, queue, and sent time in Unix milliseconds. For
verification emails, `code` is the six-digit string used in the email. Each
instance keeps its last 25 successful
deliveries in memory, newest first. In multi-instance tests, poll the instance
that runs the worker.
With `email.skipDelivery: true`, successful auth mail appears after simulated
delivery. With SMTP enabled, it appears after relay acceptance.

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
