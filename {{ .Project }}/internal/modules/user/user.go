package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/code"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
)

var (
	ErrNotFound       = apperror.New("USER_NOT_FOUND", "user not found")
	ErrRoleNotFound   = apperror.New("USER_ROLE_NOT_FOUND", "user role not found")
	ErrActionNotFound = apperror.New("USER_ACTION_NOT_FOUND", "user action not found")
	ErrIDs            = apperror.New("USER_IDS", "provide 1-100 positive user ids")
	ErrInvalid        = apperror.New("USER_INVALID", "invalid user")
	ErrConflict       = apperror.New("USER_CONFLICT", "username already exists")
)

const (
	StatusPending int16 = iota
	StatusActive
	StatusLocked
)

type User struct {
	LoginCount     int64          `json:"login_count"`
	ID             int64          `json:"id"`
	CreatedAt      int64          `json:"created_at"`
	UpdatedAt      int64          `json:"updated_at"`
	Username       string         `json:"username"`
	Code           string         `json:"code"`
	Status         int16          `json:"status"`
	Metadata       map[string]any `json:"metadata"`
	Wrong          int64          `json:"wrong"`
	LastLoggedInAt *int64         `json:"last_logged_in_at,omitempty"`
	RoleID         *int64         `json:"role_id,omitempty"`
	Role           *RoleView      `json:"role,omitempty"`
	ActionCodes    []string       `json:"action_codes"`
	Actions        []ActionView   `json:"actions"`
}

type ActionView struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	Word      string `json:"word"`
	Resource  string `json:"resource"`
	Menu      string `json:"menu"`
	Button    string `json:"button"`
}

type RoleView struct {
	ID          int64        `json:"id"`
	CreatedAt   int64        `json:"created_at"`
	UpdatedAt   int64        `json:"updated_at"`
	Name        string       `json:"name"`
	Word        string       `json:"word"`
	ActionCodes []string     `json:"action_codes"`
	Actions     []ActionView `json:"actions"`
}

type Summary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Code     string `json:"code"`
}

type CreateUserInput struct {
	Username    string         `json:"username" example:"operator"`
	Password    string         `json:"password" example:"change-me"`
	RoleID      *int64         `json:"role_id,omitempty"`
	ActionCodes []string       `json:"action_codes,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateUserInput struct {
	Username    *string         `json:"username,omitempty"`
	Password    *string         `json:"password,omitempty"`
	RoleID      *int64          `json:"role_id,omitempty"`
	ActionCodes *[]string       `json:"action_codes,omitempty"`
	Status      *int16          `json:"status,omitempty"`
	Metadata    *map[string]any `json:"metadata,omitempty"`
}

type ListUsersInput struct {
	Username string
	Code     string
	Statuses []int16
	Page     *int32
	PageSize *int32
}

type ListUsersResult struct {
	Items    []User `json:"items"`
	Total    int64  `json:"t"`
	Page     int32  `json:"p"`
	PageSize int32  `json:"s"`
}

type Module struct {
	store                   *db.Store
	limits                  pagination.Limits
	registrationCredentials RegistrationCredentials
	passwordFailures        PasswordFailureClearer
	actions                 Actions
	roles                   Roles
	switches                authmodule.Switches
}

type RegistrationCredentials interface {
	OpenRegistrationPassword(context.Context, authmodule.EncryptedRegisterInput) (string, error)
}

type PasswordFailureClearer interface {
	Clear(context.Context, int64) error
}

type Actions interface {
	SplitCodes(string) []string
	Lookup(context.Context, db.SQLExecutor, []string) ([]actionmodule.Action, error)
	ValidateCodes(context.Context, db.SQLExecutor, []string) ([]string, error)
}

type Roles interface {
	LookupRecords(context.Context, db.SQLExecutor, []int64) (map[int64]rolemodule.Role, error)
	Lookup(context.Context, db.SQLExecutor, int64) (*rolemodule.Role, error)
	Exists(context.Context, db.SQLExecutor, int64) (bool, error)
}

func New(store *db.Store, limits pagination.Limits, credentials RegistrationCredentials, passwordFailures PasswordFailureClearer, actions Actions, roles Roles, switches authmodule.Switches) *Module {
	module := &Module{store: store, limits: limits, registrationCredentials: credentials, actions: actions, roles: roles, switches: switches}
	if passwordFailures != nil {
		module.passwordFailures = passwordFailures
	}
	return module
}

func (m *Module) Get(ctx context.Context, id int64) (*User, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return m.lookup(ctx, m.store.SQL(ctx), id)
}

func (m *Module) lookup(ctx context.Context, executor db.SQLExecutor, id int64) (*User, error) {
	const query = `SELECT id, created_at, updated_at, role_id, action, username, code, last_logged_in_at, status, metadata, wrong, login_count FROM t_user WHERE id = $1`
	value, err := m.scan(executor.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}
	actions, err := m.actions.Lookup(ctx, executor, value.ActionCodes)
	if err != nil {
		return nil, err
	}
	value.Actions = actionViews(actions)
	if value.RoleID != nil {
		found, lookupErr := m.roles.Lookup(ctx, executor, *value.RoleID)
		err = lookupErr
		if err != nil {
			return nil, err
		}
		value.Role = roleView(found)
	}
	return value, nil
}

func (m *Module) scan(row interface{ Scan(...any) error }) (*User, error) {
	var value User
	var createdAt, updatedAt time.Time
	var roleID sql.NullInt64
	var codes string
	var lastLoggedInAt sql.NullTime
	var metadata []byte
	err := row.Scan(
		&value.ID, &createdAt, &updatedAt, &roleID, &codes, &value.Username,
		&value.Code, &lastLoggedInAt, &value.Status, &metadata, &value.Wrong, &value.LoginCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	if err := json.Unmarshal(metadata, &value.Metadata); err != nil {
		return nil, fmt.Errorf("decode user metadata: %w", err)
	}
	if err := validateMetadataKeys(value.Metadata); err != nil {
		return nil, fmt.Errorf("decode user metadata keys: %w", err)
	}
	if lastLoggedInAt.Valid {
		milliseconds := lastLoggedInAt.Time.UnixMilli()
		value.LastLoggedInAt = &milliseconds
	}
	value.ActionCodes = m.actions.SplitCodes(codes)
	if roleID.Valid {
		value.RoleID = &roleID.Int64
	}
	return &value, nil
}

func (m *Module) Create(ctx context.Context, input CreateUserInput) (*User, error) {
	username := strings.TrimSpace(input.Username)
	if username != input.Username || !validUsername(username) || !validPassword(input.Password) {
		return nil, ErrInvalid
	}
	input.Username = username
	metadata, err := encodeMetadata(withoutLockMetadata(input.Metadata))
	if err != nil {
		return nil, ErrInvalid
	}
	var value *User
	err = m.store.Tx(ctx, func(ctx context.Context) error {
		if err := m.validateRole(ctx, m.store.SQL(ctx), input.RoleID); err != nil {
			return err
		}
		codes, err := m.actions.ValidateCodes(ctx, m.store.SQL(ctx), input.ActionCodes)
		if errors.Is(err, actionmodule.ErrNotFound) {
			return ErrActionNotFound
		}
		if err != nil {
			return err
		}
		password, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		var id int64
		if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT nextval('t_user_id_seq')`).Scan(&id); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		inserted := false
		for range 5 {
			userCode, err := code.Generate()
			if err != nil {
				return err
			}
			result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_user (id, created_at, updated_at, role_id, action, username, code, password, status, metadata, wrong) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0) ON CONFLICT ON CONSTRAINT uk_user_code DO NOTHING`,
				id, now, now, nullableRole(input.RoleID), strings.Join(codes, ","), input.Username, userCode, string(password), StatusActive, metadata)
			if err != nil {
				return mapWriteError(err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count == 1 {
				inserted = true
				break
			}
		}
		if !inserted {
			return errors.New("generate unique user code")
		}
		value, err = m.lookup(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrRoleNotFound) || errors.Is(err, ErrActionNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
			return nil, err
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateUserInput) (*User, error) {
	if id <= 0 || allNil(input) {
		return nil, ErrInvalid
	}
	if m.switches.ProtectSuper && id == authmodule.SuperUserID && input.Password != nil {
		return nil, apperror.FeatureDisabled
	}
	if m.switches.ProtectSuper && id == authmodule.SuperUserID {
		if input.Username != nil && strings.TrimSpace(*input.Username) != authmodule.SuperUsername {
			return nil, apperror.FeatureDisabled
		}
		if input.RoleID != nil && *input.RoleID != rolemodule.AdminRoleID {
			return nil, apperror.FeatureDisabled
		}
		if input.Status != nil && *input.Status != StatusActive {
			return nil, apperror.FeatureDisabled
		}
	}
	if input.Username != nil {
		value := strings.TrimSpace(*input.Username)
		if value != *input.Username || !validUsername(value) {
			return nil, ErrInvalid
		}
		input.Username = &value
	}
	if input.Password != nil && !validPassword(*input.Password) {
		return nil, ErrInvalid
	}
	if input.Status != nil && !validStatus(*input.Status) {
		return nil, ErrInvalid
	}
	resetPasswordFailures := input.Password != nil
	var metadata string
	if input.Metadata != nil {
		normalized, err := normalizeUpdateMetadata(*input.Metadata, input.Status)
		if err != nil {
			return nil, ErrInvalid
		}
		if input.Password != nil {
			delete(normalized, authmodule.PasswordChangeFailuresMetadataKey)
		}
		if _, retained := normalized[authmodule.PasswordChangeFailuresMetadataKey]; !retained {
			resetPasswordFailures = true
		}
		input.Metadata = &normalized
		metadata, err = encodeMetadata(normalized)
		if err != nil {
			return nil, ErrInvalid
		}
	} else if input.Status != nil && *input.Status == StatusLocked {
		return nil, ErrInvalid
	}
	var value *User
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		sets := make([]string, 0, 8)
		args := make([]any, 0, 9)
		add := func(column string, field any) {
			args = append(args, field)
			sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
		}
		if input.Username != nil {
			add("username", *input.Username)
		}
		if input.Password != nil {
			password, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			add("password", string(password))
			sets = append(sets, "credential_version = credential_version + 1", "login_count = 0")
		}
		if input.RoleID != nil {
			if err := m.validateRole(ctx, m.store.SQL(ctx), input.RoleID); err != nil {
				return err
			}
			add("role_id", nullableRole(input.RoleID))
		}
		if input.ActionCodes != nil {
			codes, err := m.actions.ValidateCodes(ctx, m.store.SQL(ctx), *input.ActionCodes)
			if errors.Is(err, actionmodule.ErrNotFound) {
				return ErrActionNotFound
			}
			if err != nil {
				return err
			}
			add("action", strings.Join(codes, ","))
		}
		if input.Status != nil {
			add("status", *input.Status)
		}
		if input.Metadata != nil {
			if input.Status == nil {
				args = append(args, metadata)
				placeholder := "$" + strconv.Itoa(len(args))
				sets = append(sets, "metadata = CASE WHEN metadata ? 'lock_expired_at' THEN "+placeholder+"::jsonb || jsonb_build_object('lock_expired_at', metadata -> 'lock_expired_at') ELSE "+placeholder+"::jsonb END")
			} else {
				add("metadata", metadata)
			}
		} else {
			metadataExpression := "metadata"
			if input.Password != nil {
				metadataExpression += " - '" + authmodule.PasswordChangeFailuresMetadataKey + "'"
			}
			if input.Status != nil && *input.Status != StatusLocked {
				metadataExpression += " - 'lock_expired_at'"
			}
			if metadataExpression != "metadata" {
				sets = append(sets, "metadata = "+metadataExpression)
			}
		}
		add("updated_at", time.Now().UTC().Truncate(time.Microsecond))
		args = append(args, id)
		query := "UPDATE t_user SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
		result, err := m.store.SQL(ctx).ExecContext(ctx, query, args...)
		if err != nil {
			return mapWriteError(err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		value, err = m.lookup(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrRoleNotFound) || errors.Is(err, ErrActionNotFound) || errors.Is(err, ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("update user: %w", err)
	}
	if resetPasswordFailures && m.passwordFailures != nil {
		if err := m.passwordFailures.Clear(ctx, id); err != nil {
			slog.WarnContext(ctx, "clear updated user password failures failed: "+err.Error())
		}
	}
	return value, nil
}

func (m *Module) Delete(ctx context.Context, ids ...int64) error {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return err
	}
	if m.switches.ProtectSuper {
		for _, id := range ids {
			if id == authmodule.SuperUserID {
				return apperror.FeatureDisabled
			}
		}
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_user WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Module) List(ctx context.Context, input ListUsersInput) (*ListUsersResult, error) {
	for _, status := range input.Statuses {
		if !validStatus(status) {
			return nil, ErrInvalid
		}
	}
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListUsersResult{Items: make([]User, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	now := time.Now().UnixMilli()
	where, args := filters(input, now)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_user"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, now)
	expired := expiredLockSQL("$" + strconv.Itoa(len(args)))
	columns := "id, created_at, updated_at, role_id, action, username, code, last_logged_in_at, CASE WHEN " + expired + " THEN 1 ELSE status END, CASE WHEN " + expired + " THEN metadata - 'lock_expired_at' ELSE metadata END, wrong, login_count"
	args = append(args, size, offset)
	query := "SELECT " + columns + " FROM t_user" + where + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := m.scan(rows)
		if err != nil {
			return nil, err
		}
		value.Items = append(value.Items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	roleIDs := make([]int64, 0, len(value.Items))
	codes := make([]string, 0)
	for _, item := range value.Items {
		if item.RoleID != nil {
			roleIDs = append(roleIDs, *item.RoleID)
		}
		codes = append(codes, item.ActionCodes...)
	}
	roles, err := m.roles.LookupRecords(ctx, m.store.SQL(ctx), roleIDs)
	if err != nil {
		return nil, err
	}
	// Follow page order so the batch input is deterministic even with shared roles.
	seenRoles := make(map[int64]struct{}, len(roles))
	for _, id := range roleIDs {
		if _, seen := seenRoles[id]; seen {
			continue
		}
		seenRoles[id] = struct{}{}
		codes = append(codes, roles[id].ActionCodes...)
	}
	actions, err := m.actions.Lookup(ctx, m.store.SQL(ctx), codes)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]ActionView, len(actions))
	for _, item := range actionViews(actions) {
		byCode[item.Code] = item
	}
	for i := range value.Items {
		item := &value.Items[i]
		item.Actions = orderedActions(item.ActionCodes, byCode)
		if item.RoleID != nil {
			record := roles[*item.RoleID]
			item.Role = roleView(&record)
			item.Role.Actions = orderedActions(record.ActionCodes, byCode)
		}
	}
	return value, nil
}

// Lists report effective status without writing. Authentication persists expiration
// for the relevant user through auth.unlockExpiredUser.
func expiredLockSQL(nowPlaceholder string) string {
	return "(status = 2 AND CASE WHEN metadata ->> 'lock_expired_at' ~ '^[0-9]+$' THEN (metadata ->> 'lock_expired_at')::numeric ELSE 0 END BETWEEN 1 AND " + nowPlaceholder + ")"
}

func (m *Module) SummariesByGroup(ctx context.Context, executor db.SQLExecutor, groupID int64) ([]Summary, error) {
	rows, err := executor.QueryContext(ctx, `SELECT u.id, u.username, u.code FROM t_user AS u JOIN t_user_user_group_relation AS relation ON relation.user_id = u.id WHERE relation.user_group_id = $1 ORDER BY u.id`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list user group members: %w", err)
	}
	defer rows.Close()
	result := make([]Summary, 0)
	for rows.Next() {
		var value Summary
		if err := rows.Scan(&value.ID, &value.Username, &value.Code); err != nil {
			return nil, fmt.Errorf("scan user summary: %w", err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user group members: %w", err)
	}
	return result, nil
}

// SummariesByGroups loads all page members, retaining ascending user ID order per group.
func (m *Module) SummariesByGroups(ctx context.Context, executor db.SQLExecutor, groupIDs []int64) (map[int64][]Summary, error) {
	result := make(map[int64][]Summary, len(groupIDs))
	if len(groupIDs) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, `SELECT relation.user_group_id, u.id, u.username, u.code FROM t_user AS u JOIN t_user_user_group_relation AS relation ON relation.user_id = u.id WHERE relation.user_group_id = ANY($1) ORDER BY relation.user_group_id, u.id`, pq.Array(groupIDs))
	if err != nil {
		return nil, fmt.Errorf("list user group members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var groupID int64
		var value Summary
		if err := rows.Scan(&groupID, &value.ID, &value.Username, &value.Code); err != nil {
			return nil, fmt.Errorf("scan user summary: %w", err)
		}
		result[groupID] = append(result[groupID], value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user group members: %w", err)
	}
	return result, nil
}

func (m *Module) ValidateIDs(ctx context.Context, executor db.SQLExecutor, ids []int64) ([]int64, error) {
	ids, err := normalizeReferenceIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	var count int64
	if err := executor.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_user WHERE id = ANY($1)`, pq.Array(ids)).Scan(&count); err != nil {
		return nil, fmt.Errorf("validate users: %w", err)
	}
	if count != int64(len(ids)) {
		return nil, ErrNotFound
	}
	return ids, nil
}

func (m *Module) validateRole(ctx context.Context, executor db.SQLExecutor, roleID *int64) error {
	if roleID == nil || *roleID == 0 {
		return nil
	}
	if *roleID < 0 {
		return ErrInvalid
	}
	exists, err := m.roles.Exists(ctx, executor, *roleID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrRoleNotFound
	}
	return nil
}

func nullableRole(roleID *int64) any {
	if roleID == nil || *roleID == 0 {
		return nil
	}
	return *roleID
}

func filters(input ListUsersInput, now int64) (string, []any) {
	parts := make([]string, 0, 3)
	args := make([]any, 0, 3)
	for _, field := range []struct {
		column, value string
		multiple      bool
	}{
		{"username", input.Username, false}, {"code", input.Code, true},
	} {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if field.multiple {
			addMultiLikeFilter(&parts, &args, field.column, field.value)
			continue
		}
		args = append(args, like(field.value))
		parts = append(parts, "LOWER("+field.column+") LIKE LOWER($"+strconv.Itoa(len(args))+") ESCAPE '!'")
	}
	statusColumn := "status"
	if len(input.Statuses) > 0 {
		args = append(args, now)
		statusColumn = "(CASE WHEN " + expiredLockSQL("$"+strconv.Itoa(len(args))) + " THEN 1 ELSE status END)"
	}
	if len(input.Statuses) == 1 {
		args = append(args, input.Statuses[0])
		parts = append(parts, statusColumn+" = $"+strconv.Itoa(len(args)))
	} else if len(input.Statuses) > 1 {
		args = append(args, pq.Array(input.Statuses))
		parts = append(parts, statusColumn+" = ANY($"+strconv.Itoa(len(args))+")")
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func like(value string) string {
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.TrimSpace(value)) + "%"
}

func addMultiLikeFilter(parts *[]string, args *[]any, column, value string) {
	values := splitFilterValues(value)
	if len(values) == 0 {
		return
	}
	if len(values) == 1 {
		*args = append(*args, like(values[0]))
		*parts = append(*parts, "LOWER("+column+") LIKE LOWER($"+strconv.Itoa(len(*args))+") ESCAPE '!'")
		return
	}
	patterns := make([]string, 0, len(values))
	for _, item := range values {
		patterns = append(patterns, like(item))
	}
	*args = append(*args, pq.Array(patterns))
	*parts = append(*parts, "EXISTS (SELECT 1 FROM unnest($"+strconv.Itoa(len(*args))+"::text[]) AS candidate(value) WHERE LOWER("+column+") LIKE LOWER(candidate.value) ESCAPE '!')")
}

func splitFilterValues(value string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func validUsername(value string) bool {
	return usernamePattern.MatchString(value)
}

func validPassword(value string) bool { count := len([]byte(value)); return count >= 6 && count <= 72 }

func validStatus(value int16) bool { return value >= StatusPending && value <= StatusLocked }

func allNil(input UpdateUserInput) bool {
	return input.Username == nil && input.Password == nil && input.RoleID == nil && input.ActionCodes == nil && input.Status == nil && input.Metadata == nil
}

func withoutLockMetadata(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		if key != "lock_expired_at" {
			result[key] = item
		}
	}
	return result
}

func normalizeUpdateMetadata(value map[string]any, status *int16) (map[string]any, error) {
	result := withoutLockMetadata(value)
	if status == nil || *status != StatusLocked {
		return result, nil
	}
	expiredAt, ok := integerMetadataValue(value["lock_expired_at"])
	if !ok || (expiredAt > 0 && expiredAt <= time.Now().UnixMilli()) {
		return nil, ErrInvalid
	}
	result["lock_expired_at"] = expiredAt
	return result, nil
}

func integerMetadataValue(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number >= 0
	case int32:
		return int64(number), number >= 0
	case int64:
		return number, number >= 0
	case float64:
		converted := int64(number)
		return converted, number >= 0 && number == float64(converted)
	case json.Number:
		converted, err := number.Int64()
		return converted, err == nil && converted >= 0
	default:
		return 0, false
	}
}

func encodeMetadata(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return "", err
	}
	if err := validateMetadataKeys(decoded); err != nil {
		return "", err
	}
	return string(encoded), nil
}

var (
	usernamePattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{4,49}$`)
	snakeMetadataKey = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
)

func validateMetadataKeys(value any) error {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if !snakeMetadataKey.MatchString(key) {
				return fmt.Errorf("metadata key %q must use snake_case", key)
			}
			if err := validateMetadataKeys(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range current {
			if err := validateMetadataKeys(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, ErrIDs
	}
	result, err := normalizeReferenceIDs(ids)
	if err != nil {
		return nil, ErrIDs
	}
	return result, nil
}

func normalizeReferenceIDs(ids []int64) ([]int64, error) {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func mapWriteError(err error) error {
	var target *pq.Error
	if errors.As(err, &target) && target.Code == "23505" {
		return ErrConflict
	}
	return err
}

func actionViews(values []actionmodule.Action) []ActionView {
	result := make([]ActionView, 0, len(values))
	for _, value := range values {
		result = append(result, ActionView{
			ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
			Code: value.Code, Name: value.Name, Group: value.Group, Word: value.Word,
			Resource: value.Resource, Menu: value.Menu, Button: value.Button,
		})
	}
	return result
}

func roleView(value *rolemodule.Role) *RoleView {
	return &RoleView{
		ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Name: value.Name, Word: value.Word, ActionCodes: value.ActionCodes,
		Actions: actionViewsFromRole(value.Actions),
	}
}

func actionViewsFromRole(values []rolemodule.ActionView) []ActionView {
	result := make([]ActionView, 0, len(values))
	for _, value := range values {
		result = append(result, ActionView{
			ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
			Code: value.Code, Name: value.Name, Group: value.Group, Word: value.Word,
			Resource: value.Resource, Menu: value.Menu, Button: value.Button,
		})
	}
	return result
}

func orderedActions(codes []string, byCode map[string]ActionView) []ActionView {
	result := make([]ActionView, 0, len(codes))
	for _, code := range codes {
		if item, ok := byCode[code]; ok {
			result = append(result, item)
		}
	}
	return result
}
