package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/code"
)

var (
	ErrInvalidRegistration = apperror.New("AUTH_INVALID_REGISTRATION", "invalid registration")
	ErrUsernameExists      = apperror.New("AUTH_USERNAME_EXISTS", "username already exists")
)

type RegisterInput struct {
	Username    string `json:"username" example:"operator"`
	Password    string `json:"password" example:"change-me"`
	SliderProof string `json:"slider_proof,omitempty"`
}

type UsernameAvailability struct {
	Available bool `json:"available"`
}

func (m *Module) Register(ctx context.Context, input RegisterInput) error {
	username := strings.TrimSpace(input.Username)
	if username == "" || strings.TrimSpace(input.Password) == "" {
		return ErrInvalidRegistration
	}
	input.Username = username
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash registration password: %w", err)
	}
	if m.defaultUserPermissions == nil {
		return errors.New("default user permission provider is unavailable")
	}
	return m.store.Tx(ctx, func(ctx context.Context) error {
		codes, err := m.defaultUserPermissions.DefaultUserCodes(ctx, nil)
		if err != nil {
			return err
		}
		var id int64
		if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT nextval('t_user_id_seq')`).Scan(&id); err != nil {
			return fmt.Errorf("allocate registered user id: %w", err)
		}
		// Broadcast eligibility compares this timestamp with database publication time.
		for range 5 {
			userCode, err := code.Generate()
			if err != nil {
				return fmt.Errorf("create registered user code: %w", err)
			}
			result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_user (id, created_at, updated_at, role_id, action, username, code, password, status, wrong) VALUES ($1, FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint, FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint, NULL, $5, $2, $3, $4, 0, 0) ON CONFLICT ON CONSTRAINT uk_user_code DO NOTHING`,
				id, input.Username, userCode, string(passwordHash), strings.Join(codes, ","))
			if err != nil {
				var postgresError *pq.Error
				if errors.As(err, &postgresError) && postgresError.Code == "23505" && (postgresError.Constraint == "uk_user_username" || postgresError.Constraint == "uk_user_login_alias") {
					return ErrUsernameExists
				}
				return fmt.Errorf("create registered user: %w", err)
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("create registered user result: %w", err)
			}
			if inserted == 1 {
				return nil
			}
		}
		return errors.New("create registered user: generate unique code")
	})
}

func (m *Module) UsernameAvailability(ctx context.Context, username string) (*UsernameAvailability, error) {
	normalized := strings.TrimSpace(username)
	if normalized == "" {
		return nil, ErrInvalidRegistration
	}
	username = normalized
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM t_user WHERE username = $1)`
	if m.emailCodes != nil {
		query = `SELECT EXISTS(SELECT 1 FROM t_user WHERE username = $1 OR email = LOWER(TRIM($1)))`
	}
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query, username).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check username availability: %w", err)
	}
	return &UsernameAvailability{Available: !exists}, nil
}
