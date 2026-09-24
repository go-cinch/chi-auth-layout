package role

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
)

var (
	ErrNotFound       = apperror.New("ROLE_NOT_FOUND", "role not found")
	ErrActionNotFound = apperror.New("ROLE_ACTION_NOT_FOUND", "role action not found")
	ErrIDs            = apperror.New("ROLE_IDS", "provide 1-100 positive role ids")
	ErrInvalid        = apperror.New("ROLE_INVALID", "invalid role")
	ErrConflict       = apperror.New("ROLE_CONFLICT", "role word already exists")
)

const AdminRoleID int64 = 1

type Role struct {
	ID          int64        `json:"id"`
	CreatedAt   int64        `json:"created_at"`
	UpdatedAt   int64        `json:"updated_at"`
	Name        string       `json:"name"`
	Word        string       `json:"word"`
	ActionCodes []string     `json:"action_codes"`
	Actions     []ActionView `json:"actions"`
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

type CreateRoleInput struct {
	Name        string   `json:"name" example:"Operator"`
	Word        string   `json:"word" example:"operator"`
	ActionCodes []string `json:"action_codes,omitempty"`
}

type UpdateRoleInput struct {
	Name        *string   `json:"name,omitempty"`
	Word        *string   `json:"word,omitempty"`
	ActionCodes *[]string `json:"action_codes,omitempty"`
}

type ListRolesInput struct {
	Name       string
	Word       string
	ActionCode string
	Page       *int32
	PageSize   *int32
}

type ListRolesResult struct {
	Items    []Role `json:"items"`
	Total    int64  `json:"t"`
	Page     int32  `json:"p"`
	PageSize int32  `json:"s"`
}

type Module struct {
	store                   *db.Store
	limits                  pagination.Limits
	actions                 Actions
	protectSuper            bool
}

type Actions interface {
	SplitCodes(string) []string
	LookupByID(context.Context, db.SQLExecutor, int64) (*actionmodule.Action, error)
	Lookup(context.Context, db.SQLExecutor, []string) ([]actionmodule.Action, error)
	ValidateCodes(context.Context, db.SQLExecutor, []string) ([]string, error)
}

func New(store *db.Store, limits pagination.Limits, actions Actions, protectSuper bool) *Module {
	return &Module{store: store, limits: limits, actions: actions, protectSuper: protectSuper}
}

func (m *Module) Get(ctx context.Context, id int64) (*Role, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return m.Lookup(ctx, m.store.SQL(ctx), id)
}

func (m *Module) Lookup(ctx context.Context, executor db.SQLExecutor, id int64) (*Role, error) {
	const query = `SELECT id, created_at, updated_at, name, word, action FROM t_role WHERE id = $1`
	value, err := m.scan(executor.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}
	actions, err := m.actions.Lookup(ctx, executor, value.ActionCodes)
	if err != nil {
		return nil, err
	}
	value.Actions = make([]ActionView, 0, len(actions))
	for _, item := range actions {
		value.Actions = append(value.Actions, actionView(item))
	}
	return value, nil
}

func (m *Module) scan(row interface{ Scan(...any) error }) (*Role, error) {
	var value Role
	var createdAt, updatedAt time.Time
	var codes string
	err := row.Scan(&value.ID, &createdAt, &updatedAt, &value.Name, &value.Word, &codes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query role: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	value.ActionCodes = m.actions.SplitCodes(codes)
	return &value, nil
}

// LookupRecords loads roles without resolving actions so callers can share one action query.
func (m *Module) LookupRecords(ctx context.Context, executor db.SQLExecutor, ids []int64) (map[int64]Role, error) {
	result := make(map[int64]Role)
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, `SELECT id, created_at, updated_at, name, word, action FROM t_role WHERE id = ANY($1)`, pq.Array(unique))
	if err != nil {
		return nil, fmt.Errorf("lookup roles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := m.scan(rows)
		if err != nil {
			return nil, err
		}
		result[item.ID] = *item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lookup roles: %w", err)
	}
	// Preserve the single-record lookup's missing-role behavior.
	if len(result) != len(unique) {
		return nil, ErrNotFound
	}
	return result, nil
}

func (m *Module) Exists(ctx context.Context, executor db.SQLExecutor, id int64) (bool, error) {
	var exists bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_role WHERE id = $1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check role exists: %w", err)
	}
	return exists, nil
}

func (m *Module) Create(ctx context.Context, input CreateRoleInput) (*Role, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Word = strings.TrimSpace(input.Word)
	if !validName(input.Name) || !validName(input.Word) {
		return nil, ErrInvalid
	}
	var value *Role
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		codes, err := m.actions.ValidateCodes(ctx, m.store.SQL(ctx), input.ActionCodes)
		if errors.Is(err, actionmodule.ErrNotFound) {
			return ErrActionNotFound
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		var id int64
		err = m.store.SQL(ctx).QueryRowContext(ctx, `INSERT INTO t_role (created_at, updated_at, name, word, action) VALUES ($1, $2, $3, $4, $5) RETURNING id`, now, now, input.Name, input.Word, strings.Join(codes, ",")).Scan(&id)
		if err != nil {
			return mapWriteError(err)
		}
		value, err = m.Lookup(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrActionNotFound) || errors.Is(err, ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("create role: %w", err)
	}
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateRoleInput) (*Role, error) {
	if id <= 0 || (input.Name == nil && input.Word == nil && input.ActionCodes == nil) {
		return nil, ErrInvalid
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		if !validName(value) {
			return nil, ErrInvalid
		}
		input.Name = &value
	}
	if input.Word != nil {
		value := strings.TrimSpace(*input.Word)
		if !validName(value) {
			return nil, ErrInvalid
		}
		input.Word = &value
	}
	var value *Role
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		sets := make([]string, 0, 4)
		args := make([]any, 0, 5)
		add := func(column string, field any) {
			args = append(args, field)
			sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
		}
		if input.Name != nil {
			add("name", *input.Name)
		}
		if input.Word != nil {
			add("word", *input.Word)
		}
		if input.ActionCodes != nil {
			codes, err := m.actions.ValidateCodes(ctx, m.store.SQL(ctx), *input.ActionCodes)
			if errors.Is(err, actionmodule.ErrNotFound) {
				return ErrActionNotFound
			}
			if err != nil {
				return err
			}
			if m.protectSuper && id == AdminRoleID {
				allPermissions, err := m.actions.LookupByID(ctx, m.store.SQL(ctx), actionmodule.AllPermissionsActionID)
				if errors.Is(err, actionmodule.ErrNotFound) {
					return apperror.FeatureDisabled
				}
				if err != nil {
					return err
				}
				if !containsCode(codes, allPermissions.Code) {
					return apperror.FeatureDisabled
				}
			}
			add("action", strings.Join(codes, ","))
		}
		add("updated_at", time.Now().UTC().Truncate(time.Microsecond))
		args = append(args, id)
		query := "UPDATE t_role SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
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
		value, err = m.Lookup(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrActionNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, apperror.FeatureDisabled) {
			return nil, err
		}
		return nil, fmt.Errorf("update role: %w", err)
	}
	return value, nil
}

func (m *Module) Delete(ctx context.Context, ids ...int64) error {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return err
	}
	if m.protectSuper {
		for _, id := range ids {
			if id == AdminRoleID {
				return apperror.FeatureDisabled
			}
		}
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_role WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete role result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func containsCode(codes []string, target string) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}

func (m *Module) List(ctx context.Context, input ListRolesInput) (*ListRolesResult, error) {
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListRolesResult{Items: make([]Role, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	where, args := filters(input)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_role"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count roles: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, size, offset)
	query := "SELECT id, created_at, updated_at, name, word, action FROM t_role" + where + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
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
		return nil, fmt.Errorf("list roles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	codes := make([]string, 0)
	for _, item := range value.Items {
		codes = append(codes, item.ActionCodes...)
	}
	actions, err := m.actions.Lookup(ctx, m.store.SQL(ctx), codes)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]ActionView, len(actions))
	for _, item := range actions {
		byCode[item.Code] = actionView(item)
	}
	for i := range value.Items {
		item := &value.Items[i]
		item.Actions = orderedActions(item.ActionCodes, byCode)
	}
	return value, nil
}

func filters(input ListRolesInput) (string, []any) {
	fields := []struct {
		column, value string
		multiple      bool
	}{
		{"name", input.Name, false}, {"word", input.Word, true}, {"action", input.ActionCode, true},
	}
	parts := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields))
	for _, field := range fields {
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

func validName(value string) bool { return value != "" && utf8.RuneCountInString(value) <= 50 }

func normalizeIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, ErrIDs
	}
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrIDs
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

func actionView(value actionmodule.Action) ActionView {
	return ActionView{
		ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Code: value.Code, Name: value.Name, Group: value.Group, Word: value.Word,
		Resource: value.Resource, Menu: value.Menu, Button: value.Button,
	}
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
