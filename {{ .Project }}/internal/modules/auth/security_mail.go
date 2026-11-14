package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/action"

	"github.com/lib/pq"
)

type AlertTier struct {
	Window time.Duration
	Count  int
}

func validateAlertTiers(tiers []AlertTier) error {
	for i, t := range tiers {
		if t.Window < time.Millisecond || t.Window%time.Millisecond != 0 || t.Count < 1 ||
			(i > 0 && (t.Window < tiers[i-1].Window || t.Count <= tiers[i-1].Count)) {
			return errors.New("auth.email.alertTiers requires positive, ascending windows and strictly increasing counts")
		}
	}
	return nil
}

func (e *EmailCodes) recordSensitiveOperation(tx context.Context, userID int64, operation string) error {
	if len(e.config.AlertTiers) == 0 {
		return nil
	}
	conn := e.store.SQL(tx)
	// Order each actor's events by commit, with no contention between different users.
	if _, err := conn.ExecContext(tx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 75016))`, fmt.Sprint(userID)); err != nil {
		return err
	}
	kind, permissionRelated := sensitiveOperationCategory(operation)
	_, err := conn.ExecContext(tx, `INSERT INTO t_sensitive_operation(user_id,operation_kind,permission_related) VALUES($1,$2,$3)`, userID, kind, permissionRelated || action.IsPermissionMutation(tx))
	return err
}

// Notice rows are a transactional outbox. SMTP and retries run after commit;
// recovery and unlink transitions can complete while the transport is offline.
func (m *Module) enrollmentNotice(ctx context.Context, uid int64) error {
	if !m.sensitiveEmailEnabled() {
		return nil
	}
	id, err := randomSessionValue()
	if err != nil {
		return err
	}
	_, err = m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_auth_email_code
        (verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts)
        SELECT $1,'',CASE WHEN totp_recovery_pending THEN 'totp_rebound' ELSE 'totp_bound' END,'',id,email,credential_version,'',$2,0
        FROM t_user WHERE id=$3 AND email IS NOT NULL AND email_verified_at IS NOT NULL`, id, time.Now().Add(24*time.Hour).UnixMilli(), uid)
	if err != nil {
		return err
	}
	return m.emailCodes.sealEmail(ctx, id)
}

func (e *EmailCodes) unlinkNotice(ctx context.Context, uid, version int64, address string) error {
	id, err := randomSessionValue()
	if err != nil {
		return err
	}
	_, err = e.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_auth_email_code
        (verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts)
        VALUES ($1,'','unlink_notice','',$2,$3,$4,'',$5,0)`, id, uid, address, version, time.Now().Add(24*time.Hour).UnixMilli())
	if err != nil {
		return err
	}
	return e.sealEmail(ctx, id)
}

// Queue the confirmation in the binding transaction, using only the newly
// verified persisted address. Workers cannot send it before that transaction commits.
func (e *EmailCodes) emailLinkedNotice(ctx context.Context, uid int64) error {
	id, err := randomSessionValue()
	if err != nil { return err }
	_, err = e.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_auth_email_code
        (verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts)
        SELECT $1,'','email_linked','',id,email,credential_version,'',$2,0
        FROM t_user WHERE id=$3 AND email IS NOT NULL AND email_verified_at IS NOT NULL`, id, time.Now().Add(24*time.Hour).UnixMilli(), uid)
	if err != nil { return err }
	return e.sealEmail(ctx, id)
}

// Each invocation atomically reserves a ten-minute database time slot, marks all
// visible pending events and writes at most one notice per actor. Inserts racing
// the statement snapshot remain pending, even when their IDs precede the batch.
func (e *EmailCodes) ScanSensitiveOperations(ctx context.Context) error {
	tiers := e.config.AlertTiers
	if len(tiers) == 0 {
		return nil
	}
	windows, counts := make([]int64, len(tiers)), make([]int64, len(tiers))
	for i, t := range tiers {
		windows[i], counts[i] = t.Window.Milliseconds(), int64(t.Count)
	}
	return e.store.Tx(ctx, func(tx context.Context) error {
		db := e.store.SQL(tx)
		var acquired bool
		if err := db.QueryRowContext(tx, `SELECT pg_try_advisory_xact_lock(1735290211, 4)`).Scan(&acquired); err != nil || !acquired {
			return err
		}
		var scanID int64
		err := db.QueryRowContext(tx, `INSERT INTO t_sensitive_scan(slot) VALUES (floor(extract(epoch FROM statement_timestamp())/600)::bigint)
            ON CONFLICT(slot) DO NOTHING RETURNING id`).Scan(&scanID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		// Count through each event millisecond using [event+1-window, event+1).
		// The ID cutoff separately orders events sharing the same millisecond.
		// Stop counting after threshold+1 even for a large burst. Count == threshold is a new crossing. While a higher tier is active,
		// a lower crossing is suppressed. ID breaks timestamp ties for bursts.
		rows, err := db.QueryContext(tx, `WITH pending AS (
            UPDATE t_sensitive_operation SET scan_id=$1,updated_at=floor(extract(epoch FROM statement_timestamp())*1000)::bigint
            WHERE scan_id IS NULL RETURNING id,user_id,created_at
        ), tiers AS (SELECT * FROM unnest($2::bigint[], $3::bigint[]) WITH ORDINALITY AS t(ms,minimum,rank)),
        observed AS (
            SELECT p.*, tier.*, sample.n FROM pending p CROSS JOIN tiers tier
            CROSS JOIN LATERAL (SELECT count(*) n FROM (SELECT 1 FROM t_sensitive_operation h
                WHERE h.user_id=p.user_id AND h.created_at>=p.created_at+1-tier.ms AND h.created_at<p.created_at+1
                  AND (h.created_at,h.id)<=(p.created_at,p.id) LIMIT tier.minimum+1) bounded) sample
            WHERE sample.n >= tier.minimum
        ), highest_active AS (
            SELECT DISTINCT ON(id) * FROM observed ORDER BY id,rank DESC
        ), highest_crossing AS (
            SELECT DISTINCT ON(user_id) * FROM highest_active WHERE n=minimum ORDER BY user_id,rank DESC,created_at DESC,id DESC
        ), summary_windows AS (
            SELECT x.*, endpoint.created_at+1 AS ended_at, endpoint.id AS end_id
            FROM highest_crossing x
            CROSS JOIN LATERAL (
                SELECT o.created_at,o.id FROM observed o
                WHERE o.user_id=x.user_id AND o.rank=x.rank
                    AND (o.created_at,o.id)>=(x.created_at,x.id)
                ORDER BY o.created_at DESC,o.id DESC LIMIT 1
            ) endpoint
        )
        INSERT INTO t_auth_email_code
            (verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts,notice_detail)
        SELECT 'activity-' || $1::bigint::text || '-' || x.user_id,'','sensitive_notice','',x.user_id,u.email,u.credential_version,'',
            floor(extract(epoch FROM statement_timestamp())*1000)::bigint+86400000,0,
            jsonb_build_object('version',2,'started_at',x.ended_at-x.ms,'ended_at',x.ended_at,
                'create_count',summary.created,'delete_count',summary.deleted,
                'update_count',summary.updated,'permission_count',summary.permissions,'other_count',summary.other)::text
        FROM summary_windows x JOIN t_user u ON u.id=x.user_id
        CROSS JOIN LATERAL (
            SELECT count(*) FILTER (WHERE h.operation_kind='create') AS created,
                count(*) FILTER (WHERE h.operation_kind='delete') AS deleted,
                count(*) FILTER (WHERE h.operation_kind='update') AS updated,
                count(*) FILTER (WHERE h.permission_related) AS permissions,
                count(*) FILTER (WHERE h.operation_kind='other') AS other
            FROM t_sensitive_operation h
            WHERE h.user_id=x.user_id AND h.created_at>=x.ended_at-x.ms AND h.created_at<x.ended_at
                AND (h.created_at,h.id)<=(x.ended_at-1,x.end_id)
        ) summary
        WHERE u.email_verified_at IS NOT NULL AND u.email IS NOT NULL RETURNING verification_id`, scanID, pq.Array(windows), pq.Array(counts))
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err = e.sealEmail(tx, id); err != nil {
				return err
			}
		}
		_, err = db.ExecContext(tx, `DELETE FROM t_sensitive_operation WHERE scan_id IS NOT NULL AND created_at <
            LEAST(floor(extract(epoch FROM statement_timestamp())*1000)::bigint,
                  COALESCE((SELECT min(created_at) FROM t_sensitive_operation WHERE scan_id IS NULL),9223372036854775807))-$1`, windows[len(windows)-1])
		return err
	})
}

// Count successful requests, not affected rows. Permission counts overlap CRUD totals.
func sensitiveOperationCategory(operation string) (kind string, permissionRelated bool) {
	method, target, ok := strings.Cut(operation, " ")
	if !ok {
		return "other", false
	}
	target = path.Clean(target)
	parts := strings.Split(strings.Trim(target, "/"), "/")
	root := parts[0]
	permissionRelated = root == "role" || root == "action" || root == "user-group" ||
		(root == "user" && ((method == "POST" && len(parts) == 1) || method == "DELETE"))
	switch method {
	case "DELETE":
		return "delete", permissionRelated
	case "PATCH", "PUT":
		return "update", permissionRelated
	case "POST":
		if len(parts) == 1 {
			switch root {
			case "user", "role", "action", "user-group", "dictionary", "mail-template":
				return "create", permissionRelated
			}
		}
		return "update", permissionRelated
	default:
		return "other", false
	}
}

type sensitiveActivityCounts struct {
	Version     int    `json:"version"`
	StartedAt   *int64 `json:"started_at,omitempty"`
	EndedAt     *int64 `json:"ended_at,omitempty"`
	Created     int64  `json:"create_count"`
	Deleted     int64  `json:"delete_count"`
	Updated     int64  `json:"update_count"`
	Permissions int64  `json:"permission_count"`
	Other       int64  `json:"other_count"`
}

func sensitiveActivitySummary(detail, language string) string {
	var counts sensitiveActivityCounts
	// Already queued, signed legacy notices remain deliverable without rewriting their payload.
	if json.Unmarshal([]byte(detail), &counts) != nil || (counts.Version != 1 && counts.Version != 2) {
		return detail
	}
	var lines []string
	if language == "en-US" {
		lines = []string{fmt.Sprintf("Created: %d", counts.Created), fmt.Sprintf("Deleted: %d", counts.Deleted), fmt.Sprintf("Updated: %d", counts.Updated)}
		if counts.Other > 0 {
			lines = append(lines, fmt.Sprintf("Other: %d", counts.Other))
		}
		lines = append(lines, fmt.Sprintf("Of these, permission-related: %d", counts.Permissions))
	} else {
		lines = []string{fmt.Sprintf("创建：%d 次", counts.Created), fmt.Sprintf("删除：%d 次", counts.Deleted), fmt.Sprintf("修改：%d 次", counts.Updated)}
		if counts.Other > 0 {
			lines = append(lines, fmt.Sprintf("其他：%d 次", counts.Other))
		}
		lines = append(lines, fmt.Sprintf("其中涉及权限：%d 次", counts.Permissions))
	}
	return strings.Join(lines, "\n")
}

// Persisted window bounds are the same [start, end) bounds used by the aggregate.
// Never substitute delivery time for an older notice with no recorded window.
func sensitiveActivityPeriod(detail, language, timezone string) (string, string) {
	unknown := "未记录"
	if language == "en-US" {
		unknown = "Not recorded"
	}
	var counts sensitiveActivityCounts
	if json.Unmarshal([]byte(detail), &counts) != nil || counts.Version != 2 ||
		counts.StartedAt == nil || counts.EndedAt == nil || *counts.StartedAt > *counts.EndedAt {
		return unknown, unknown
	}
	location := emailTimeZone(timezone)
	return time.UnixMilli(*counts.StartedAt).In(location).Format(time.RFC3339),
		time.UnixMilli(*counts.EndedAt).In(location).Format(time.RFC3339)
}
