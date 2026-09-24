# Business Module Guidelines

## Organization

- Organize business code by capability under `internal/modules/<capability>`.
- Use a short, singular package name such as `user`, `order`, or `payment`.
- Keep one capability cohesive. Its business types, rules, database operations, and transport adapters may share the same Go package.
- Do not create MVC-style `model`, `controller`, `service`, or `repository` directories by default.
- Keep shared technical infrastructure in `internal/common` or `internal/infra`; do not move capability-specific behavior into those directories.

## Business Entry Point and Dependencies

- Use one concrete `Module` type as the capability's business entry point. It owns the capability's dependencies, including database and cache access, and exposes business methods. `Module` is not limited to HTTP routing; here it means the concrete type in a capability package, not the interface alias in `internal/modules`.
- Do not split the same capability into `Service + Module` or add a `Repository` layer merely for CRUD, caching, cross-module reuse, or testing. Put these business methods on `Module`; split large implementations into behavior-named files in the same package.
- Keep focused helper types when they encapsulate distinct behavior, such as sessions or captcha generation. They must not become an extra layer that duplicates the capability's business entry point.
- Every module provides a package-level `New` constructor and accepts required dependencies explicitly; handler-maker functions also accept their dependencies. Avoid package globals.
- Introduce interfaces only when substitution or isolation is needed, and define them beside their consumers. Cross-module callers use small consumer-owned interfaces.
- Wire shared modules and modules whose constructors return errors in `internal/app`. Store their concrete pointers on `Application` before calling `generatedModules()`; `make api` reuses these instances for transport registration. Cross-module callers receive the same instance.
- Constructor dependency types must match fields on `app.Application`; `make api` wires matching fields automatically.
- Pass `context.Context` through database, Redis,{{ if .Computed.enable_grpc_final }} gRPC,{{ end }} and downstream HTTP operations.
- Module-specific SQL belongs in the owning capability. `internal/infra/db` contains only generic connection, transaction, creation, and migration support.

## Layout and Growth

Start with `game.go` for business types, rules, and small capability-specific database operations. A capability without a transport boundary may remain in that single file. When it exposes HTTP, use:

```text
internal/modules/game/
  game.go
  http.go
```

- `http.go` contains the HTTP boundary: {{ .Computed.http_router_final }} route registration, handlers, request decoding, response mapping, and transport-only validation. Keep HTTP interface assertions and `Name()` here, even for a small capability.
{{ if .Computed.enable_grpc_final -}}
- Add `grpc.go` when the capability exposes gRPC. Keep registration, the interface assertion, and the adapter in that file. Embed the generated unimplemented server on the adapter and call shared business methods. A gRPC-only capability needs no `http.go`; either transport can be removed independently.
{{ end -}}
- Do not create empty placeholder files or directories for possible future layers. Add files only when the capability needs them.

### Growth and File Size

- When a non-generated file approaches 1,000 lines, review whether it contains multiple business behaviors and split it only when doing so improves cohesion and navigation.
- Do not impose a total line limit on a module or package.
- Prefer names such as `registration.go`, `profile.go`, `password.go`, or `permission.go` when splitting by behavior.
- Keep files in the same package until the capability has a real internal boundary that justifies another package.
- Generated files do not participate in line-count guidance.

## Module Contracts

- Keep module interfaces in `internal/modules/module.go`. `Module` is an alias of `HTTPModule`, not a separate contract.
{{ if eq .Computed.http_router_final "gin" -}}
- HTTP capabilities implement `modules.HTTPModule` with `Name() string` and `HTTP(r *gin.RouterGroup)`.
{{ else -}}
- HTTP capabilities implement `modules.HTTPModule` with `Name() string` and `HTTP() http.Handler`.
{{ end -}}
{{ if .Computed.enable_grpc_final -}}
- gRPC capabilities implement `modules.GRPCModule` with `GRPC(grpc.ServiceRegistrar)`; gRPC-only modules require neither `Name()` nor `HTTP()`.
{{ end -}}
- HTTP modules return their application mount path from `Name()`.

## HTTP

### Route Paths and Mounting

Keep these three path layers distinct:

| Layer | Owner | Example |
| --- | --- | --- |
| Module-local routes | The module's `HTTP` method | `/` (chi) or `""` (Gin) for the collection; `/{id}` (chi) or `/:id` (Gin) for one record |
| Application mount path | `internal/app` and the shared router, using `Name()` | `Name()` returns `/game`, yielding `/game` and `/game/{id}` (chi) or `/game/:id` (Gin) |
| Deployed gateway path | Nginx configuration | The external path may retain or rewrite the application path; see the [deployment examples in the root guidelines](../../AGENTS.md#authentication-and-authorization) |

- Use singular resource nouns in the module mount path and nested resources. Register relative routes without repeating the module mount path or adding a deployment prefix.
- Do not impose an `/api` or version prefix inside the module.

### Router Integration

{{ if eq .Computed.http_router_final "gin" -}}
- Register routes on the provided `*gin.RouterGroup` inside `http.go`; the application owns the single Gin engine and the module prefix.
- Apply capability-wide middleware with `Use`, nested resource middleware with `Group`, and route-specific middleware before the final handler.
- Use `c.Param` for path parameters and `c.Request.Context()` for downstream operations. Keep `*gin.Context` out of business methods.
- Call `c.Abort()` when middleware rejects a request; writing an error alone does not stop subsequent handlers.
{{ else -}}
- Construct module routes with `chi.NewRouter` inside `http.go`.
- Apply capability-wide middleware with `Use`, route-specific middleware with `With`, and nested resource middleware with `Route`.
{{ end -}}
- Use literal route paths and inline groups inside `HTTP` so `make api` can discover complete paths and final handlers.

### Input, Responses, and Errors

- Parse transport input and map transport output at the HTTP boundary; keep reusable business decisions in non-transport functions.
- Follow the [root response-format rules](../../AGENTS.md#responses-and-errors) and use the shared JSON helpers.
- Translate an expected missing record (`ErrNotFound`, including mapped `sql.ErrNoRows`) into HTTP 404 using `server.WriteError(w, r, http.StatusNotFound, err)` (or `c.Writer` for Gin). The body contains stable `error_code` and localized `msg`.
- HTTP 404 covers missing records and unmatched routes. Do not convert unexpected SQL/connection errors into a missing-record result; retain HTTP 500 for those failures.
- Propagate the request context and stop work when it is canceled.
- Use PATCH for partial updates: the module-local item route is `/{id}` (chi) or `/:id` (Gin), mounted under the resource path by the application. Preserve omitted fields; use pointer fields in HTTP inputs{{ if .Computed.enable_grpc_final }} and Proto `optional` fields{{ end }} to distinguish omission from an explicit empty value.

### Idempotency

- Modules that use `x-idempotent` for POST create operations must implement `modules.IdempotentHTTPModule`; clients reuse one key for retries of the same logical creation. Repeated keys return HTTP 409 as specified by the root guidelines.

## Naming and Pagination

### Method Naming

- Follow the [root CRUD naming rules and AIP references](../../AGENTS.md#crud-and-pagination). Resource-qualified names such as `Get<Resource>` and `List<Resources>` describe API operations; within a resource-scoped Go package, use short business method names such as `Get` and `List`.
{{ if .Computed.enable_grpc_final -}}
- RPC methods use the resource-qualified names. Requests use `<Method>Request`; list responses use `<Method>Response`. Get/Create/Update return the resource; Delete returns `google.protobuf.Empty`.
{{ end -}}

### Pagination

- Inject `pagination.Limits` from `conf/pagination.yml` into business modules and use the same limits in generated documentation. HTTP validates parameter presence and format; business methods apply page-number and page-size limits.
- In both chi and Gin handlers, parse the query once with `query, err := url.ParseQuery(r.URL.RawQuery)`; in Gin, obtain `r` from `c.Request`. Return HTTP 400 on a parsing error and do not use the partial result.
- Inspect `query["p"]` and `query["s"]` directly to distinguish omission from an explicitly supplied value. Each supplied parameter must have exactly one value that parses with `strconv.ParseInt(value, 10, 32)`. Reject duplicates, empty strings, non-integers, and integer overflow with HTTP 400. Use the same parsed `query` for other query fields.
- Use Go pointers{{ if .Computed.enable_grpc_final }} and Proto `optional` fields{{ end }} to preserve presence. Omitted `p`/`s` select page 1 and size `min(1, maxS)`. Explicit zero, negative, or configured out-of-range `int32` values return an empty list without querying the database; these are range results, not format errors.
- Order paginated results consistently. Paginated HTTP responses expose the total count as `t`. Pages beyond the data return an empty list with the matching total count; configured out-of-range requests return `t: 0`. Consider [cursor pagination](https://google.aip.dev/158) when deep offsets become expensive.

## Testing

- Prioritize business rules, permission boundaries, state transitions, and failure behavior over tests written only to raise coverage.
- Test exported behavior and HTTP routes with `net/http/httptest`.
- Add database integration coverage when module behavior depends on SQL semantics or constraints.
- Cover an existing record, a missing record (404 with message), an unmatched route (404), invalid input (400), and a database failure (500) in HTTP tests.
