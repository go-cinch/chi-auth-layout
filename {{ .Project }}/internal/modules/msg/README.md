# Personal message status

`GET /msg/unread-count` reads `t_user_msg_status` by its unique `user_id` index.
The existing response (`count`, `latest_unread_at`, `observed_at`, `user_id`)
is unchanged. Normal polling does not join messages or aggregate inbox rows.

The status table stores:

| Column | Meaning |
| --- | --- |
| `user_id` | Unique owner; cascades when the user is deleted |
| `unread_count` | Visible, unread, undeleted, unexpired messages |
| `latest_unread_at` | Latest publication in that set; null when empty |
| `next_refresh_at` | Next unread publication or expiry; null when none |

`t_msg` and sparse `t_msg_recipient` rows remain the source of truth. Statistics are rebuildable from messages and receipts without changing read receipts. New
users initialize their row on first access or delivery.

Publication, visibility, receipt timestamps, expiry validation, and refresh
checks use PostgreSQL's Unix-millisecond statement time, not the application host clock.
User registration timestamps use the same database clock for broadcast eligibility.
Statements after lock acquisition use a fresh time (not transaction start time).
Future publications from older writers are excluded from the current count but
retained as refresh deadlines, so polling and the worker discover them once due.
Rebuilding a missing statistics row preserves these deadlines too.

```text
send:
  begin transaction
  lock affected statistics; repair newly initialized/overdue rows
  insert publication and targeted recipients
  increment count; advance latest timestamp; update earliest expiry
  commit both delivery and statistics

read / personal delete / read-all / delete published message:
  begin transaction
  lock affected statistics before changing receipts/publications
  perform mutation and recalculate affected statistics
  commit together (including idempotent receipts and full rollback on failure)

status(user):
  read one persisted row
  if absent or next_refresh_at <= now:
    transaction: lock, recheck deadline, repair only if still due, reread
  return count, latest timestamp and server observation time

background refresh:
  every 5 seconds select up to 100 due users using the refresh index
  transaction: lock in user-ID order; repair only still-due rows
  each sweep has a 4-second deadline and stops with the application
```

Concurrent targeted operations share a PostgreSQL transaction advisory gate and
lock status rows in ascending user-ID order. Broadcast/global deletion takes the
exclusive gate so newly initialized rows and concurrent personal operations
cannot miss a publication or overwrite each other's statistics. Ordinary status
reads take neither advisory nor row locks. Multiple replicas use the same gate.
An overdue row is repaired before an API response even when the background
worker is delayed or restarting.

Broadcast delivery now writes one status row per eligible existing user, without
materializing broadcast recipient rows. Sending uses counter increments, rather
than recomputing every user's entire history. Global deletion may recalculate
all users; personal changes only recalculate the affected user. This trades
write cost for cheap repeated reads. No Redis result cache is required.

Writers must use these module transactions. Direct SQL imports or a mixed
deployment containing old writers must rebuild affected status rows before
serving them; deploy the migration and updated writers together.

Clients merge concurrent status requests per account. A successful message
mutation invalidates the in-flight generation before its promise resolves. All
waiters discard an invalidated response and share a new request. Completed
responses are not reused as a TTL cache; remote sends and expiry remain visible
to the existing foreground polling. Visits stored locally only acknowledge the
indicator and do not alter persisted unread counts or receipts.
