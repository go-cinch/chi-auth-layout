package user

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

// A profile edit replaces ordinary metadata. Only the owning actions change these keys.
var userActionMetadataKeys = []string{"lock_expired_at", auth.PasswordChangeFailuresMetadataKey, "password_reset_skipped", "reject_register_reason"}

func (m *Module) requireEditControl(ctx context.Context, control string) error {
	if m.administration == nil {
		return authn.RequirePermissionOperation(ctx, nil, "user", "update")
	}
	return m.administration.RequireUserEditControl(ctx, control)
}

func (m *Module) Lock(ctx context.Context, id, expiresAt int64) (*User, error) {
	if expiresAt < 0 || expiresAt > 0 && expiresAt <= time.Now().UnixMilli() {
		return nil, ErrInvalid
	}
	return m.accountAction(ctx, id, "lock", func(tx context.Context) (sql.Result, error) {
		now := time.Now().UnixMilli()
		if expiresAt > 0 && expiresAt <= now {
			return nil, ErrInvalid
		}
		status, _ := lockStatusColumns("$3")
		return m.store.SQL(tx).ExecContext(tx, `UPDATE t_user SET status = 2, metadata = jsonb_set(metadata, '{lock_expired_at}', to_jsonb($2::bigint)), updated_at = $3 WHERE id = $1 AND `+status+" = 1", id, expiresAt, now)
	})
}

func (m *Module) Unlock(ctx context.Context, id int64) (*User, error) {
	return m.accountAction(ctx, id, "unlock", func(tx context.Context) (sql.Result, error) {
		return m.store.SQL(tx).ExecContext(tx, `UPDATE t_user SET status = 1, metadata = metadata - 'lock_expired_at', updated_at = $2 WHERE id = $1 AND status = 2 AND NOT `+expiredLockSQL("$2"), id, time.Now().UnixMilli())
	})
}

func (m *Module) Review(ctx context.Context, id int64, decision, reason string) (*User, error) {
	reason = strings.TrimSpace(reason)
	if (decision != "approve" && decision != "reject") || (decision == "reject") != (reason != "") {
		return nil, ErrInvalid
	}
	return m.accountAction(ctx, id, "approve", func(tx context.Context) (sql.Result, error) {
		if decision == "reject" {
			return m.store.SQL(tx).ExecContext(tx, `UPDATE t_user SET metadata = jsonb_set(metadata, '{reject_register_reason}', to_jsonb($2::text)), updated_at = $3 WHERE id = $1 AND status = 0`, id, reason, time.Now().UnixMilli())
		}
		return m.store.SQL(tx).ExecContext(tx, `UPDATE t_user SET status = 1, metadata = metadata - 'reject_register_reason', updated_at = $2 WHERE id = $1 AND status = 0`, id, time.Now().UnixMilli())
	})
}

func (m *Module) ResetPassword(ctx context.Context, id int64, password string) (*User, error) {
	password = strings.TrimSpace(password)
	if !validPassword(password) {
		return nil, ErrInvalid
	}
	user, err := m.accountAction(ctx, id, "reset-password", func(tx context.Context) (sql.Result, error) {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return nil, err
		}
		return m.store.SQL(tx).ExecContext(tx, `UPDATE t_user SET password = $2, credential_version = credential_version + 1, login_count = 0, admin_password_reset_pending = TRUE, metadata = metadata - 'password_change_failures' - 'password_reset_skipped', updated_at = $3 WHERE id = $1`, id, string(hash), time.Now().UnixMilli())
	})
	if err == nil && m.passwordFailures != nil {
		if clearErr := m.passwordFailures.Clear(ctx, id); clearErr != nil {
			slog.WarnContext(ctx, "clear password failures after administrator reset: "+clearErr.Error(), "user_id", id, "error", clearErr)
		}
	}
	return user, err
}

func (m *Module) accountAction(ctx context.Context, id int64, control string, apply func(context.Context) (sql.Result, error)) (*User, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	var user *User
	err := m.store.MutationTx(ctx, func(tx context.Context) error {
		if control == "approve" {
			if err := m.requireEditControl(tx, control); err != nil {
				return err
			}
		} else if err := authn.RequirePermissionEndpoint(tx, m.administration, http.MethodPost, "/user/"+strconv.FormatInt(id, 10)+"/"+control); err != nil {
			return err
		}
		if control != "reset-password" {
			action.MarkPermissionMutation(tx)
		}
		result, err := apply(tx)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			var exists bool
			if err := m.store.SQL(tx).QueryRowContext(tx, "SELECT EXISTS(SELECT 1 FROM t_user WHERE id = $1)", id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return ErrInvalid
			}
			return ErrNotFound
		}
		user, err = m.lookup(tx, m.store.SQL(tx), id, false)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("%s user: %w", control, err)
	}
	return user, nil
}
