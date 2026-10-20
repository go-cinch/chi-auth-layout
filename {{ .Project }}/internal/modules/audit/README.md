# Operation audit

Independent Go Cinch implementation; no code or assets from scaffold-master.

`GET /audit/{login|operation}` lists append-only HTTP audit metadata. Companion
routes: `/summary`, `/option`, `/detail/{id}`, `/export` (CSV). The concrete routes
are registered independently for permission evaluation. Login and operation read
and export actions are independent; detail permissions never imply export access.
The existing super action grants access, while other roles require explicit grants.

Filters: `q` (literal case-insensitive account/IP/path/trace substring), `module`,
`result` (`success`/`failure`), `from`/`to` (Unix ms, inclusive/exclusive),
`time_zone` (IANA timezone); list uses existing `p`/`s` semantics and `{items,t,p,s}`.
Omitting both bounds selects the last seven days. A supplied `from` or `to` constrains only that side; omitted boundaries remain absent. Paired ranges and chart allocations span at most 90 days, and timestamps must not exceed year 9999. For a single-bound chart, matching event extents determine only the chart allocation; summary `from`/`to` retain null for the omitted bound. Blank `q` values are ignored after trimming; nonblank keywords use case-insensitive literal substring matching. CSV exports at most 10,000
matching records and rejects larger results rather than silently truncating.
Untrusted spreadsheet cell prefixes are neutralized.

`t_audit_event` stores request start (Unix milliseconds), verified actor ID when available,
account (attempted account on failed login), HTTP outcome, method/path, resolved client IP,
user agent, trace ID, safe error code and duration.

Both login and operation events include a dedicated `header` JSON field containing
request headers as seen by the service before handlers execute, including `Host`
(which Go stores separately). Single values are strings; repeated values are arrays.
The three IP headers (`X-Verified-Client-IP`, `X-Real-IP`, `X-Forwarded-For`) are retained
for investigation. The resolved `ip` field accepts one valid `X-Verified-Client-IP`
from any connection, falling back to the transport peer for missing, invalid or
repeated values. Kubernetes/network configuration enforces the trusted sources;
auth has no proxy allowlist and does not use the other IP headers for resolution.

Sensitive header names use the shared redaction policy plus `redact.keys` overrides.
Authorization, proxy authorization, cookies, API keys, tokens, sessions, signatures,
CSRF/XSRF, JWT and baggage values are replaced completely with `[redacted]`.
URL-valued headers such as Referer and X-Original-URI also hide credentials, sensitive
query parameters and fragments. Embedded JSON values use recursive redaction.
Other custom sensitive header names must be added to `redact.keys`.
Snapshots are bounded to 64 KiB; oversized snapshots contain an explicit `_omitted`
marker. Records without a snapshot have `header: null`. The initial auth schema
includes the header column and its object-or-null constraint.

Operation events also include bounded, recursively sanitized request query/body and
response parameters. Challenge, credential, proof, JWK, password, token and configured
custom keys are hidden. Files are represented by `file`; unstructured, invalid or
oversized bodies are explicitly omitted. Each captured side is limited to 64 KiB.
Login request and response bodies are never captured.

Success means HTTP 200–399. Login records include restricted first-login sessions
and failed credentials/challenges submitted to the final login endpoint. Setup
challenges, captcha refresh, token refresh and audit reads are excluded.
Operations include HTTP writes in user/role/action/user-group/dictionary/
msg and registration/password changes/authenticated logout. RPC calls are not
captured by this HTTP observer. There is no user-facing edit/delete endpoint.

The HTTP middleware persists after handling the request with a detached one-second
budget. Persistence failures emit an error without replacing the business response;
there is no durable retry queue. Retention/archival is not automated in this version.

Summary, heatmap, options, table and CSV reuse identical filters. Trends use fixed
epoch-aligned hourly buckets for <=24h, otherwise daily buckets; the UI formats bucket instants
in its selected timezone. Heatmap cells expose independent `started_at`/`ended_at` intervals instead of
combining recurring weekdays. Ranges up to 24 hours use consecutive hourly bins;
shorter calendar ranges use local date/hour bins, and ranges above eight days
use local calendar days. `heatmap_granularity` identifies hour/day mode. All cells
are clipped to `from`/`to`, zero-filled and ordered chronologically. Calendar-day
boundaries follow the IANA timezone, including DST changes. Repeated failures means at least five
failures for an account within the selected scope, not a threat score.

Design references (interaction concepts only):
- https://auth0.com/docs/deploy-monitor/logs/view-log-events
- https://www.elastic.co/docs/solutions/security/advanced-entity-analytics/users-page
- https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/visualizations/heatmap/
- https://echarts.apache.org/examples/en/editor.html?c=heatmap-cartesian

PC UI uses the existing Vben EchartsUI/useEcharts plugin, its ECharts dependency,
Ant Design Vue cards/statistics/table/drawer and existing Go Cinch table controls.

## V2 investigation filters

`weekday` (Monday=0 through Sunday=6) and `hour` (0–23) must be supplied together.
They select every matching local wall-clock hour in `time_zone` within the original
`from`/`to` bounds, including both occurrences of a repeated DST hour. Heatmap,
selected-scope summary, records and CSV use the same filter implementation.
`username`, `ip` and `path` are exact filters for related-event investigation.
`sort` accepts `newest`, `failure` (failed requests first), and `duration` (slowest
first), with occurrence time and ID as deterministic ties. CSV uses the same order.
Summary includes `bucket_ms`, per-cell `failed`, and ranked `accounts`/`sources`.
The UI keeps the global overview separate from the selected scope and the record
filters, so its metrics do not silently change when a table filter is applied.

Summary breakdowns return at most ten ranked entries, including account and source-IP rankings.

Business timestamps use BIGINT Unix milliseconds in SQL and int64 in Go/HTTP. Calendar grouping alone uses the requested IANA timezone; `Local` is rejected. The complete schema, including audit headers, lives in `01-auth-schema`, while audit permission actions live in `03-action-default-data`. Shared localization messages live in `04-dictionary-default-data`.

Operation events include `operation`, `resource_type`, and `resource_id` when known. Request/response parameters describe the HTTP exchange; they are not a database before/after snapshot. Audit records retain actor/account snapshots after account deletion.

Headers and request/response payloads are returned by the detail endpoint only. List and CSV queries load metadata without headers or payloads, keeping large-page memory usage bounded by metadata rather than captured request data.

## Selected detail fields

Both `/audit/login/detail/{id}` and `/audit/operation/detail/{id}` accept `f`.
The selector includes every list metadata field plus `header`, `request_params`,
and `response_params`. For example, `f=id,occurred_at,status` omits all payload SQL
columns. Omitted `f` preserves the full detail response. Explicit selections return
only selected fields, retaining zero values and nulls. Empty, unknown, duplicate,
or repeated selectors and malformed query strings return HTTP 400 before SQL.
