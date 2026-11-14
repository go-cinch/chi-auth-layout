package auth

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
)

// Mutable retry/consumption state is deliberately excluded. All recipient and
// rendering inputs are signed; the account/language snapshot is taken at enqueue.
func (e *EmailCodes) deliveryMAC(id string, job emailAttempt) string {
	raw, _ := json.Marshal([]any{id, job.userID, job.recipient, job.purpose, job.created, job.expires, job.detail, job.account, job.language})
	return hex.EncodeToString(e.mac("delivery-v1", string(raw)))
}

func (e *EmailCodes) validDelivery(id string, job emailAttempt) bool {
	expected, _ := hex.DecodeString(e.deliveryMAC(id, job))
	actual, err := hex.DecodeString(job.signature)
	return err == nil && hmac.Equal(actual, expected)
}

// Caller holds the insertion transaction; workers cannot observe unsigned rows.
func (e *EmailCodes) sealEmail(ctx context.Context, id string) error {
	var job emailAttempt
	err := e.store.SQL(ctx).QueryRowContext(ctx, `SELECT verification_id, COALESCE(user_id,0), recipient, purpose, created_at, expired_at, notice_detail,
 COALESCE((SELECT username FROM t_user WHERE id=t.user_id),''),
 COALESCE((SELECT metadata->>'language' FROM t_user WHERE id=t.user_id),'')
 FROM t_auth_email_code t WHERE verification_id=$1 FOR UPDATE`, id).Scan(
		&id, &job.userID, &job.recipient, &job.purpose, &job.created, &job.expires, &job.detail, &job.account, &job.language)
	// Conditional notice inserts legitimately produce no row (no verified email).
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = e.store.SQL(ctx).ExecContext(ctx, `UPDATE t_auth_email_code SET delivery_account=$2, delivery_language=$3, delivery_mac=$4, updated_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint WHERE verification_id=$1`,
		id, job.account, job.language, e.deliveryMAC(id, job))
	return err
}

func warnEmailIntegrity(ctx context.Context, id any) {
	slog.WarnContext(ctx, "email delivery integrity check failed; skipped", "operation", "email_integrity", "outcome", "aborted", "job_id", id)
}
