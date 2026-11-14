package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const historyBatchSize = 500

// CleanupHistory removes expired temporary state, retaining active challenges,
// renewed cooldowns and scan batches still referenced by operation records.
// Each batch commits independently so cancellation/retries preserve progress.
func (e *EmailCodes) CleanupHistory(ctx context.Context) error {
	var now int64
	if err := e.store.SQL(ctx).QueryRowContext(ctx,
		`SELECT FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint`).Scan(&now); err != nil {
		return fmt.Errorf("read history cleanup time: %w", err)
	}
	cutoff := now - (time.Duration(e.config.HistoryRetentionDays) * 24 * time.Hour).Milliseconds()
	tables := []struct {
		name  string
		query string
	}{
		{"t_auth_email_code", `WITH expired AS (
            SELECT id FROM t_auth_email_code WHERE expired_at < $1
            ORDER BY expired_at, id LIMIT $2 FOR UPDATE SKIP LOCKED
        ) DELETE FROM t_auth_email_code t USING expired WHERE t.id = expired.id`},
		{"t_auth_email_cooldown", `WITH expired AS (
            SELECT id FROM t_auth_email_cooldown WHERE retry_at < $1
            ORDER BY retry_at, id LIMIT $2 FOR UPDATE SKIP LOCKED
        ) DELETE FROM t_auth_email_cooldown t USING expired WHERE t.id = expired.id`},
		{"t_sensitive_scan", `WITH expired AS (
            SELECT s.id FROM t_sensitive_scan s WHERE s.created_at < $1
                AND NOT EXISTS (SELECT 1 FROM t_sensitive_operation o WHERE o.scan_id = s.id)
            ORDER BY s.created_at, s.id LIMIT $2 FOR UPDATE OF s SKIP LOCKED
        ) DELETE FROM t_sensitive_scan t USING expired WHERE t.id = expired.id`},
	}
	for _, table := range tables {
		var total int64
		for {
			result, err := e.store.SQL(ctx).ExecContext(ctx, table.query, cutoff, historyBatchSize)
			if err != nil {
				return fmt.Errorf("clean history from %s: %w", table.name, err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("count history removed from %s: %w", table.name, err)
			}
			total += count
			if count < historyBatchSize {
				break
			}
		}
		slog.InfoContext(ctx, fmt.Sprintf("removed %d historical rows from %s before %d", total, table.name, cutoff),
			"operation", "auth_history_cleanup", "outcome", "success", "table", table.name,
			"deleted_count", total, "cutoff_at", cutoff)
	}
	return nil
}
