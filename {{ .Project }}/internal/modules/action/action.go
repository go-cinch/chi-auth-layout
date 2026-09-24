package action

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
	"{{ .Computed.module_name_final }}/internal/common/code"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrNotFound = apperror.New("ACTION_NOT_FOUND", "action not found")
	ErrIDs      = apperror.New("ACTION_IDS", "provide 1-100 positive action ids")
	ErrInvalid  = apperror.New("ACTION_INVALID", "invalid action")
	ErrConflict = apperror.New("ACTION_CONFLICT", "action code or word already exists")
)

const AllPermissionsActionID int64 = 1

type Action struct {
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

type CreateActionInput struct {
	Name     string `json:"name" example:"User Query"`
	Group    string `json:"group" example:"User"`
	Word     string `json:"word" example:"user.read"`
	Resource string `json:"resource,omitempty" example:"GET|/user|/auth.v1.User/ListUsers"`
	Menu     string `json:"menu,omitempty" example:"/system/user"`
	Button   string `json:"button,omitempty" example:"system.user.read"`
}

type UpdateActionInput struct {
	Name     *string `json:"name,omitempty"`
	Group    *string `json:"group,omitempty"`
	Word     *string `json:"word,omitempty"`
	Resource *string `json:"resource,omitempty"`
	Menu     *string `json:"menu,omitempty"`
	Button   *string `json:"button,omitempty"`
}

type ListActionsInput struct {
	Code     string
	Name     string
	Group    string
	Word     string
	Resource string
	Menu     string
	Button   string
	Page     *int32
	PageSize *int32
}

type ListActionsResult struct {
	Items    []Action `json:"items"`
	Total    int64    `json:"t"`
	Page     int32    `json:"p"`
	PageSize int32    `json:"s"`
}

type ListActionGroupsResult struct {
	Items []string `json:"items"`
}

type Module struct {
	store                   *db.Store
	limits                  pagination.Limits
	protectSuper            bool
}

func New(store *db.Store, limits pagination.Limits, protectSuper bool) *Module {
	return &Module{store: store, limits: limits, protectSuper: protectSuper}
}

func (m *Module) Get(ctx context.Context, id int64) (*Action, error) {
	return m.LookupByID(ctx, m.store.SQL(ctx), id)
}

func (*Module) LookupByID(ctx context.Context, executor db.SQLExecutor, id int64) (*Action, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return get(ctx, executor, id)
}

func get(ctx context.Context, executor db.SQLExecutor, id int64) (*Action, error) {
	const query = `SELECT id, created_at, updated_at, code, name, action_group, word, resource, menu, btn FROM t_action WHERE id = $1`
	var value Action
	var createdAt, updatedAt time.Time
	err := executor.QueryRowContext(ctx, query, id).Scan(
		&value.ID, &createdAt, &updatedAt, &value.Code, &value.Name,
		&value.Group, &value.Word, &value.Resource, &value.Menu, &value.Button,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query action: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	return &value, nil
}

func (m *Module) Create(ctx context.Context, input CreateActionInput) (*Action, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Group = strings.TrimSpace(input.Group)
	input.Word = strings.TrimSpace(input.Word)
	var valid bool
	input.Resource, valid = normalizeResourceRules(input.Resource)
	if !validName(input.Name) || !validName(input.Group) || !validName(input.Word) || !valid {
		return nil, ErrInvalid
	}
	var value *Action
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		var id int64
		if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT nextval('t_action_id_seq')`).Scan(&id); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		inserted := false
		for range 5 {
			actionCode, err := code.Generate()
			if err != nil {
				return err
			}
			result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_action (id, created_at, updated_at, code, name, action_group, word, resource, menu, btn) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT ON CONSTRAINT uk_action_code DO NOTHING`,
				id, now, now, actionCode, input.Name, input.Group, input.Word, input.Resource, input.Menu, input.Button)
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
			return errors.New("generate unique action code")
		}
		created, err := get(ctx, m.store.SQL(ctx), id)
		value = created
		return err
	})
	if err != nil {
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
			return nil, err
		}
		return nil, fmt.Errorf("create action: %w", err)
	}
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateActionInput) (*Action, error) {
	if id <= 0 || allNil(input) {
		return nil, ErrInvalid
	}
	if m.protectSuper && id == AllPermissionsActionID && (input.Resource != nil || input.Menu != nil || input.Button != nil) {
		return nil, apperror.FeatureDisabled
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		if !validName(value) {
			return nil, ErrInvalid
		}
		input.Name = &value
	}
	if input.Group != nil {
		value := strings.TrimSpace(*input.Group)
		if !validName(value) {
			return nil, ErrInvalid
		}
		input.Group = &value
	}
	if input.Word != nil {
		value := strings.TrimSpace(*input.Word)
		if !validName(value) {
			return nil, ErrInvalid
		}
		input.Word = &value
	}
	if input.Resource != nil {
		value, valid := normalizeResourceRules(*input.Resource)
		if !valid {
			return nil, ErrInvalid
		}
		input.Resource = &value
	}
	var value *Action
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		sets := make([]string, 0, 7)
		args := make([]any, 0, 8)
		add := func(column string, field any) {
			args = append(args, field)
			sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
		}
		if input.Name != nil {
			add("name", *input.Name)
		}
		if input.Group != nil {
			add("action_group", *input.Group)
		}
		if input.Word != nil {
			add("word", *input.Word)
		}
		if input.Resource != nil {
			add("resource", *input.Resource)
		}
		if input.Menu != nil {
			add("menu", *input.Menu)
		}
		if input.Button != nil {
			add("btn", *input.Button)
		}
		add("updated_at", time.Now().UTC().Truncate(time.Microsecond))
		args = append(args, id)
		query := "UPDATE t_action SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
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
		value, err = get(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("update action: %w", err)
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
			if id == AllPermissionsActionID {
				return apperror.FeatureDisabled
			}
		}
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_action WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("delete action: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete action result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Module) List(ctx context.Context, input ListActionsInput) (*ListActionsResult, error) {
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListActionsResult{Items: make([]Action, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	where, args := actionFilters(input)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_action"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count actions: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, size, offset)
	query := "SELECT id, created_at, updated_at, code, name, action_group, word, resource, menu, btn FROM t_action" + where +
		" ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		value.Items = append(value.Items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	return value, nil
}

func (m *Module) ListGroups(ctx context.Context, keyword string) (*ListActionGroupsResult, error) {
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) > 50 {
		return nil, ErrInvalid
	}
	value := &ListActionGroupsResult{Items: make([]string, 0)}
	query := `SELECT MIN(action_group) FROM t_action WHERE BTRIM(action_group) <> ''`
	args := make([]any, 0, 1)
	if keyword != "" {
		args = append(args, like(keyword))
		query += ` AND LOWER(action_group) LIKE LOWER($1) ESCAPE '!'`
	}
	query += ` GROUP BY LOWER(action_group) ORDER BY LOWER(action_group) LIMIT 20`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, fmt.Errorf("scan action group: %w", err)
		}
		value.Items = append(value.Items, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	return value, nil
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (*Action, error) {
	var value Action
	var createdAt, updatedAt time.Time
	if err := row.Scan(&value.ID, &createdAt, &updatedAt, &value.Code, &value.Name, &value.Group, &value.Word, &value.Resource, &value.Menu, &value.Button); err != nil {
		return nil, fmt.Errorf("scan action: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	return &value, nil
}

func (m *Module) Lookup(ctx context.Context, executor db.SQLExecutor, codes []string) ([]Action, error) {
	codes = NormalizeCodes(codes)
	result := make([]Action, 0, len(codes))
	if len(codes) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, `SELECT id, created_at, updated_at, code, name, action_group, word, resource, menu, btn FROM t_action WHERE code = ANY($1)`, pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("lookup actions: %w", err)
	}
	defer rows.Close()
	byCode := make(map[string]Action, len(codes))
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		byCode[item.Code] = *item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lookup actions: %w", err)
	}
	for _, code := range codes {
		if item, ok := byCode[code]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (m *Module) ValidateCodes(ctx context.Context, executor db.SQLExecutor, codes []string) ([]string, error) {
	codes = NormalizeCodes(codes)
	items, err := m.Lookup(ctx, executor, codes)
	if err != nil {
		return nil, err
	}
	if len(items) != len(codes) {
		return nil, ErrNotFound
	}
	return codes, nil
}

func NormalizeCodes(codes []string) []string {
	result := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func (m *Module) SplitCodes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return NormalizeCodes(strings.Split(value, ","))
}

func actionFilters(input ListActionsInput) (string, []any) {
	parts := make([]string, 0, 7)
	args := make([]any, 0, 7)
	code := strings.TrimSpace(input.Code)
	if code != "" {
		if strings.Contains(code, ",") {
			args = append(args, pq.Array(NormalizeCodes(strings.Split(code, ","))))
			parts = append(parts, "code = ANY($"+strconv.Itoa(len(args))+")")
		} else {
			args = append(args, like(code))
			parts = append(parts, "LOWER(code) LIKE LOWER($"+strconv.Itoa(len(args))+") ESCAPE '!'")
		}
	}
	fields := []struct {
		column, value string
		multiple      bool
	}{
		{"name", input.Name, false}, {"action_group", input.Group, true}, {"word", input.Word, true},
		{"resource", input.Resource, true}, {"menu", input.Menu, true}, {"btn", input.Button, true},
	}
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

func normalizeResourceRules(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	rules := make([]string, 0)
	for line := range strings.SplitSeq(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "*" {
			rules = append(rules, line)
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 2 && len(parts) != 3 {
			return "", false
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		hasHTTP := parts[0] != "" || parts[1] != ""
		if hasHTTP && (parts[0] == "" || !strings.HasPrefix(parts[1], "/")) {
			return "", false
		}
		hasRPC := len(parts) == 3
		if hasRPC && !strings.HasPrefix(parts[2], "/") {
			return "", false
		}
		if !hasHTTP && !hasRPC {
			return "", false
		}
		rules = append(rules, strings.Join(parts, "|"))
	}
	if len(rules) == 0 {
		return "", true
	}
	return strings.Join(rules, "\n"), true
}

func allNil(input UpdateActionInput) bool {
	return input.Name == nil && input.Group == nil && input.Word == nil && input.Resource == nil && input.Menu == nil && input.Button == nil
}

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
