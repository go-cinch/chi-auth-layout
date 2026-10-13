# Management summaries

The authenticated GET routes `/user/summary`, `/role/summary`,
`/user-group/summary`, `/action/summary`, `/dictionary/summary`,
`/whitelist/summary` and `/msg/sent/summary` return:

```json
{"total":0,"counts":{},"groups":[],"trend":[]}
```

These are global snapshots, independent of page size and record filters. Each
uses one SQL statement and returns initialized collections plus explicit zero
counts. Existing read-resource wildcard grants cover the new paths; default,
create/send, update and delete grants do not grant access to a summary.

| Resource | Counts | Groups |
| --- | --- | --- |
| user | pending, active, locked, recent_logged_in_users | none |
| role | with_permissions, without_permissions | action associations grouped by action group |
| user-group | memberships, unique_members, empty_groups | top 8 groups by member count; id breaks ties |
| action | resource, menu, button | all action groups with their record counts |
| dictionary | enabled, disabled | none |
| whitelist | permission, jwt | none |
| msg/sent | notice, system, all, targeted, latest_published_at | none |

Role assignments resolve against existing actions and count each role/action
association once. A wildcard permission is one associated record, not a measure
of its effective access scope. User-group membership totals count associations;
unique_members counts distinct users across all groups. Message statistics include
expired publications, matching the management list. Personal deletion and read
receipts do not change them; global deletion removes the publication.

## User calendar and drilldown

`/user/summary` accepts `days=14|30` (default 14) and `time_zone=<IANA name>`
(default UTC; `Local` is rejected). Other summary routes accept no query fields.

`trend` contains every local calendar date in the selected window, including
zero-count dates. Entries are `{date,from,to,count,recent_logged_in_users}`; instants are Unix milliseconds
and intervals are half-open `[from,to)`. Every point, including today, returns the
complete local calendar day from its midnight to the next midnight. These public
bounds are suitable for list drilldown and preserve the final day's milliseconds.
Midnight changes and DST follow the selected zone, without fixed 24-hour offsets.
The SQL aggregates separately use `min(last.to, captured_now + 1ms)` to exclude
future records; the public bounds are never clipped to that snapshot cutoff.
Status totals use that same captured instant to treat expired temporary locks as active;
summary and list reads never persist an unlock or alter updated_at.

`counts.recent_logged_in_users` counts distinct user accounts whose positive
`last_logged_in_at` falls within the selected window up to the captured request
cutoff, using the same 14/30-day and timezone window. Each user row contributes at most one, regardless
of repeated logins or `login_count`. Null, zero, out-of-window and future login
timestamps do not count. Account creation time is irrelevant: an old account with
a recent completed login counts. A password reset that sets `login_count` to zero
but retains a recent `last_logged_in_at` still counts that completed login.

Each daily point includes `recent_logged_in_users` even when it is zero. This is
the distribution of current users by their **most recent login date**, not daily
active users or login attempts. A user belongs to at most one daily bucket; the
sum of this series equals `counts.recent_logged_in_users`. A later login moves
that user from the old last-login date to the new date. Both series use the same
calendar boundaries, timezone, snapshot and zero-filled date sequence. Audit
records are not mixed into this metric. The new-user series remains `count`.

The user list accepts optional `created_from` / `created_to` and
`last_logged_in_from` / `last_logged_in_to`. Each pair is inclusive/exclusive,
allows either bound independently and accepts nonnegative JavaScript-safe Unix
milliseconds; supplied `from` must precede supplied `to`. Last-login filters
exclude NULL/zero (never logged in), including a to-only filter. The predicates
combine with status and other search filters and apply equally to the count and
page queries. Send the selected point's complete `from` / `to` directly; do not
construct a `23:59:59` end. Live lists may include newly added records after the
summary snapshot. The action list accepts `group_exact`, including an empty string, independently of its existing
substring group filter. The role list also accepts `group_exact`: a role matches
when any resolved action belongs to that exact, case-sensitive group. Omission
means no group restriction; an explicit empty string matches the empty group,
and an unknown group returns an empty list. Commas and wildcard characters are
literal group-name characters. The filter combines with name, word, action-code
and permission-status filters, counts each role once, and applies to both totals
and paginated rows. Repeated `group_exact` values return HTTP 400.
The role list additionally accepts `groups_all` as one JSON-encoded string array,
for example `groups_all=["Finance","Operations","Audit"]` (URL-encode the query
value). Each selected group must contain at least one resolved action belonging
to the role: selections combine with **AND**, not OR. Omitting the parameter or
sending `[]` adds no restriction; `[""]` selects the exact empty group. Group
names remain case-sensitive and untrimmed, including literal commas and wildcard
characters. Duplicate names are ignored. This filter combines with `group_exact`
and all other role filters and applies identically to totals and paginated rows
without duplicating roles. Repeated `groups_all` parameters, malformed JSON,
non-array values, `null`, and any non-string/null array element return HTTP 400.
Role and user-group lists accept `has_permissions=true|false`
using resolved action associations. User-group lists also accept
`has_members=true|false`. These predicates apply to both totals and paginated rows.
The sent-message list accepts `q` for escaped case-insensitive substring matching
against title or content. Duplicate and malformed new query values are rejected.

SQL, timezone, HTTP, imported OpenAPI-schema and permission tests accompany the
implementation. PostgreSQL integration uses CHI_AUTH_TEST_DSN and creates isolated
per-test schemas; it never modifies application tables outside those schemas.

The action list also accepts `groups_exact`, a JSON array of exact group names.
Selected groups are combined with OR; other filters are combined with AND.
Empty arrays remove this restriction, while an empty string selects ungrouped actions.
Names retain spaces, commas and wildcard characters literally. Non-arrays, null
values, non-string elements and repeated parameters return HTTP 400. Both count
and paginated rows use the same predicate.
