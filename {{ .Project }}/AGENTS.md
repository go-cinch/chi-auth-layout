# Development Guidelines

## Scoped Guidelines

- Follow [business module guidelines](internal/modules/AGENTS.md) when changing business modules or module contracts, including routing, authentication/authorization, response handling, application wiring, and API documentation generation in `internal/common/server`, `internal/app`, or `internal/cmd`.
- Follow [database guidelines](internal/infra/db/AGENTS.md) when changing database infrastructure, schemas, or migrations. Business code using transactions must also follow their [transaction rules](internal/infra/db/AGENTS.md#transactions).

## Architecture and Dependencies

- Keep `cmd/*` thin: parse flags, initialize the application, and run it.
- Put process wiring and lifecycle code in `internal/app`.
- Put reusable technical infrastructure in `internal/common` and `internal/infra`; keep capability-specific behavior in business packages.
- Organize business code by capability under `internal/modules/<capability>`.
- Do not introduce global `handler`, `service`, `repository`, or `model` folders.
- `internal/common` must not import concrete business packages under `internal/modules/<capability>`. HTTP server infrastructure{{ if .Computed.enable_grpc_final }} and RPC server infrastructure{{ end }} may depend on the shared interfaces in `internal/modules/module.go`; follow the [module contracts](internal/modules/AGENTS.md#module-contracts).
- Business modules may depend on common infrastructure and small interfaces.
- Wire dependencies explicitly in `internal/app`; avoid package globals.
- Propagate `context.Context` through database, Redis, and downstream calls; stop work when the context is canceled.

## Naming and Data Conventions

### Identifiers and JSON Keys

- Exported Go struct fields use upper camel case; locals, parameters, and unexported fields use lower camel case. Preserve standard initialisms: `UserID`, `HTTPServer`, `userID`, `apiURL`.
- Database columns and all HTTP request/response and database JSON keys use snake case, including nested keys. Keep JSON names identical across HTTP and database boundaries; do not add camel case aliases or recursive key conversion.
- Map Go fields to JSON names with tags, for example `UserID int64` with `json:"user_id"`.

### Timestamps and Numeric Representation

- Represent instants as Unix milliseconds: `int64` in Go{{ if .Computed.enable_grpc_final }} and Proto{{ end }}, numbers in HTTP JSON, and `BIGINT` in SQL. Use `time.Now().UnixMilli()` or `FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint`; do not explicitly convert stored or transmitted values to UTC. Apply timezones only for calendar grouping or display.
- Name event timestamps with a past participle plus `At`: `CreatedAt`, `UpdatedAt`, `StartedAt`, `EndedAt`; use their snake-case equivalents across JSON, SQL{{ if .Computed.enable_grpc_final }}, Proto{{ end }}, configuration, examples, and tests. Do not mix `started_at`/`ended_at` with `starts_at`/`ends_at` or `start_at`/`end_at`.
- HTTP JSON uses numbers.{{ if .Computed.enable_grpc_final }} ProtoJSON uses decimal strings for `int64`.{{ end }} Convert to JavaScript `Number` only within its safe integer range (±(2^53−1)); otherwise use `BigInt` or lossless JSON parsing.

### Error Naming

- Prefix package-level errors with `Err` when exported and `err` otherwise, except in `internal/common/apperror`, which omits the prefix: `auth.ErrLoginFailed`, `apperror.InvalidBody`. Local error results use `err` or descriptive names such as `parseErr`.
- Public error codes use uppercase snake case and describe both the scope and the failure, such as `HTTP_INVALID_USER_ID` or `AUTH_LOGIN_FAILED`; avoid field-only codes such as `HTTP_USER_ID`. Keep translation catalog keys, tests, and API documentation in sync when codes change.

## Go Style

- Separate functions and methods with exactly one blank line, placed before any attached documentation comments. Short bodies may stay on one line. Preserve intentional whitespace in strings and test fixture source; do not add a lint check for this convention.

{{ if .Computed.enable_grpc_final -}}
## Protocols and RPC Infrastructure

- Store Proto contracts in `api/<service>-proto/`, with `<service>.proto` as the service entry file; generate Go bindings into `api/<service>/`. For published contracts, keep field numbers stable and reserve removed fields.
- Format Proto files with two-space indentation and a blank line between top-level definitions; see the [Proto style guide](https://protobuf.dev/programming-guides/style/).
- Use service-specific Proto packages, such as `catalog.v1`. Preserve imported contracts and paths; `make gen` maps their Go imports to the local module. Shared Google contracts belong in `third_party`.
- Start gRPC only when business modules register it.
- Configure address, unary timeout, shutdown deadline and reflection in `conf/grpc.yml`. Streams use caller deadlines; graceful shutdown has a forced-stop fallback.
- Initialize downstream clients with `rpc.NewClient` in `internal/app/client.go`, using `conf/client.yml`. Store typed clients on `Application`, inject through constructors, and register cleanup for shutdown and startup failures.
- Client unary calls default to 5 seconds and preserve shorter deadlines. Optional health checks require `SERVING`; otherwise initialization is lazy. Clients use TLS by default; set `Insecure: true` for plaintext.

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

- Configure `<environment>:<service>:` through `redis.prefix` in `conf/redis.yml` or `SERVICE_REDIS_PREFIX`; the default is `dev:auth:`. Replicas of one deployment share a prefix; services and environments use distinct prefixes.
- Use `<environment>:<service>:<resource-type>:<subtype>:<id>`. Separate every segment, including IDs, with `:`; use lowercase kebab-case names and reserve opaque URL-safe values for ID segments. Correct: `prod:auth:point-captcha:slider-proof:<id>`. Never join IDs with `-`, `.`, or `_`.
- Pass relative keys such as `point-captcha:slider-proof:<id>` to `internal/infra/rds`; its client prefixes all key-bearing operations, including multi-key deletion and Lua `KEYS`. Do not repeat the prefix, environment, or service in business keys.
- Create all Redis connections, including cache connections, in `internal/infra/rds` using its prefix-aware client. Never expose the raw client, embed its unrestricted command API, or bypass it with `Do` or raw pipelines. Extend the shared client with key-isolation tests when adding commands.
- Lua accesses keys only through `KEYS`, never hardcoded names or `ARGV`. Values and script arguments remain unprefixed.

{{ end -}}
## Logging

- For all logs, put the event, outcome, and essential values in a concise `msg`. Include field names only to resolve ambiguity.
- Also emit values needed for filtering, aggregation, and alerting as structured slog fields, even when they appear in `msg`. Repeating these values is intentional: the message must remain readable, and queries must not depend on parsing its wording.
- Use stable snake-case field names, consistent meanings, and native JSON types. Status codes, counts, and durations must be numbers, not strings; put units in field names, such as `duration_ms`, rather than values such as `"1250ms"`.
- For business and lifecycle events, use `operation` as a stable snake-case identifier and `outcome` for the result (`success`, `failure`, or `aborted`). Put failure details in `error`; include relevant resource IDs or counts without dumping request/response objects.
- Keep fields at the top level. Add relevant secondary context without logging every variable, adding unnecessary wrapper objects, or emitting aliases for the same field.
- Let the shared slog setup supply standard metadata (`time`, `level`, `v`, `caller`) and trace correlation fields.
- Keep fixed prose lowercase, preserve the original case of values, and redact sensitive data before placing it in either `msg` or additional fields.

HTTP access logs in both Chi and Gin must include these structured fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `method` | string | HTTP request method, such as `GET`. |
| `status` | integer | Written HTTP response status, such as `500`; use `0` when a request is aborted before any response is written. |
| `duration_ms` | integer | Non-negative elapsed request time in milliseconds. |
| `route` | string | Matched route template in the router's syntax, such as `/user/{id}` or `/user/:id`; use `"-"` when no template is available. |
| `path` | string | Redacted request path, excluding the query string; use `"-"` when routing has not resolved the parameters needed for safe redaction, including unmatched requests and unresolved Chi mount/catch-all paths. |

- Build `msg` and its structured fields from the same captured values so they agree, including duration and redaction.
- Preserve `http.ErrAbortHandler` by rethrowing it after logging. Emit the access log with `outcome: "aborted"`, retaining a status already written or using `0` otherwise; never synthesize a successful response for an aborted request.
- Aggregate per-endpoint statistics by `method` and `route`; never substitute the actual request path for `route`. Keep useful client context such as `remote_addr` and `user_agent` as separate fields.
- Logging tests must verify required fields, native types, values, and redaction as well as message readability. Do not reject a structured field merely because its value also appears in `msg`.

{{ if .Computed.enable_grpc_final -}}
- RPC access and client-call logs use `method` for the fully qualified RPC method, integer `grpc_code`, string `grpc_status`, and integer `duration_ms`. Reserve `status` for HTTP response codes; include `client` on downstream call logs.

{{ end -}}

HTTP example (automatic metadata omitted):

```json
{
  "msg": "GET 500 1250ms /user/{id} /user/42",
  "method": "GET",
  "status": 500,
  "duration_ms": 1250,
  "route": "/user/{id}",
  "path": "/user/42",
  "remote_addr": "127.0.0.1:53441",
  "user_agent": "Mozilla/5.0"
}
```

This supports field-based queries such as `status = 500 AND duration_ms > 1000` without parsing `msg`.

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
