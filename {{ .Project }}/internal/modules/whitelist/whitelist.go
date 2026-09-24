package whitelist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrNotFound = apperror.New("WHITELIST_NOT_FOUND", "whitelist entry not found")
	ErrIDs      = apperror.New("WHITELIST_IDS", "provide 1-100 positive whitelist ids")
	ErrInvalid  = apperror.New("WHITELIST_INVALID", "invalid whitelist entry")
	ErrConflict = apperror.New("WHITELIST_CONFLICT", "whitelist entry already exists")
)

type Whitelist struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Category  int16  `json:"category"`
	Resource  string `json:"resource"`
}

type CreateWhitelistInput struct {
	Category int16  `json:"category" example:"0"`
	Resource string `json:"resource" example:"GET|/healthz"`
}

type UpdateWhitelistInput struct {
	Category *int16  `json:"category,omitempty"`
	Resource *string `json:"resource,omitempty"`
}

type ListWhitelistsInput struct {
	Category *int16
	Resource string
	Page     *int32
	PageSize *int32
}

type ListWhitelistsResult struct {
	Items    []Whitelist `json:"items"`
	Total    int64       `json:"t"`
	Page     int32       `json:"p"`
	PageSize int32       `json:"s"`
}

type Module struct {
	store  *db.Store
	limits pagination.Limits
}

func New(store *db.Store, limits pagination.Limits) *Module {
	return &Module{store: store, limits: limits}
}

func (m *Module) Get(ctx context.Context, id int64) (*Whitelist, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return get(ctx, m.store.SQL(ctx), id)
}

func get(ctx context.Context, executor db.SQLExecutor, id int64) (*Whitelist, error) {
	const query = `SELECT id, created_at, updated_at, category, resource FROM t_whitelist WHERE id = $1`
	var value Whitelist
	var createdAt, updatedAt time.Time
	err := executor.QueryRowContext(ctx, query, id).Scan(&value.ID, &createdAt, &updatedAt, &value.Category, &value.Resource)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query whitelist entry: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	return &value, nil
}

func (m *Module) Create(ctx context.Context, input CreateWhitelistInput) (*Whitelist, error) {
	input.Resource = strings.TrimSpace(input.Resource)
	if !valid(input.Category, input.Resource) {
		return nil, ErrInvalid
	}
	var value *Whitelist
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		now := time.Now().UTC().Truncate(time.Microsecond)
		var id int64
		err := m.store.SQL(ctx).QueryRowContext(ctx, `INSERT INTO t_whitelist (created_at, updated_at, category, resource) VALUES ($1, $2, $3, $4) RETURNING id`, now, now, input.Category, input.Resource).Scan(&id)
		if err != nil {
			return mapWriteError(err)
		}
		value, err = get(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("create whitelist entry: %w", err)
	}
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateWhitelistInput) (*Whitelist, error) {
	if id <= 0 || (input.Category == nil && input.Resource == nil) {
		return nil, ErrInvalid
	}
	if input.Category != nil && (*input.Category < 0 || *input.Category > 1) {
		return nil, ErrInvalid
	}
	if input.Resource != nil {
		value := strings.TrimSpace(*input.Resource)
		if value == "" {
			return nil, ErrInvalid
		}
		input.Resource = &value
	}
	var value *Whitelist
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		sets := make([]string, 0, 3)
		args := make([]any, 0, 4)
		add := func(column string, field any) {
			args = append(args, field)
			sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
		}
		if input.Category != nil {
			add("category", *input.Category)
		}
		if input.Resource != nil {
			add("resource", *input.Resource)
		}
		add("updated_at", time.Now().UTC().Truncate(time.Microsecond))
		args = append(args, id)
		query := "UPDATE t_whitelist SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
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
		return nil, fmt.Errorf("update whitelist entry: %w", err)
	}
	return value, nil
}

func (m *Module) Delete(ctx context.Context, ids ...int64) error {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return err
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_whitelist WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("delete whitelist entries: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete whitelist result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Module) List(ctx context.Context, input ListWhitelistsInput) (*ListWhitelistsResult, error) {
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListWhitelistsResult{Items: make([]Whitelist, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	where, args := filters(input)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_whitelist"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count whitelist entries: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, size, offset)
	query := "SELECT id, created_at, updated_at, category, resource FROM t_whitelist" + where + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list whitelist entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item Whitelist
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &createdAt, &updatedAt, &item.Category, &item.Resource); err != nil {
			return nil, fmt.Errorf("scan whitelist entry: %w", err)
		}
		item.CreatedAt = createdAt.UnixMilli()
		item.UpdatedAt = updatedAt.UnixMilli()
		value.Items = append(value.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list whitelist entries: %w", err)
	}
	return value, nil
}

func filters(input ListWhitelistsInput) (string, []any) {
	parts := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if input.Category != nil {
		args = append(args, *input.Category)
		parts = append(parts, "category = $"+strconv.Itoa(len(args)))
	}
	if strings.TrimSpace(input.Resource) != "" {
		addMultiLikeFilter(&parts, &args, "resource", input.Resource)
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

func valid(category int16, resource string) bool {
	return category >= 0 && category <= 1 && resource != ""
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
