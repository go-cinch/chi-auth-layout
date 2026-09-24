package dictionary

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var dictionaryTestTime = time.Date(2026, 9, 22, 13, 0, 0, 0, time.UTC)

type trackingValueCache struct {
	ValueCache
	invalidations int
}

func (c *trackingValueCache) Invalidate(ctx context.Context) error {
	c.invalidations++
	return c.ValueCache.Invalidate(ctx)
}

func newTestModule(t *testing.T) (*Module, *trackingValueCache, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = database.Close()
	})
	cache := &trackingValueCache{ValueCache: NewMemoryValueCache()}
	limits, err := pagination.New(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(&db.Store{DB: database}, cache, CacheTTL, limits, false)
	if err != nil {
		t.Fatal(err)
	}
	return m, cache, mock
}

func dictionaryRows(id int64, key, name, value, description string, enabled bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "dictionary_key", "name", "value", "description", "enabled"}).
		AddRow(id, dictionaryTestTime, dictionaryTestTime, key, name, []byte(value), description, enabled)
}

func expectDictionaryGet(mock sqlmock.Sqlmock, id int64, key, name, value string, enabled bool) {
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WithArgs(id).
		WillReturnRows(dictionaryRows(id, key, name, value, "description", enabled))
}

func TestDictionaryValueUsesCache(t *testing.T) {
	m, _, mock := newTestModule(t)
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs("TEST_VALUES").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(`["A","B"]`)))
	first, err := m.DictionaryValue(t.Context(), " TEST_VALUES ")
	if err != nil || string(first) != `["A","B"]` {
		t.Fatalf("first value = %s, %v", first, err)
	}
	second, err := m.DictionaryValue(t.Context(), "TEST_VALUES")
	if err != nil || string(second) != string(first) {
		t.Fatalf("cached value = %s, %v", second, err)
	}
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs("DISABLED").WillReturnError(sql.ErrNoRows)
	if _, err := m.DictionaryValue(t.Context(), "DISABLED"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled value error = %v", err)
	}
}

func TestDictionaryCRUD(t *testing.T) {
	m, cache, mock := newTestModule(t)
	ctx := t.Context()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectDictionaryGet(mock, 1, "TEST_VALUES", "Test Values", `[1]`, true)
	value, err := m.Get(ctx, 1)
	if err != nil || value.Key != "TEST_VALUES" || string(value.Value) != `[1]` {
		t.Fatalf("get = %#v, %v", value, err)
	}
	if _, err := m.Create(ctx, CreateDictionaryInput{Key: "lower", Name: "Name", Value: json.RawMessage(`[1]`)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid create = %v", err)
	}
	enabled := false
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_dictionary").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "TEST_VALUES", "Test Values", `[1,2]`, "description", false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectDictionaryGet(mock, 3, "TEST_VALUES", "Test Values", `[1,2]`, false)
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateDictionaryInput{Key: " TEST_VALUES ", Name: " Test Values ", Value: json.RawMessage(`[1,2]`), Description: " description ", Enabled: &enabled})
	if err != nil || value.ID != 3 || value.Enabled || cache.invalidations != 1 {
		t.Fatalf("create = %#v, invalidations %d, %v", value, cache.invalidations, err)
	}
	if _, err := m.Update(ctx, 1, UpdateDictionaryInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty update = %v", err)
	}
	m.protectCaptchaDictionaries = true
	protectedKey := "OTHER_KEY"
	protectedValue := json.RawMessage(`["changed"]`)
	protectedEnabled := false
	for _, id := range []int64{PointCaptchaEnglishDictionaryID, PointCaptchaChineseDictionaryID} {
		for field, input := range map[string]UpdateDictionaryInput{
			"key":     {Key: &protectedKey},
			"value":   {Value: &protectedValue},
			"enabled": {Enabled: &protectedEnabled},
		} {
			if _, err := m.Update(ctx, id, input); !errors.Is(err, apperror.FeatureDisabled) {
				t.Fatalf("update protected dictionary %d %s: %v", id, field, err)
			}
		}
	}
	description := "updated"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_dictionary SET description").WithArgs(description, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectDictionaryGet(mock, 1, "TEST_VALUES", "Test Values", `[1,2]`, true)
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateDictionaryInput{Description: &description})
	if err != nil || value.Description != "description" || cache.invalidations != 2 {
		t.Fatalf("update = %#v, invalidations %d, %v", value, cache.invalidations, err)
	}
	if err := m.Delete(ctx, -1); !errors.Is(err, ErrIDs) {
		t.Fatalf("invalid delete = %v", err)
	}
	if err := m.Delete(ctx, PointCaptchaEnglishDictionaryID, 3); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("delete protected English dictionary: %v", err)
	}
	if err := m.Delete(ctx, PointCaptchaChineseDictionaryID); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("delete protected Chinese dictionary: %v", err)
	}
	m.protectCaptchaDictionaries = false
	mock.ExpectExec("DELETE FROM t_dictionary").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil || cache.invalidations != 3 {
		t.Fatalf("delete invalidations %d: %v", cache.invalidations, err)
	}
	mock.ExpectExec("DELETE FROM t_dictionary").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing delete = %v", err)
	}
}

func TestDictionaryListAndFailures(t *testing.T) {
	m, _, mock := newTestModule(t)
	enabled := true
	mock.ExpectQuery("SELECT COUNT.*FROM t_dictionary WHERE").WithArgs("%POINT%", "%Captcha%", true).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE").WithArgs("%POINT%", "%Captcha%", true, int32(1), int64(0)).
		WillReturnRows(dictionaryRows(1, "POINT_VALUES", "Captcha", `[]`, "description", true))
	result, err := m.List(t.Context(), ListDictionariesInput{Key: "POINT", Name: "Captcha", Enabled: &enabled})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list = %#v, %v", result, err)
	}
	invalid := int32(0)
	result, err = m.List(t.Context(), ListDictionariesInput{Page: &invalid})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("out-of-range list = %#v, %v", result, err)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_dictionary").WillReturnError(errors.New("database"))
	if _, err := m.List(t.Context(), ListDictionariesInput{}); err == nil {
		t.Fatal("list database failure ignored")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_dictionary").WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()
	if _, err := m.Create(t.Context(), CreateDictionaryInput{Key: "TEST", Name: "Test", Value: json.RawMessage(`{}`)}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict = %v", err)
	}
	name := "Missing"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_dictionary").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := m.Update(t.Context(), 99, UpdateDictionaryInput{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing update = %v", err)
	}
}

func TestMemoryValueCacheAndValidation(t *testing.T) {
	cache := NewMemoryValueCache()
	_, hit, version, err := cache.Load(t.Context(), "KEY")
	if err != nil || hit || version != "0" {
		t.Fatalf("empty cache = %v, %q, %v", hit, version, err)
	}
	if err := cache.Store(t.Context(), "KEY", `[]`, version, time.Minute); err != nil {
		t.Fatal(err)
	}
	value, hit, _, err := cache.Load(t.Context(), "KEY")
	if err != nil || !hit || value != `[]` {
		t.Fatalf("cache value = %q, %v, %v", value, hit, err)
	}
	if err := cache.Invalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, hit, version, _ = cache.Load(t.Context(), "KEY"); hit || version != "1" {
		t.Fatalf("invalidated cache = %v, %q", hit, version)
	}
	if err := cache.Store(t.Context(), "KEY", `[]`, "bad", time.Minute); err == nil {
		t.Fatal("invalid cache version accepted")
	}
	if _, err := New(nil, cache, CacheTTL, pagination.Limits{}, false); err == nil {
		t.Fatal("nil store accepted")
	}
	database, _, _ := sqlmock.New()
	t.Cleanup(func() { _ = database.Close() })
	if _, err := New(&db.Store{DB: database}, nil, CacheTTL, pagination.Limits{}, false); err == nil {
		t.Fatal("nil cache accepted")
	}
	if _, err := New(&db.Store{DB: database}, cache, 0, pagination.Limits{}, false); err == nil {
		t.Fatal("invalid ttl accepted")
	}
}

type faultValueCache struct {
	ValueCache
	load       func(context.Context, string) (string, bool, string, error)
	store      func(context.Context, string, string, string, time.Duration) error
	invalidate func(context.Context) error
}

func (c *faultValueCache) Load(ctx context.Context, key string) (string, bool, string, error) {
	if c.load != nil {
		return c.load(ctx, key)
	}
	return c.ValueCache.Load(ctx, key)
}

func (c *faultValueCache) Store(ctx context.Context, key, value, version string, ttl time.Duration) error {
	if c.store != nil {
		return c.store(ctx, key, value, version, ttl)
	}
	return c.ValueCache.Store(ctx, key, value, version, ttl)
}

func (c *faultValueCache) Invalidate(ctx context.Context) error {
	return c.invalidate(ctx)
}

func TestCommittedWritesSurviveCacheFailure(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			m, _, mock := newTestModule(t)
			attempts := 0
			m.cache = &faultValueCache{invalidate: func(ctx context.Context) error {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("cache invalidated before database write completed: %v", err)
				}
				attempts++
				return errors.New("redis unavailable")
			}}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			var err error
			switch operation {
			case "create":
				mock.ExpectBegin()
				mock.ExpectQuery("INSERT INTO t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
				expectDictionaryGet(mock, 3, "TEST", "Test", `[1]`, true)
				mock.ExpectCommit()
				var value *Dictionary
				value, err = m.Create(t.Context(), CreateDictionaryInput{Key: "TEST", Name: "Test", Value: json.RawMessage(`[1]`)})
				if value == nil || value.ID != 3 {
					t.Fatalf("committed result lost: %#v", value)
				}
			case "update":
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE t_dictionary").WillReturnResult(sqlmock.NewResult(0, 1))
				expectDictionaryGet(mock, 3, "TEST", "Updated", `[1]`, true)
				mock.ExpectCommit()
				name := "Updated"
				var value *Dictionary
				value, err = m.Update(t.Context(), 3, UpdateDictionaryInput{Name: &name})
				if value == nil || value.Name != name {
					t.Fatalf("committed result lost: %#v", value)
				}
			case "delete":
				mock.ExpectExec("DELETE FROM t_dictionary").WillReturnResult(sqlmock.NewResult(0, 1))
				err = m.Delete(t.Context(), 3)
			}
			if err != nil || attempts != 3 {
				t.Fatalf("write error = %v, attempts = %d", err, attempts)
			}
			for _, want := range []string{`"event":"dictionary_cache_invalidation_failed"`, `"db_committed":true`, `"attempts":3`, `"operation":"` + operation + `"`} {
				if !strings.Contains(logs.String(), want) {
					t.Fatalf("missing %s in %s", want, logs.String())
				}
			}
		})
	}
}

func TestInvalidationRetriesWithIndependentDeadline(t *testing.T) {
	for _, succeedsAt := range []int{1, 2, 3, 0} {
		t.Run(fmt.Sprint(succeedsAt), func(t *testing.T) {
			m, _, _ := newTestModule(t)
			type traceKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), traceKey{}, "trace"))
			cancel()
			attempts := 0
			m.cache = &faultValueCache{invalidate: func(ctx context.Context) error {
				attempts++
				if ctx.Err() != nil || ctx.Value(traceKey{}) != "trace" {
					t.Fatal("request cancellation or trace propagation is wrong")
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 200*time.Millisecond {
					t.Fatal("missing attempt deadline")
				}
				if attempts == succeedsAt {
					return nil
				}
				if succeedsAt == 0 {
					<-ctx.Done()
					return ctx.Err()
				}
				return errors.New("temporary failure")
			}}
			started := time.Now()
			m.invalidate(ctx, "update", 1)
			want := succeedsAt
			if want == 0 {
				want = 3
			}
			if attempts != want {
				t.Fatalf("attempts = %d, want %d", attempts, want)
			}
			if time.Since(started) > 1500*time.Millisecond {
				t.Fatal("invalidation exceeded short retry budget")
			}
		})
	}
}

func TestDatabaseFailureDoesNotInvalidate(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			m, cache, mock := newTestModule(t)
			var err error
			switch operation {
			case "create":
				mock.ExpectBegin()
				mock.ExpectQuery("INSERT INTO t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				expectDictionaryGet(mock, 1, "TEST", "Test", `[]`, true)
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
				_, err = m.Create(t.Context(), CreateDictionaryInput{Key: "TEST", Name: "Test", Value: json.RawMessage(`[]`)})
			case "update":
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE t_dictionary").WillReturnError(errors.New("write failed"))
				mock.ExpectRollback()
				name := "Updated"
				_, err = m.Update(t.Context(), 1, UpdateDictionaryInput{Name: &name})
			case "delete":
				mock.ExpectExec("DELETE FROM t_dictionary").WillReturnError(errors.New("write failed"))
				err = m.Delete(t.Context(), 1)
			}
			if err == nil || cache.invalidations != 0 {
				t.Fatalf("error = %v, invalidations = %d", err, cache.invalidations)
			}
		})
	}
}

func TestDictionaryReadCacheFailures(t *testing.T) {
	for _, read := range []string{"value", "record"} {
		for _, failure := range []string{"load", "store", "corrupt", "database"} {
			t.Run(read+"/"+failure, func(t *testing.T) {
				m, _, mock := newTestModule(t)
				stores := 0
				m.cache = &faultValueCache{
					load: func(context.Context, string) (string, bool, string, error) {
						if failure == "load" {
							return "", false, "", errors.New("redis down")
						}
						if failure == "corrupt" {
							return "{broken", true, "7", nil
						}
						return "", false, "7", nil
					},
					store: func(_ context.Context, _, _ string, version string, ttl time.Duration) error {
						stores++
						if version != "7" || ttl != CacheTTL {
							t.Fatalf("refill version = %s, ttl = %s", version, ttl)
						}
						if failure == "store" {
							return errors.New("redis down")
						}
						return nil
					},
				}
				var err error
				if read == "value" {
					query := mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs("TEST")
					if failure == "database" {
						query.WillReturnError(errors.New("db down"))
					} else {
						query.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(`[1]`)))
					}
					var value json.RawMessage
					value, err = m.DictionaryValue(t.Context(), "TEST")
					if failure != "database" && string(value) != `[1]` {
						t.Fatalf("value = %s", value)
					}
				} else {
					if failure == "database" {
						mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WillReturnError(errors.New("db down"))
					} else {
						expectDictionaryGet(mock, 1, "TEST", "Test", `[1]`, true)
					}
					var hit bool
					_, hit, err = m.getCached(t.Context(), 1)
					if hit {
						t.Fatal("database response marked as cache hit")
					}
				}
				if (err != nil) != (failure == "database") {
					t.Fatalf("read error = %v", err)
				}
				wantStores := 1
				if failure == "load" || failure == "database" {
					wantStores = 0
				}
				if stores != wantStores {
					t.Fatalf("cache stores = %d", stores)
				}
			})
		}
	}
}

func TestDictionaryCacheTTLAndGeneration(t *testing.T) {
	m, cache, mock := newTestModule(t)
	expectDictionaryGet(mock, 1, "TEST", "Old", `[1]`, true)
	if _, hit, err := m.getCached(t.Context(), 1); err != nil || hit {
		t.Fatalf("first read: %v %v", hit, err)
	}
	if _, hit, err := m.getCached(t.Context(), 1); err != nil || !hit {
		t.Fatalf("cached read: %v %v", hit, err)
	}
	memory := cache.ValueCache.(*memoryValueCache)
	memory.mu.Lock()
	stale := memory.values["record:1"]
	expired := stale
	expired.expiresAt = time.Now().Add(-time.Second)
	memory.values["record:1"] = expired
	memory.mu.Unlock()
	expectDictionaryGet(mock, 1, "TEST", "Fresh", `[2]`, true)
	value, hit, err := m.getCached(t.Context(), 1)
	if err != nil || hit || value.Name != "Fresh" {
		t.Fatalf("expired read: %#v %v %v", value, hit, err)
	}
	m.invalidate(t.Context(), "update", 1)
	if err := memory.Store(t.Context(), "record:1", stale.value, "0", CacheTTL); err != nil {
		t.Fatal(err)
	}
	expectDictionaryGet(mock, 1, "TEST", "New generation", `[3]`, true)
	value, hit, err = m.getCached(t.Context(), 1)
	if err != nil || hit || value.Name != "New generation" {
		t.Fatalf("stale refill: %#v %v %v", value, hit, err)
	}
}
