package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

func TestActionBuiltinOperationConstraintPostgres(t *testing.T) {
	store := permissionDatabase(t)
	insert := func(scope, operation, builtinJSON string) error {
		t.Helper()
		config := `{"bindings":[],"ui":[],"endpoints":[]}`
		if builtinJSON != "" {
			config = `{"ui":[],"endpoints":[],"builtin":` + builtinJSON + `}`
		}
		_, err := store.DB.ExecContext(t.Context(), `INSERT INTO t_action(action_group,code,word,config) VALUES($1,$2,$3,$4)`, scope+":system:access", fmt.Sprintf("BILT%04d", len(scope)), scope+":"+operation+":system:access", config)
		return err
	}
	for index, test := range []struct {
		name, operation, builtin string
		valid                    bool
	}{
		{"ordinary", "read", "", true},
		{"all", "all", `"all"`, true},
		{"default", "default", `"default"`, true},
		{"read all mismatch", "read", `"all"`, false},
		{"read default mismatch", "read", `"default"`, false},
		{"all default mismatch", "all", `"default"`, false},
		{"default all mismatch", "default", `"all"`, false},
		{"identity containing all is ordinary", "all", "", true},
		{"identity containing default is ordinary", "default", "", true},
		{"null builtin", "all", "null", false},
		{"numeric builtin", "all", "1", false},
		{"empty builtin", "read", `""`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope := "check" + strings.Repeat("x", index)
			err := insert(scope, test.operation, test.builtin)
			if test.valid {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var constraint *pq.Error
			if !errors.As(err, &constraint) || constraint.Code != "23514" || constraint.Constraint != "ck_action_builtin" {
				t.Fatalf("expected builtin constraint rejection, got %v", err)
			}
		})
	}
}

func TestUpdateBuiltinAllPreservesStoredMarkerPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	actions.SetDefinitionCatalog(dictionaries)
	actions.SetNameLocalization(dictionaries)
	word := "admin:all:system:all"
	value, err := actions.Update(t.Context(), 1, action.UpdateActionInput{Word: &word, Names: map[string]string{"en-US": "All permissions revised"}})
	if err != nil {
		t.Fatal(err)
	}
	if value.Names["en-US"] != "All permissions revised" || len(value.Config.Endpoints) == 0 {
		t.Fatal("read expansion or name update missing", value)
	}
	var raw []byte
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT config FROM t_action WHERE id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var config action.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.Builtin != "" || config.PermissionScope != "*" || config.Bindings == nil || config.UI == nil || len(config.UI) != 0 || config.Endpoints == nil || len(config.Endpoints) != 0 {
		t.Fatalf("stored all changed: %s", raw)
	}
	// A detail response may contain expanded arrays; submitting it must still persist a marker.
	if _, err := actions.Update(t.Context(), 1, action.UpdateActionInput{Config: &value.Config}); err != nil {
		t.Fatal(err)
	}
}
