package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

func TestDictionaryConditionalUpdatePostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, err := pagination.New(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	module, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage("{\"a\":1,\"b\":2}")
	value, err := module.Create(t.Context(), dictionary.CreateDictionaryInput{Key: "CONCURRENT_TEST", Name: "Concurrent test", Value: original})
	if err != nil {
		t.Fatal(err)
	}
	// JSONB comparison must ignore key ordering and numeric formatting.
	reordered := json.RawMessage("{\"b\":2, \"a\":1.0}")
	next := json.RawMessage("{\"a\":2,\"b\":2}")
	if _, err := module.Update(t.Context(), value.ID, dictionary.UpdateDictionaryInput{Value: &next, ExpectedValue: &reordered}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := range 2 {
		go func() {
			<-start
			candidate := json.RawMessage(fmt.Sprintf("{\"winner\":%d}", i))
			_, err := module.Update(t.Context(), value.ID, dictionary.UpdateDictionaryInput{Value: &candidate, ExpectedValue: &next})
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, dictionary.ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("conditional update failed: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d; expected exactly one writer", successes, conflicts)
	}
	saved, err := module.Get(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	var winner map[string]int
	if err := json.Unmarshal(saved.Value, &winner); err != nil {
		t.Fatal(err)
	}
	if number, ok := winner["winner"]; !ok || number < 0 || number > 1 {
		t.Fatalf("unexpected winner: %s", saved.Value)
	}
}
