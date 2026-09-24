package tests

{{ if .Computed.enable_redis_final -}}
import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "strconv"
    "testing"
    "time"

    "github.com/redis/go-redis/v9"
{{- if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{- end }}
    "{{ .Computed.module_name_final }}/internal/common/pagination"
    "{{ .Computed.module_name_final }}/internal/infra/rds"
    "{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

type failingDictionaryInvalidation struct {
    dictionary.ValueCache
    fail bool
    attempts int
}

func (c *failingDictionaryInvalidation) Invalidate(ctx context.Context) error {
    c.attempts++
    if c.fail { return errors.New("injected cache invalidation outage") }
    return c.ValueCache.Invalidate(ctx)
}

// Run with CHI_AUTH_TEST_DSN and CHI_AUTH_TEST_REDIS_DSN. PostgreSQL uses an
// isolated schema; Redis uses a unique namespace and removes only its test keys.
func TestDictionaryCommittedWritesAndCacheTTL(t *testing.T) {
    dsn := os.Getenv("CHI_AUTH_TEST_REDIS_DSN")
    if dsn == "" { t.Skip("set CHI_AUTH_TEST_REDIS_DSN to run dictionary cache integration") }
    store := permissionDatabase(t)
    ctx := t.Context()
    prefix := fmt.Sprintf("test:dictionary-%d:", time.Now().UnixNano())
    backend, cleanup, err := rds.NewRedisNamespaceCache(dsn, prefix, "dictionary:cache")
    if err != nil { t.Fatal(err) }
    t.Cleanup(cleanup)
    prefix += "dictionary:cache:"
    options, err := redis.ParseURL(dsn)
    if err != nil { t.Fatal(err) }
    client := redis.NewClient(options)
    t.Cleanup(func() { _ = client.Close() })
    cache := &failingDictionaryInvalidation{ValueCache: backend}
    limits, err := pagination.New(100, 100)
    if err != nil { t.Fatal(err) }
    m, err := dictionary.New(store, cache, dictionary.CacheTTL, limits, false)
    if err != nil { t.Fatal(err) }
{{ if eq .Computed.http_router_final "gin" -}}
    router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else -}}
    handler := m.HTTP()
{{ end -}}
    value, err := m.Create(ctx, dictionary.CreateDictionaryInput{Key: "CACHE_TEST", Name: "Cache test", Value: json.RawMessage(`[1]`)})
    if err != nil { t.Fatal(err) }
    id := strconv.FormatInt(value.ID, 10)
    keys := []string{prefix+"version", prefix+"value:1:record:"+id, prefix+"value:1:CACHE_TEST"}
    t.Cleanup(func() { _ = client.Del(context.Background(), keys...).Err() })
    read := func(wantHeader string, wantValue string, wantStatus int) {
        t.Helper()
        recorder := httptest.NewRecorder()
        handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/"+id, nil))
        if recorder.Code != wantStatus || recorder.Header().Get("X-Cache-Hit") != wantHeader { t.Fatalf("read: %d %v %s", recorder.Code, recorder.Header(), recorder.Body.String()) }
        if wantStatus == http.StatusOK {
            var result dictionary.Dictionary
            if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil { t.Fatal(err) }
            if string(result.Value) != wantValue { t.Fatalf("read value: %s, want %s", result.Value, wantValue) }
        }
    }
    read("", `[1]`, http.StatusOK)
    read("1", `[1]`, http.StatusOK)
    if _, err := m.DictionaryValue(ctx, "CACHE_TEST"); err != nil { t.Fatal(err) }
    if ttl := client.TTL(ctx, prefix+"version").Val(); ttl != -1 { t.Fatalf("version TTL: %s", ttl) }
    if ttl := client.TTL(ctx, keys[1]).Val(); ttl <= 0 || ttl > dictionary.CacheTTL { t.Fatalf("record TTL: %s", ttl) }
    cache.fail = true
    cache.attempts = 0
    updated := json.RawMessage(`[2]`)
    if result, err := m.Update(ctx, value.ID, dictionary.UpdateDictionaryInput{Value: &updated}); err != nil || string(result.Value) != `[2]` { t.Fatalf("committed update: %#v %v", result, err) }
    if cache.attempts != 3 { t.Fatalf("invalidation attempts: %d", cache.attempts) }
    var saved string
    if err := store.DB.QueryRowContext(ctx, `SELECT value FROM t_dictionary WHERE id=$1`, value.ID).Scan(&saved); err != nil || saved != `[2]` { t.Fatalf("database: %s %v", saved, err) }
    read("1", `[1]`, http.StatusOK) // Accepted stale read until TTL expires.
    // Advance expiry for this isolated test key instead of waiting ten minutes.
    if err := client.PExpire(ctx, keys[1], time.Millisecond).Err(); err != nil { t.Fatal(err) }
    deadline := time.Now().Add(time.Second)
    for client.Exists(ctx, keys[1]).Val() != 0 {
        if time.Now().After(deadline) { t.Fatal("cache key did not expire") }
        time.Sleep(time.Millisecond)
    }
    read("", `[2]`, http.StatusOK)
    read("1", `[2]`, http.StatusOK)
    cache.fail = false
    if err := m.Delete(ctx, value.ID); err != nil { t.Fatal(err) }
    if client.Exists(ctx, keys[1]).Val() != 1 { t.Fatal("invalidation deleted the old value") }
    read("", "", http.StatusNotFound)
    if _, err := m.DictionaryValue(ctx, "CACHE_TEST"); !errors.Is(err, dictionary.ErrNotFound) { t.Fatalf("deleted value visible: %v", err) }
}
{{ end -}}
