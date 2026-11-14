package auth

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const emailLease = 30 * time.Second

// A random ticket and a domain-separated keyed PRF reproduce the same OTP on
// redelivery without storing the OTP in job arguments or logs.
func (e *EmailCodes) verificationCode(id string) string {
	if e.config.Debug {
		return "000000"
	}
	digest := e.mac("delivery-otp", id)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint64(digest[:8])%1_000_000)
}

type emailAttempt struct {
	userID    int64
	signature string
	created   int64
	account   string
	language  string
	timezone  string
	detail    string
	purpose   string
	recipient string
	expires   int64
	number    int
	wait      time.Duration
}

func (e *EmailCodes) claimEmail(ctx context.Context, id string) (emailAttempt, error) {
	var job emailAttempt
	err := e.store.Tx(ctx, func(txCtx context.Context) error {
		now := time.Now().UnixMilli()
		conn := e.store.SQL(txCtx)
		var tries int
		var lease int64
		err := conn.QueryRowContext(txCtx, `SELECT recipient, expired_at, delivery_attempts, delivery_after, purpose, created_at, notice_detail,
            delivery_account, delivery_language, COALESCE(user_id,0), delivery_mac,
            COALESCE((SELECT u.metadata->>'timezone' FROM t_user u WHERE u.id=t.user_id), '')
			FROM t_auth_email_code t WHERE verification_id = $1 AND delivery_attempts IS NOT NULL
			AND NOT delivered AND NOT consumed AND expired_at > $2
			AND user_id IS NOT NULL AND recipient <> '' FOR UPDATE`, id, now).Scan(&job.recipient, &job.expires, &tries, &lease, &job.purpose, &job.created, &job.detail, &job.account, &job.language, &job.userID, &job.signature, &job.timezone)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !e.validDelivery(id, job) {
			warnEmailIntegrity(txCtx, id)
			_, err = conn.ExecContext(txCtx, `UPDATE t_auth_email_code SET consumed=TRUE, updated_at=$2 WHERE verification_id=$1`, id, now)
			return err
		}
		if lease > now {
			job.wait = time.Duration(min(lease, job.expires)-now) * time.Millisecond
			return nil
		}
		if tries >= e.config.SendLimit {
			_, err = conn.ExecContext(txCtx, `UPDATE t_auth_email_code SET consumed = TRUE, updated_at = $2 WHERE verification_id = $1`, id, now)
			return err
		}
		job.number = tries + 1
		_, err = conn.ExecContext(txCtx, `UPDATE t_auth_email_code SET delivery_attempts = $2, delivery_after = $3, updated_at = $4 WHERE verification_id = $1`,
			id, job.number, now+emailLease.Milliseconds(), now)
		return err
	})
	return job, err
}

func waitEmail(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (e *EmailCodes) handleEmail(ctx context.Context, id string) error {
	for ctx.Err() == nil {
		job, err := e.claimEmail(ctx, id)
		if err != nil {
			if ctx.Err() == nil {
				slog.ErrorContext(ctx, "could not claim OTP delivery", "operation", "otp_claim", "outcome", "failure", "error", err)
			}
			if !waitEmail(ctx, time.Second) {
				return ctx.Err()
			}
			continue
		}
		if job.wait > 0 {
			if !waitEmail(ctx, job.wait) {
				return ctx.Err()
			}
			continue
		}
		if job.number == 0 {
			return nil
		}
		accepted := e.sendAttempt(ctx, id, job)
		if e.finishEmail(ctx, id, job.number, accepted) {
			return nil
		}
	}
	return ctx.Err()
}

// After SMTP acceptance, retry a failed database write without sending again.
// A reclaimed task can only finalize the attempt it actually reserved.
func (e *EmailCodes) finishEmail(ctx context.Context, id string, attempt int, accepted bool) bool {
	exhausted := !accepted && attempt >= e.config.SendLimit
	for {
		now := time.Now()
		var nextAt int64
		if !accepted && !exhausted {
			nextAt = now.Add(time.Second << min(attempt-1, 6)).UnixMilli()
		}
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		result, err := e.store.SQL(writeCtx).ExecContext(writeCtx, `UPDATE t_auth_email_code SET
			delivered = $3, consumed = $4, delivery_after = $5, updated_at = $6
			WHERE verification_id = $1 AND delivery_attempts = $2 AND NOT consumed AND NOT delivered`,
			id, attempt, accepted, exhausted, nextAt, now.UnixMilli())
		cancel()
		if err == nil {
			var changed int64
			changed, err = result.RowsAffected()
			if err == nil {
				if changed == 0 {
					return false
				} // recheck the ticket before acknowledging
				if !accepted {
					slog.WarnContext(ctx, "OTP delivery attempt failed", "operation", "otp_delivery", "outcome", "failure", "attempt", attempt, "exhausted", exhausted)
				}
				return accepted || exhausted
			}
		}
		slog.ErrorContext(ctx, "could not record OTP delivery", "operation", "otp_finalize", "outcome", "failure", "error", err)
		if !waitEmail(ctx, time.Second) {
			return false
		}
	}
}

func (e *EmailCodes) sendAttempt(ctx context.Context, id string, job emailAttempt) bool {
	sendCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(job.expires))
	defer cancel()
	if sendCtx.Err() != nil {
		return false
	}
	result := make(chan error, 1)
	go func() {
		subject, body, html, err := e.deliveryMessage(sendCtx, id, job)
		if err != nil {
			// Do not log values, rendered content, or database errors containing them.
			slog.WarnContext(sendCtx, "email template rendering failed", "operation", "email_template_render", "outcome", "failure", "purpose", job.purpose)
			result <- err
			return
		}
		result <- e.sender.SendHTML(sendCtx, job.recipient, subject, body, html)
	}()
	heartbeat := time.NewTicker(emailLease / 3)
	defer heartbeat.Stop()
	for {
		select {
		case err := <-result:
			return err == nil
		case <-sendCtx.Done():
			// Join the SMTP call: an acknowledgement still wins over cancellation.
			return <-result == nil
		case <-heartbeat.C:
			writeCtx, stop := context.WithTimeout(sendCtx, 3*time.Second)
			res, err := e.store.SQL(writeCtx).ExecContext(writeCtx, `UPDATE t_auth_email_code SET delivery_after = $3, updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
				WHERE verification_id = $1 AND delivery_attempts = $2 AND NOT consumed AND NOT delivered`, id, job.number, time.Now().Add(emailLease).UnixMilli())
			stop()
			if err == nil {
				if n, countErr := res.RowsAffected(); countErr == nil && n == 0 {
					cancel()
				}
			}
		}
	}
}
