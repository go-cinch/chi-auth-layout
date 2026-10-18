# Business Module Guidelines

Follow the [root guidelines](../../AGENTS.md) for project-wide rules. This file covers business modules and their API contracts, including supporting infrastructure and generators.

## Organization

- Use a short, singular package name such as `user`, `order`, or `payment`.
- Keep one capability cohesive. Its business types, rules, database operations, and transport adapters may share the same Go package.
- Do not create MVC-style `model`, `controller`, `service`, or `repository` directories by default.

## Business Entry Point and Dependencies

- Each capability has one concrete `Module` owning its dependencies and business methods, beyond HTTP routing. This type is distinct from the interface alias in `internal/modules`.
- Keep CRUD, caching, cross-module reuse, and testable business logic on `Module`; do not add a `Service + Module` or `Repository` layer for those purposes. Split large implementations into behavior-named files within the package.
- Helper types may encapsulate distinct behavior, such as sessions or captcha generation, without duplicating the business entry point.
- Every module provides a package-level `New` constructor and accepts required dependencies explicitly; handler-maker functions also accept their dependencies.
- Introduce interfaces only when substitution or isolation is needed, and define them beside their consumers. Cross-module callers use small consumer-owned interfaces.
- Wire shared modules and modules whose constructors can fail in `internal/app`. Store concrete pointers on `Application` before `generatedModules()`; `make api` and cross-module callers reuse these instances.
- Constructor dependency types must match fields on `app.Application`; `make api` wires matching fields automatically.
- Module-specific SQL belongs in the owning capability. `internal/infra/db` contains only generic connection, transaction, creation, and migration support.

## Layout and Growth

Start with `game.go` for business types, rules, and SQL. Add `http.go` when exposing HTTP; a capability without a transport may remain a single file:

```text
internal/modules/game/
  game.go
  http.go
```

- `http.go` contains the HTTP boundary: {{ .Computed.http_router_final }} route registration, handlers, request decoding, response mapping, and transport-only validation. Keep HTTP interface assertions and `Name()` here, even for a small capability.
- Do not create empty placeholder files or directories for possible future layers. Add files only when the capability needs them.

### Growth and File Size

- Review non-generated files approaching 1,000 lines; split by business behavior when it improves cohesion and navigation. This is not a module or package size limit.
- Prefer names such as `registration.go`, `profile.go`, `password.go`, or `permission.go` when splitting by behavior.
- Keep files in the same package until the capability has a real internal boundary that justifies another package.

## Module Contracts

- Keep module interfaces in `internal/modules/module.go`. `Module` is an alias of `HTTPModule`, not a separate contract.
{{ if eq .Computed.http_router_final "gin" -}}
- HTTP capabilities implement `modules.HTTPModule` with `Name() string` and `HTTP(r *gin.RouterGroup)`.
{{ else -}}
- HTTP capabilities implement `modules.HTTPModule` with `Name() string` and `HTTP() http.Handler`.
{{ end -}}
- HTTP modules return their application mount path from `Name()`; match it to the singular module name. The auth module declares `ConfigurableHTTPModule`: its `auth.path` scalar may be empty for a root mount or an explicit prefix such as `/auth`.
- HTTP infrastructure integrates modules through `HTTPModule` and the optional `IdempotentHTTPModule`, `PublicHTTPModule`, `HTTPPermissionAuthorizer`, `AnonymousHTTPModule`, and `HTTPAuditRecorder`, and `ConfigurableHTTPModule` contracts in the same package.

## HTTP

### Route Paths and Mounting

Keep these three path layers distinct:

| Layer | Owner | Example |
| --- | --- | --- |
| Module-local routes | The module's `HTTP` method | `/` (chi) or `""` (Gin) for the collection; `/{id}` (chi) or `/:id` (Gin) for one record |
| Application mount path | `internal/app` and the shared router, using `Name()` | `Name()` returns `/game`, yielding `/game` and `/game/{id}` (chi) or `/game/:id` (Gin) |
| Deployed gateway path | Nginx configuration | The external path may retain or rewrite the application path; see the [authentication and authorization examples](#authentication-and-authorization) |

- Use singular resource nouns in the module mount path and nested resources. Register relative routes without repeating the module mount path or adding a deployment prefix.
- Do not impose an `/api` or version prefix inside the module.

### Router Integration

{{ if eq .Computed.http_router_final "gin" -}}
- Register relative routes on the provided `*gin.RouterGroup` inside `http.go`; the application owns the single Gin engine and the module prefix.
- Apply capability-wide middleware with `Use`, nested resource middleware with `Group`, and route-specific middleware before the final handler.
- Use `c.Param` for path parameters and `c.Request.Context()` for downstream operations. Keep `*gin.Context` out of business methods and use `c.Writer` with shared JSON helpers.
- Call `c.Abort()` when middleware rejects a request; writing an error alone does not stop subsequent handlers.
{{ else -}}
- Construct module routes with `chi.NewRouter` inside `http.go` and mount the returned handler in `internal/app`. Keep chi-specific routing inside the HTTP boundary.
- Apply capability-wide middleware with `Use`, route-specific middleware with `With`, and nested resource middleware with `Route`.
{{ end -}}
- Use literal route paths and inline groups inside `HTTP` so `make api` can discover complete paths and final handlers.
- Do not retain request-scoped values after the handler returns unless copied.

### Authentication and Authorization

- Put unauthenticated business endpoints under module-local `/pub`; other endpoints require authentication by default. Permission-check exemptions alone do not qualify for `/pub`. Deployed gateway paths such as `POST /auth/pub/login` and `POST /order/pub/create` include mount prefixes, not module-local registrations.
- Only the gateway permission endpoint may allow anonymous checks when gateway-overwritten target headers identify a public `/pub` path or HEAD/OPTIONS method; ordinary protected routes remain authenticated. After authenticated authorization, return the `X-Code` response header, not a username.

### Idempotency

- Modules that use `x-idempotent` for POST create operations must implement `modules.IdempotentHTTPModule`. Clients reuse one key for retries of the same logical creation. The router atomically claims non-empty keys before the handler; repeated keys return HTTP 409.

### Input, Responses, and Errors

- Parse transport input and map transport output at the HTTP boundary; keep reusable business decisions in non-transport functions. Use the shared JSON helpers.
- Use JSON responses and `{"error_code":"STABLE_CODE","msg":"localized message"}` for errors. Downloads, including CSV, use their media type, filename, and matching OpenAPI content; download errors still use JSON. For an empty HTTP 200, call `WriteOK(w)` without a body argument.
- Map `ErrNotFound` (including mapped `sql.ErrNoRows`) to HTTP 404 with `server.WriteError(w, r, http.StatusNotFound, err)`; use `c.Writer` for Gin. Unmatched routes also return HTTP 404.
- Apply error mapping at the HTTP boundary. Return HTTP 400 for malformed input or invalid business fields, and HTTP 500 for unexpected database/server failures. Do not turn SQL/connection failures into missing-record errors or globally rewrite error status codes.
- Generated OpenAPI response statuses and body schemas must match the actual HTTP behavior.

### Partial Updates

- Use PATCH for partial updates: the module-local item route is `/{id}` (chi) or `/:id` (Gin), mounted under the resource path by the application. Preserve omitted fields; use pointer fields in HTTP inputs{{ if .Computed.enable_grpc_final }} and Proto `optional` fields{{ end }} to distinguish omission from an explicit empty value.

## API Conventions

### Method Naming

Use these AIP names for API operations and short names such as `Get` and `List` within resource-scoped Go packages. HTTP paths and pagination follow this project's conventions.

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
HTTP bulk deletion uses comma-separated IDs.

### Query Parsing

- In both chi and Gin handlers, parse the query once with `query, err := url.ParseQuery(r.URL.RawQuery)`; in Gin, obtain `r` from `c.Request`. Return HTTP 400 on a parsing error and do not use the partial result. Reuse the parsed query for all fields.

### Keyword Queries

- Prefer `q` for one keyword across multiple fields, such as name OR word. Combine alternatives in a parenthesized backend OR group, not separate client requests.
- Reserve `q1`, `q2`, etc. for independent keyword inputs; document each input's fields and how groups combine.
- Combine keyword groups with other filters using AND unless the endpoint explicitly documents different semantics. Apply identical filters to counts and page records before pagination.
- Trim keywords and ignore empty values. Use bound SQL parameters and escape LIKE wildcards for literal substring matching. Document case sensitivity and preserve dedicated field filters where needed.

### Pagination

- Use `p` (page number) and `s` (page size) in HTTP{{ if .Computed.enable_grpc_final }} and Proto{{ end }} requests/responses.
- Configure `maxP` and `maxS` in `conf/pagination.yml`; both default to 10000. They are independent limits with no additional cap on `p * s` or the offset. Environment overrides are `SERVICE_PAGINATION_MAXP` and `SERVICE_PAGINATION_MAXS`.
- Inject `pagination.Limits` from `conf/pagination.yml` into business modules and use the same limits in generated documentation. HTTP validates parameter presence and format; business methods apply page-number and page-size limits.
- Inspect `query["p"]` and `query["s"]` directly to distinguish omission from an explicitly supplied value. Each supplied parameter must have exactly one value that parses with `strconv.ParseInt(value, 10, 32)`. Reject duplicates, empty strings, non-integers, and integer overflow with HTTP 400. Use the same parsed `query` for other query fields.
- Use Go pointers{{ if .Computed.enable_grpc_final }} and Proto `optional` fields{{ end }} to preserve presence. Omitted `p`/`s` select page 1 and size `min(1, maxS)`. Explicit zero, negative, or configured out-of-range `int32` values return an empty list without querying the database; these are range results, not format errors.
- Order paginated results consistently. Paginated HTTP responses expose the total count as `t`. Pages beyond the data return an empty list with the matching total count; configured out-of-range requests return `t: 0`. Consider [cursor pagination](https://google.aip.dev/158) when deep offsets become expensive.

### Field Selection (`f`)

- Composite read endpoints with independently useful results must support `f`, including summaries, reports, and details with expensive associations. It selects independently loadable fields or groups, regardless of route name.
- Accept one comma-separated string of allowlisted English `snake_case` names, e.g. `f=metrics,trend`. Prefer response field names; document group selectors' keys and dependencies. Clients explicitly select only the results they use.
- Use the shared parser before data access: trim whitespace; reject empty selections/elements, unknown or duplicate names, and repeated `f` parameters with HTTP 400. Names are case-sensitive; order is irrelevant. Do not add aliases such as `fields` or `include`.
- Execute only queries, aggregates, joins, CTEs, calls, and transformations needed by selected results; reuse shared dependencies. Do not compute everything then filter JSON. Map selectors to server-owned SQL fragments, never directly to identifiers.
- Return selected results and documented required metadata. Preserve selected zero values, empty arrays, and meaningful nulls; omit unselected results. Document always-present and selection-dependent metadata, and align client types and OpenAPI.
- Document omitted-`f` behavior: preserve existing contracts for older clients; give new endpoints small, stable defaults. Clients still send `f` explicitly.
- Preserve authorization, filters, time boundaries, and consistency across selections, including required shared snapshots or captured request times. Selection must not grant access or change a result's meaning.
- Test response keys, zero/empty results, invalid selectors, omitted defaults, and skipped queries/aggregations. Verify actual query or downstream-call execution, not just response size; count authentication/authorization separately.

### Time Ranges and Calendar Days

- Express time queries as half-open intervals `[from, to)`: SQL uses `timestamp >= from AND timestamp < to`. Apply identical predicates to total counts, paginated records and any matching aggregates.
- Business days run from local midnight to the next midnight, exclusive. Never end at `23:59:59` or truncate milliseconds.
- Resolve both boundaries in the business IANA timezone by advancing calendar dates, not adding 24 hours. Handle 23/25-hour DST days and skipped or repeated midnights using actual local date boundaries.
- Preserve optional bounds using pointers. Validate supplied Unix-millisecond values, duplicates and ordering; when both bounds are present require `from < to`. A single bound constrains only its corresponding side, subject to the business meaning of missing/zero timestamps.
- Chart drilldown ranges retain complete calendar-day boundaries, including today. When excluding future records from live aggregates, use a separate captured-request cutoff; retain the public next-midnight bound.

{{ if .Computed.enable_grpc_final -}}
## gRPC

- Put adapters in each capability's `grpc.go`. Implement `modules.GRPCModule` with `GRPC(grpc.ServiceRegistrar)`; keep registration and the interface assertion beside the adapter, and embed the generated unimplemented server.
- HTTP and gRPC share the business module instance and call its business methods. Either transport can be removed independently; gRPC-only modules require neither `http.go`, `Name()`, nor `HTTP()`.
- RPC methods use the resource-qualified [method names](#method-naming). Requests use `<Method>Request`; list responses use `<Method>Response`. Get/Create/Update return the resource; Delete returns `google.protobuf.Empty`.
- Follow the [root protocol and infrastructure rules](../../AGENTS.md#protocols-and-rpc-infrastructure) for Proto files, compatibility, server lifecycle, and client connections.

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

## Testing

- Prioritize business rules, permission boundaries, state transitions, and failure behavior.
- Add database integration coverage when module behavior depends on SQL semantics or constraints.
- Cover an existing record, a missing record (404 with message), an unmatched route (404), invalid input (400), and a database failure (500) in HTTP tests.
