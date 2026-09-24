package dictionary

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
	"sync"
	"time"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

const (
	CacheTTL                          = 10 * time.Minute
	PointCaptchaEnglishDictionaryID  = int64(1)
	PointCaptchaChineseDictionaryID  = int64(2)
)

var (
	ErrNotFound = apperror.New("DICTIONARY_NOT_FOUND", "dictionary not found")
	ErrIDs      = apperror.New("DICTIONARY_IDS", "provide 1-100 positive dictionary ids")
	ErrInvalid  = apperror.New("DICTIONARY_INVALID", "invalid dictionary")
	ErrConflict = apperror.New("DICTIONARY_CONFLICT", "dictionary key already exists")
)

var dictionaryKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$`)

type ValueCache interface {
	Load(context.Context, string) (value string, hit bool, version string, err error)
	Store(context.Context, string, string, string, time.Duration) error
	Invalidate(context.Context) error
}

type memoryCacheValue struct {
	value     string
	version   uint64
	expiresAt time.Time
}

type memoryValueCache struct {
	mu      sync.Mutex
	version uint64
	values  map[string]memoryCacheValue
}

func NewMemoryValueCache() ValueCache {
	return &memoryValueCache{values: make(map[string]memoryCacheValue)}
}

func (c *memoryValueCache) Load(_ context.Context, key string) (string, bool, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	version := strconv.FormatUint(c.version, 10)
	value, ok := c.values[key]
	if !ok || value.version != c.version || !value.expiresAt.After(time.Now()) {
		delete(c.values, key)
		return "", false, version, nil
	}
	return value.value, true, version, nil
}

func (c *memoryValueCache) Store(_ context.Context, key, value, version string, ttl time.Duration) error {
	parsed, err := strconv.ParseUint(version, 10, 64)
	if err != nil || ttl <= 0 {
		return errors.New("memory dictionary cache input is invalid")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = memoryCacheValue{value: value, version: parsed, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (c *memoryValueCache) Invalidate(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version++
	clear(c.values)
	return nil
}

type Module struct {
	store                      *db.Store
	cache                      ValueCache
	ttl                        time.Duration
	limits                     pagination.Limits
	protectCaptchaDictionaries bool
}

func New(store *db.Store, cache ValueCache, ttl time.Duration, limits pagination.Limits, protectCaptchaDictionaries bool) (*Module, error) {
	if store == nil || store.DB == nil {
		return nil, errors.New("dictionary database is required")
	}
	if cache == nil {
		return nil, errors.New("dictionary cache is required")
	}
	if ttl <= 0 {
		return nil, errors.New("dictionary cache ttl must be positive")
	}
	return &Module{store: store, cache: cache, ttl: ttl, limits: limits, protectCaptchaDictionaries: protectCaptchaDictionaries}, nil
}

// DictionaryValue returns an enabled dictionary's arbitrary JSON value.
func (m *Module) DictionaryValue(ctx context.Context, key string) (json.RawMessage, error) {
	key = strings.TrimSpace(key)
	value, hit, version, cacheErr := m.cache.Load(ctx, key)
	if cacheErr != nil {
		slog.WarnContext(ctx, "read dictionary cache failed: "+cacheErr.Error())
	} else if hit && json.Valid([]byte(value)) {
		return json.RawMessage(value), nil
	}
	var raw []byte
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT value FROM t_dictionary WHERE dictionary_key = $1 AND enabled`, key).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("query dictionary value: %w", err)
	}
	if !json.Valid(raw) {
		return nil, errors.New("stored dictionary value is invalid JSON")
	}
	if cacheErr == nil {
		if err := m.cache.Store(ctx, key, string(raw), version, m.ttl); err != nil {
			slog.WarnContext(ctx, "write dictionary cache failed: "+err.Error())
		}
	}
	return json.RawMessage(append([]byte(nil), raw...)), nil
}

// invalidate runs only after a successful database write. Cache synchronization
// is best effort: callers must not report a committed write as a failure.
func (m *Module) invalidate(ctx context.Context, operation string, ids ...int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	started := time.Now()
	var lastErr error
	attempts := 0
	for attempts < 3 && ctx.Err() == nil {
		if attempts > 0 {
			timer := time.NewTimer(time.Duration(attempts) * 50 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			if ctx.Err() != nil {
				break
			}
		}
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 200*time.Millisecond)
		attempts++
		lastErr = m.cache.Invalidate(attemptCtx)
		attemptCancel()
		if lastErr == nil {
			return
		}
	}
	if ctx.Err() != nil {
		lastErr = ctx.Err()
	}
	slog.ErrorContext(ctx, "dictionary data saved but cache invalidation failed",
		"event", "dictionary_cache_invalidation_failed", "db_committed", true,
		"operation", operation, "ids", ids, "attempts", attempts,
		"elapsed_ms", time.Since(started).Milliseconds(), "error", lastErr)
}

type Dictionary struct {
	ID          int64           `json:"id"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
}

type CreateDictionaryInput struct {
	Key         string          `json:"key" example:"APPLICATION_FEATURE_FLAGS"`
	Name        string          `json:"name" example:"Application Feature Flags"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

type UpdateDictionaryInput struct {
	Key         *string          `json:"key,omitempty"`
	Name        *string          `json:"name,omitempty"`
	Value       *json.RawMessage `json:"value,omitempty"`
	Description *string          `json:"description,omitempty"`
	Enabled     *bool            `json:"enabled,omitempty"`
}

type ListDictionariesInput struct {
	Key      string
	Name     string
	Enabled  *bool
	Page     *int32
	PageSize *int32
}

type ListDictionariesResult struct {
	Items    []Dictionary `json:"items"`
	Total    int64        `json:"t"`
	Page     int32        `json:"p"`
	PageSize int32        `json:"s"`
}

func (m *Module) Get(ctx context.Context, id int64) (*Dictionary, error) {
	value, _, err := m.getCached(ctx, id)
	return value, err
}

// getCached reports a hit only when a complete, valid record came from cache.
// Record keys are separate from the uppercase keys used by DictionaryValue.
func (m *Module) getCached(ctx context.Context, id int64) (*Dictionary, bool, error) {
	if id <= 0 {
		return nil, false, ErrInvalid
	}
	key := "record:" + strconv.FormatInt(id, 10)
	raw, hit, version, cacheErr := m.cache.Load(ctx, key)
	if cacheErr != nil {
		slog.WarnContext(ctx, "read dictionary record cache failed: "+cacheErr.Error())
	} else if hit {
		var value Dictionary
		if err := json.Unmarshal([]byte(raw), &value); err == nil && value.ID == id && validKey(value.Key) && json.Valid(value.Value) {
			return &value, true, nil
		}
	}
	value, err := get(ctx, m.store.SQL(ctx), id)
	if err != nil {
		return nil, false, err
	}
	if cacheErr == nil {
		encoded, err := json.Marshal(value)
		if err == nil {
			err = m.cache.Store(ctx, key, string(encoded), version, m.ttl)
		}
		if err != nil {
			slog.WarnContext(ctx, "write dictionary record cache failed: "+err.Error())
		}
	}
	return value, false, nil
}

func get(ctx context.Context, executor db.SQLExecutor, id int64) (*Dictionary, error) {
	const query = `SELECT id, created_at, updated_at, dictionary_key, name, value, description, enabled FROM t_dictionary WHERE id = $1`
	var value Dictionary
	var createdAt, updatedAt time.Time
	err := executor.QueryRowContext(ctx, query, id).Scan(&value.ID, &createdAt, &updatedAt, &value.Key, &value.Name, &value.Value, &value.Description, &value.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query dictionary: %w", err)
	}
	value.CreatedAt = createdAt.UnixMilli()
	value.UpdatedAt = updatedAt.UnixMilli()
	return &value, nil
}

func (m *Module) Create(ctx context.Context, input CreateDictionaryInput) (*Dictionary, error) {
	input.Key = strings.TrimSpace(input.Key)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validKey(input.Key) || input.Name == "" || len(input.Name) > 100 || len(input.Description) > 2000 || !validValue(input.Value) {
		return nil, ErrInvalid
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	var value *Dictionary
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		now := time.Now().UTC().Truncate(time.Microsecond)
		var id int64
		err := m.store.SQL(ctx).QueryRowContext(ctx, `INSERT INTO t_dictionary (created_at, updated_at, dictionary_key, name, value, description, enabled) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7) RETURNING id`, now, now, input.Key, input.Name, string(input.Value), input.Description, enabled).Scan(&id)
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
		return nil, fmt.Errorf("create dictionary: %w", err)
	}
	m.invalidate(ctx, "create", value.ID)
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateDictionaryInput) (*Dictionary, error) {
	if id <= 0 || (input.Key == nil && input.Name == nil && input.Value == nil && input.Description == nil && input.Enabled == nil) {
		return nil, ErrInvalid
	}
	if m.protectCaptchaDictionaries && isPointCaptchaDictionary(id) && (input.Key != nil || input.Value != nil || input.Enabled != nil) {
		return nil, apperror.FeatureDisabled
	}
	if input.Key != nil {
		value := strings.TrimSpace(*input.Key)
		if !validKey(value) {
			return nil, ErrInvalid
		}
		input.Key = &value
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		if value == "" || len(value) > 100 {
			return nil, ErrInvalid
		}
		input.Name = &value
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len(value) > 2000 {
			return nil, ErrInvalid
		}
		input.Description = &value
	}
	if input.Value != nil && !validValue(*input.Value) {
		return nil, ErrInvalid
	}
	var value *Dictionary
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		sets := make([]string, 0, 6)
		args := make([]any, 0, 7)
		add := func(column string, field any) {
			args = append(args, field)
			sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
		}
		if input.Key != nil {
			add("dictionary_key", *input.Key)
		}
		if input.Name != nil {
			add("name", *input.Name)
		}
		if input.Value != nil {
			args = append(args, string(*input.Value))
			sets = append(sets, "value = $"+strconv.Itoa(len(args))+"::jsonb")
		}
		if input.Description != nil {
			add("description", *input.Description)
		}
		if input.Enabled != nil {
			add("enabled", *input.Enabled)
		}
		add("updated_at", time.Now().UTC().Truncate(time.Microsecond))
		args = append(args, id)
		query := "UPDATE t_dictionary SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
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
		return nil, fmt.Errorf("update dictionary: %w", err)
	}
	m.invalidate(ctx, "update", id)
	return value, nil
}

func (m *Module) Delete(ctx context.Context, ids ...int64) error {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return err
	}
	if m.protectCaptchaDictionaries {
		for _, id := range ids {
			if isPointCaptchaDictionary(id) {
				return apperror.FeatureDisabled
			}
		}
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_dictionary WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("delete dictionaries: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete dictionaries result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	m.invalidate(ctx, "delete", ids...)
	return nil
}

func isPointCaptchaDictionary(id int64) bool {
	return id == PointCaptchaEnglishDictionaryID || id == PointCaptchaChineseDictionaryID
}

func (m *Module) List(ctx context.Context, input ListDictionariesInput) (*ListDictionariesResult, error) {
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListDictionariesResult{Items: make([]Dictionary, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	where, args := filters(input)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_dictionary"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count dictionaries: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, size, offset)
	query := "SELECT id, created_at, updated_at, dictionary_key, name, value, description, enabled FROM t_dictionary" + where + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list dictionaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item Dictionary
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &createdAt, &updatedAt, &item.Key, &item.Name, &item.Value, &item.Description, &item.Enabled); err != nil {
			return nil, fmt.Errorf("scan dictionary: %w", err)
		}
		item.CreatedAt = createdAt.UnixMilli()
		item.UpdatedAt = updatedAt.UnixMilli()
		value.Items = append(value.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dictionaries: %w", err)
	}
	return value, nil
}

func filters(input ListDictionariesInput) (string, []any) {
	parts := make([]string, 0, 3)
	args := make([]any, 0, 3)
	addLike := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		args = append(args, "%"+strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value)+"%")
		parts = append(parts, "LOWER("+column+") LIKE LOWER($"+strconv.Itoa(len(args))+") ESCAPE '!'")
	}
	addLike("dictionary_key", input.Key)
	addLike("name", input.Name)
	if input.Enabled != nil {
		args = append(args, *input.Enabled)
		parts = append(parts, "enabled = $"+strconv.Itoa(len(args)))
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func validKey(value string) bool {
	return len(value) <= 100 && dictionaryKeyPattern.MatchString(value)
}

func validValue(value json.RawMessage) bool {
	return len(value) > 0 && len(value) <= 1<<20 && json.Valid(value)
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
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
