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
The default range is seven days; maximum is 90 days, and timestamps must not exceed year 9999. CSV exports at most 10,000
matching records and rejects larger results rather than silently truncating.
Untrusted spreadsheet cell prefixes are neutralized.

`t_audit_event` stores request start (Unix milliseconds), verified actor ID when available,
account (attempted account on failed login), HTTP outcome, method/path, peer IP,
user agent, trace ID, safe error code and duration. Operation events include bounded, recursively sanitized request query/body and response parameters. Sensitive keys use the shared redaction policy and are replaced completely with `[redacted]`; challenge, credential, proof, JWK, password, token and configured custom keys are hidden. Headers are never captured. Files are represented by `file`; unstructured, invalid or oversized bodies are explicitly omitted. Each captured side is limited to 64 KiB. Login events remain metadata-only. Forwarded IP headers
are not trusted; when proxied the recorded IP is the proxy peer.

Success means HTTP 200–399. Login records include restricted first-login sessions
and failed credentials/challenges submitted to the final login endpoint. Setup
challenges, captcha refresh, token refresh and audit reads are excluded.
Operations include HTTP writes in user/role/action/user-group/dictionary/whitelist/
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

Business timestamps use BIGINT Unix milliseconds in SQL and int64 in Go/HTTP. Calendar grouping alone uses the requested IANA timezone; `Local` is rejected. The schema lives in `01-auth-schema`, while audit permission actions live in `02-auth-default-data`.

Operation events include `operation`, `resource_type`, and `resource_id` when known. Request/response parameters describe the HTTP exchange; they are not a database before/after snapshot. Audit records retain actor/account snapshots after account deletion.

Request and response payloads are returned by the detail endpoint only. List and CSV queries load metadata without payloads, keeping large-page memory usage bounded by metadata rather than captured bodies.
