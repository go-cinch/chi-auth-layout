# Development Guidelines

## Architecture and Dependencies

- Keep `cmd/*` thin: parse flags, initialize the application, and run it.
- Put process wiring and lifecycle code in `internal/app`.
- Put reusable technical infrastructure in `internal/common` and `internal/infra`.
- Organize business code by capability under `internal/modules/<capability>`.
- Do not introduce global `handler`, `service`, `repository`, or `model` folders.
- Use the capability's concrete `Module` as its business entry point; follow the [business module guidelines](internal/modules/AGENTS.md) instead of adding a `Service`/`Repository` layer around the same capability.
- `internal/common` must not import concrete business packages under `internal/modules/<capability>`. HTTP server infrastructure may depend on the interfaces defined in `internal/modules`: `HTTPModule` (also aliased as `Module`), `IdempotentHTTPModule`, `PublicHTTPModule`, `HTTPPermissionAuthorizer`, and `AnonymousHTTPModule`.{{ if .Computed.enable_grpc_final }} RPC server infrastructure may depend on `GRPCModule` in the same package.{{ end }}
- Business modules may depend on common infrastructure and small interfaces.
- Wire dependencies explicitly in `internal/app`; avoid package globals.
- Propagate `context.Context` through database, Redis, and downstream calls.

## Scoped Guidelines

- Business module changes must follow [`internal/modules/AGENTS.md`](internal/modules/AGENTS.md).
- Database changes must follow [`internal/infra/db/AGENTS.md`](internal/infra/db/AGENTS.md).

## Naming and Data Conventions

### Identifiers and JSON Keys

- Exported Go struct fields use upper camel case; locals, parameters, and unexported fields use lower camel case. Preserve standard initialisms: `UserID`, `HTTPServer`, `userID`, `apiURL`.
- Database columns and all HTTP request/response and database JSON keys use snake case, including nested keys. Keep JSON names identical across HTTP and database boundaries; do not add camel case aliases or recursive key conversion.
- Map Go fields to JSON names with tags, for example `UserID int64` with `json:"user_id"`.

### Timestamps and Numeric Representation

- Date/time response fields use `int64` Unix milliseconds since `1970-01-01T00:00:00Z` in Go{{ if .Computed.enable_grpc_final }} and Proto{{ end }}. Convert database time values with `UnixMilli()`.
- Name event timestamps consistently with a past participle followed by `At`: `CreatedAt`, `UpdatedAt`, `StartedAt`, and `EndedAt` in Go. Use `created_at`, `updated_at`, `started_at`, and `ended_at` in HTTP JSON, SQL{{ if .Computed.enable_grpc_final }}, and Proto{{ end }}. Do not mix these with `starts_at`/`ends_at` or `start_at`/`end_at`. Apply this convention to requests, responses, configuration, examples, and tests.
- HTTP JSON uses numbers.{{ if .Computed.enable_grpc_final }} ProtoJSON uses decimal strings for `int64`.{{ end }} Convert to JavaScript `Number` only within its safe integer range (±(2^53−1)); otherwise use `BigInt` or lossless JSON parsing.

### Error Naming

- Package-level error variables outside `internal/common/apperror` use an `Err` prefix when exported (for example `auth.ErrLoginFailed`) and an `err` prefix when unexported. Error variables inside `apperror` omit the prefix (for example `apperror.InvalidBody` and `apperror.NotFound`). Local error results continue to use `err` or descriptive names such as `parseErr`.
- Public error codes use uppercase snake case and describe both the scope and the failure, such as `HTTP_INVALID_USER_ID` or `AUTH_LOGIN_FAILED`; avoid field-only codes such as `HTTP_USER_ID`. Keep translation catalog keys, tests, and API documentation in sync when codes change.

## Go Style

- Separate Go function and method declarations with exactly one blank line; add a missing blank line and reduce multiple blank lines to one. Keep documentation comments attached to the function they describe, with the separating blank line before the comments. Short function bodies may remain on one line, such as `func (*Module) Name() string { return "/whitelist" }`. Preserve intentional whitespace in string literals and test fixture source. This is a source style convention, not an additional lint check.

## HTTP

### Routing and Request Boundaries

{{ if eq .Computed.http_router_final "gin" -}}
- Modules implement `HTTP(*gin.RouterGroup)` and register relative paths on the group provided by the application.
- Keep Gin routing and `*gin.Context` inside the HTTP boundary; pass `c.Request.Context()` to business code.
- Use the shared JSON helpers with `c.Writer` and call `c.Abort()` when middleware stops a request.
{{ else -}}
- Modules expose `http.Handler` and are mounted in `internal/app`.
- Keep chi-specific routing inside the HTTP boundary.
{{ end -}}
- Use singular nouns for business resource path segments, such as `/game`, `/order`, and `/payment`, including collection endpoints and nested resources. Match the prefix returned by `Name()` to the singular module name.
- Do not retain request-scoped values after the handler returns unless copied.

### Authentication and Authorization

- Put every unauthenticated business endpoint under the module-local `/pub` prefix. For example, final microservice deployment paths exposed through Nginx may be `POST /auth/pub/login` and `POST /order/pub/create`; these illustrate deployed gateway paths, not module-local route registrations. Endpoints outside `/pub` are authenticated by default. Do not use `/pub` for endpoints that merely skip permission checks but still require a logged-in user.
- The gateway permission endpoint may allow anonymous authorization checks when the gateway-overwritten target method and path headers identify a public `/pub` path (for example, a deployed gateway path such as `/auth/pub/login` or `/order/pub/create`) or a HEAD or OPTIONS target method. This exception applies only to the gateway permission endpoint, never to direct requests to ordinary protected routes. After authenticated authorization, return the `X-Code` response header, not a username.

### Idempotency

- Modules that use `x-idempotent` for POST create operations must implement `modules.IdempotentHTTPModule`. The router atomically claims non-empty keys before the handler; repeated keys return HTTP 409.

### Responses and Errors

- Response bodies use JSON; errors use `{"error_code":"STABLE_CODE","msg":"localized message"}`. For HTTP 200 with no response body, call `WriteOK(w)` without a body argument; it writes only the status code and no body.
- A missing business/database record returns HTTP 404 with a stable `error_code` and localized `msg` (for example `{"error_code":"GAME_NOT_FOUND","msg":"game not found"}`). Unmatched routes also return HTTP 404.
- Apply this mapping at the module's HTTP boundary. Return HTTP 400 for malformed input or invalid business fields, and HTTP 500 for unexpected database/server failures; do not globally rewrite error status codes.
- Generated OpenAPI response statuses and body schemas must match the actual HTTP behavior.

## CRUD and Pagination

The AIP references below apply to method naming only; HTTP paths and pagination follow the project conventions below.

Google CRUD naming references:

| Operation | Method pattern | Specification |
| --- | --- | --- |
| Get one | `Get<Resource>` | [AIP-131](https://google.aip.dev/131) |
| List | `List<Resources>` | [AIP-132](https://google.aip.dev/132) |
| Create | `Create<Resource>` | [AIP-133](https://google.aip.dev/133) |
| Update | `Update<Resource>` | [AIP-134](https://google.aip.dev/134) |
| Delete one | `Delete<Resource>` | [AIP-135](https://google.aip.dev/135) |

{{ if .Computed.enable_grpc_final -}}
Use `BatchDelete<Resources>` for bulk RPC deletion ([AIP-235](https://google.aip.dev/235)).
{{ end -}}
HTTP uses singular resource paths, `p`/`s`, and comma-separated IDs for bulk deletion.

### Pagination

- Use `p` (page number) and `s` (page size) for pagination fields in HTTP{{ if .Computed.enable_grpc_final }} and Proto{{ end }} requests/responses. Request fields are optional; omission selects defaults.
- For values that parse as `int32`, explicit zero, negative, or configured out-of-range values return an empty list.
- Malformed HTTP pagination parameters return HTTP 400, including empty values, non-integers, repeated parameters, and values outside the `int32` range.
- Configure `maxP` (maximum page number) and `maxS` (maximum page size) in `conf/pagination.yml`; both default to 10000. These are independent limits, with no additional cap on `p × s` or the offset.
- Use the same page-number and page-size limits in business methods and Swagger. Environment overrides are `SERVICE_PAGINATION_MAXP` and `SERVICE_PAGINATION_MAXS`.

{{ if .Computed.enable_grpc_final -}}
## gRPC

- Store Proto contracts in `api/<service>-proto/`, with `<service>.proto` as the service entry file; generate Go bindings into `api/<service>/`. For published contracts, keep field numbers stable and reserve removed fields.
- Format Proto files with two-space indentation and a blank line between top-level definitions; see the [Proto style guide](https://protobuf.dev/programming-guides/style/).
- Use service-specific Proto packages, such as `catalog.v1`. Preserve imported contracts and paths; `make gen` maps their Go imports to the local module. Shared Google contracts belong in `third_party`.
- Put gRPC adapters in each capability's `grpc.go`. Implement `modules.GRPCModule` and register via `GRPC(grpc.ServiceRegistrar)`; HTTP and gRPC share the business module instance.
- Start gRPC only when business modules register it. gRPC-only modules require neither `Name()` nor `HTTP()`.
- Configure address, unary timeout, shutdown deadline and reflection in `conf/grpc.yml`. Streams use caller deadlines; graceful shutdown has a forced-stop fallback.
- Initialize downstream clients with `rpc.NewClient` in `internal/app/client.go`, using `conf/client.yml`. Store typed clients on `Application`, inject through constructors, and register cleanup for shutdown and startup failures.
- Client unary calls default to 5 seconds and preserve shorter deadlines. Optional health checks require `SERVING`; otherwise initialization is lazy. Clients use TLS by default; set `Insecure: true` for plaintext.

Map business errors at the gRPC boundary:

| Error case | Go status code | Numeric code |
| --- | --- | --- |
| Invalid input, such as a non-positive game ID | `codes.InvalidArgument` | 3 |
| Missing business/database record | `codes.NotFound` | 5 |
| Request canceled | `codes.Canceled` | 1 |
| Request deadline exceeded | `codes.DeadlineExceeded` | 4 |
| Unexpected database/server failure | `codes.Internal` | 13 |

Use `status.FromContextError(err).Err()` for cancellation and deadline errors; keep internal failure details in logs and return a generic message to clients. See the [official complete gRPC status code list](https://grpc.io/docs/guides/status-codes/#the-full-list-of-status-codes) for other cases.

### User context between services

| Information | Location | Example |
| --- | --- | --- |
| Business input that defines the operation or must be persisted | Proto request fields | `owner_id`, `resource_id`, `quantity` |
| Current caller identity and authentication context | gRPC metadata | Verified identity claims or an appropriate credential |

- Distinguish the operator from the business subject. Identify users by stable user IDs; usernames are display/context information.
- Pass the request `ctx` to preserve deadlines and tracing. Add selected identity fields to outgoing metadata explicitly.
- Forward identity from verified context. Before authorizing a request, validate its credential or trust an authenticated calling service; `x-code` alone is not authentication. Never forward all inbound headers or credentials blindly.

Example using `google.golang.org/grpc/metadata` (`SomeRPC` stands for the generated client method):

```go
// Caller: use the verified user code; Copy/Set preserves other metadata without duplicates.
code := "EXP78RGH"
md, _ := metadata.FromOutgoingContext(ctx)
md = md.Copy()
md.Set("x-code", code)
ctx = metadata.NewOutgoingContext(ctx, md)
reply, err := client.SomeRPC(ctx, request)
```

```go
// Receiver: read at the gRPC boundary, after establishing caller trust.
md, _ := metadata.FromIncomingContext(ctx)
code := ""
if values := md.Get("x-code"); len(values) == 1 {
    code = values[0]
}
```

See the [official gRPC metadata guide](https://grpc.io/docs/guides/metadata/).

{{ end -}}

## Configuration

- `conf/*.yml` is the source of truth for the configuration model.
- Environment variables override existing YAML scalar leaves using uppercase paths joined by underscores and the `SERVICE_` prefix. Lists use zero-based indices, for example `SERVICE_HTTP_DOCS_SERVERS_2_URL` maps to `http.docs.servers[2].url`; nested lists include each index.
- Preserve untouched list items and sibling fields. Unknown fields and out-of-range indices are ignored; do not grow lists implicitly. Explicitly empty values are valid overrides.
- Share YAML/environment loading between the application and generators through `config.LoadValues`. Redact sensitive values in override logs, including indexed fields.
- Render OpenAPI server URLs and marked pagination range descriptions from runtime configuration, including environment overrides.
- Add project-specific sensitive names to `redact.keys`; never log raw secrets.

{{ if .Computed.enable_redis_final -}}
### Redis Keys

- Configure the shared Redis key prefix with `redis.prefix` in `conf/redis.yml` or `SERVICE_REDIS_PREFIX`. Use `<environment>:<service>:`; this auth template defaults to `dev:auth:`. All replicas of the same deployment must use the same prefix; other services and environments must use distinct prefixes.
- Build keys from semantic segments in the order `<environment>:<service>:<resource-type>:<subtype>:<id>`. Use `:` exclusively between segments, lowercase kebab-case for multi-word segments, and opaque URL-safe values only in ID segments. Do not use `.`, `_`, or `-` as substitutes for the `:` segment separator.
- The boundary before every ID must also be `:`. A hyphen may join words inside one semantic segment, as in `slider-proof:<id>`, but must never attach the ID itself. Generate `slider-proof:<id>`, never `slider-proof-<id>`, `slider-proof.<id>`, or `slider-proof_<id>`.
- Good examples: `prod:auth:point-captcha:slider-proof:<id>`, `dev:auth:point-captcha:password-change:<id>`, and `dev:auth:dictionary:cache:version`. Bad examples: `prod.auth.point-captcha.slider-proof.<id>`, `prod:auth:point_captcha:slider-proof:<id>`, `prod:auth:point-captcha:slider-proof.<id>`, and `prod:auth:point:captcha:slider-proof-<id>`.
- Business code passes relative keys such as `point-captcha:slider-proof:<id>` to the client from `internal/infra/rds`. The client automatically adds the `<environment>:<service>:` prefix for all key-bearing operations, including multi-key deletion and Lua `KEYS`. Do not add the configured prefix or repeat the environment or service name in business keys.
- Create Redis connections only inside `internal/infra/rds`, including dedicated cache connections, and use the same prefix-aware client. Do not expose the raw client, embed its unrestricted command API, or bypass it with `Do` or a raw pipeline. Add any newly needed commands to the shared client with key-isolation tests.
- Lua scripts must receive every accessed key through `KEYS`; never hardcode key names or pass keys in `ARGV`. Values and script arguments are not prefixed.

{{ end -}}
## Logging

- Apply these principles to all logs: business operations, infrastructure, startup, configuration, and HTTP requests.
- Put the information the reader needs first in `msg`: what happened, the outcome, and the key values for understanding it. Choose those values for the event rather than following a fixed field checklist.
- Keep messages concise. Use values directly when their meaning and order are clear; include field names only when they resolve ambiguity.
- Add fields only for useful secondary context. Omit irrelevant details, avoid duplicating values already in `msg`, and do not turn every variable into a separate field or add unnecessary wrapper objects.
- Let the shared slog setup supply standard metadata (`time`, `level`, `v`, `caller`) and trace correlation fields.
- Keep fixed prose lowercase, preserve the original case of values, and redact sensitive data before placing it in either `msg` or additional fields.

HTTP example (automatic metadata omitted):

Good: the message immediately shows the method, result, elapsed time, route, and path; client details remain available as secondary fields.

```json
{"msg":"GET 404 25ms /game/{id} /game/1","remote_addr":"127.0.0.1:53441","user_agent":"Mozilla/5.0"}
```

Bad: the generic message says little, and the reader must scan separate fields to understand the request.

```json
{"msg":"http request","method":"GET","status":404,"duration_ms":25,"route":"/game/{id}","path":"/game/1","remote_addr":"127.0.0.1:53441","user_agent":"Mozilla/5.0"}
```

## Code Generation

- Run `make config` when configuration fields or types change. Value changes take effect on restart.
- Run `make api` after changing module constructors, HTTP routes, or shared response helper calls.
- Run `make gen` to regenerate configuration,{{ if .Computed.enable_grpc_final }} protobuf bindings,{{ end }} module registrations and HTTP documentation.{{ if .Computed.enable_grpc_final }} Install `protoc` when adding Proto definitions; Go plugins are pinned and installed automatically into `.tools`.{{ end }}
- Generated config, module registries,{{ if .Computed.enable_grpc_final }} `.pb.go` files,{{ end }} and OpenAPI are tool-owned; regenerate them instead of editing by hand.
- Automatic generation tools must print a unified diff when an existing generated file changes; print `create` for a new file and `unchanged` when no change is needed.

## Testing and Verification

- Put package-level unit tests beside their implementation files.
- Name each unit test file after the implementation file it covers, such as `game.go` and `game_test.go`; do not add generic package-wide test filenames without a corresponding source file.
- Put cross-package and external-service integration tests in `internal/tests`.
- Use `net/http/httptest` for handlers and routers.
- Prefer external test packages for public behavior; use the implementation package when testing internal helpers is justified.
- Require at least 80% statement coverage for every package under `internal/modules` and at least 60% for every other production package.{{ if .Computed.enable_grpc_final }} Exclude compiler-generated `.pb.go` files from coverage thresholds; test adapters and RPC behavior with generated clients.{{ end }}
- Treat coverage as a guardrail: test meaningful behavior and failure paths, and do not add low-value tests or production indirection only to increase the percentage.
- Run `make lint`, `make test`, and `make build` before committing.

## Release Tags

- Stable tags use `vYYYY.M.PATCH`; beta tags use `vYYYY.M.PATCH-beta.N`. Require the `v` prefix, a four-digit year, month 1–12, and positive `PATCH`/`N` values. Do not zero-pad the month or counters.
- Choose the release sequence's year/month in `Asia/Shanghai`. `PATCH` counts monthly release sequences, starting at 1 each month; it is not the calendar day. Inspect remote tags, including prereleases, before choosing the next unused counter. Every new release or fix, including same-day fixes, advances `PATCH`.
- Beta candidates start at `beta.1` and increment `N`; promote a validated candidate by removing the suffix. Example: `v2026.9.1-beta.1` → `v2026.9.1-beta.2` → `v2026.9.1`; the next fix is `v2026.9.2`, and the first October sequence is `v2026.10.1`.
- Create an annotated tag on the exact clean, committed revision that passed `make lint`, `make test`, and `make build`. Use signed tags when signing is configured. Record release changes in `CHANGELOG.md` before tagging.
- Build release binaries with `make build` from the exact tagged or committed revision. The Makefile resolves an exact tag first and otherwise uses the five-character `git rev-parse --short=5 HEAD` SHA; use the same immutable tag for release images. Verify the artifact's version and source commit before publishing.
- Never move, overwrite, delete, or reuse published release tags or versioned images. Publish a new version for corrections.
- Create or push release tags only when explicitly requested. Commit/push authorization alone does not authorize a release. Push the specific tag, not all local tags.
