package action

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"reflect"
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

type assignmentCatalog struct {
	value localization.Catalog
	err   error
}

func (c assignmentCatalog) ActionCatalog(context.Context) (localization.Catalog, error) {
	return c.value, c.err
}

type selectedAssignmentConfig struct{ binding ResourceBinding }

func (a selectedAssignmentConfig) Match(v driver.Value) bool {
	raw, ok := v.(string)
	if !ok {
		return false
	}
	var config Config
	if json.Unmarshal([]byte(raw), &config) != nil || config.Bindings == nil {
		return false
	}
	return reflect.DeepEqual(*config.Bindings, []ResourceBinding{a.binding}) && len(config.Endpoints) == 1 && config.Endpoints[0].Path == "/user"
}

func assignmentFixture() localization.Catalog {
	c := actionCatalogFixture()
	c.Messages["en-US"] = localization.Messages{Frontend: map[string]any{"groups": map[string]any{"admin:system:user": "Users"}}}
	return c
}

func TestResolveAssignmentReusesOnlyExactGrant(t *testing.T) {
	for _, existing := range []bool{true, false} {
		t.Run(map[bool]string{true: "reuse exact", false: "reject broader grant"}[existing], func(t *testing.T) {
			m, mock := newTestModule(t)
			m.definitions = assignmentCatalog{value: assignmentFixture()}
			binding := ResourceBinding{ResourceKey: "admin:system:user", Operation: "read", FeatureKeys: []string{"admin:system:user:query"}}
			candidate := binding
			if !existing {
				candidate.FeatureKeys = append(candidate.FeatureKeys, "admin:system:user:detail")
			}
			bindings := []ResourceBinding{candidate}
			raw, _ := json.Marshal(Config{Bindings: &bindings})
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT code, word, config").WillReturnRows(sqlmock.NewRows([]string{"code", "word", "config"}).AddRow("ABCDEFGH", "admin:read:system:user", raw))
			if !existing {
				mock.ExpectExec("INSERT INTO t_action").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "admin:system:user", sqlmock.AnyArg(), selectedAssignmentConfig{binding}).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			var codes []string
			err := m.store.Tx(t.Context(), func(ctx context.Context) error {
				var err error
				codes, err = ResolveAssignment(ctx, m, "user", "create", []ResourceBinding{binding})
				return err
			})
			if err != nil || len(codes) != 1 {
				t.Fatal(codes, err)
			}
			if existing && codes[0] != "ABCDEFGH" || !existing && codes[0] == "ABCDEFGH" {
				t.Fatal("grant selection broadened", codes)
			}
		})
	}
}

func TestAssignmentValidationAndFailures(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := t.Context()
	catalog := assignmentFixture()
	m.definitions = assignmentCatalog{value: catalog}
	if _, err := ResolveAssignment(ctx, struct{}{}, "user", "create", nil); err == nil {
		t.Fatal("missing resolver")
	}
	for _, pair := range [][2]string{ {"other", "create"}, {"user", "delete"}} {
		if _, err := m.ResolveAssignment(ctx, pair[0], pair[1], nil); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	valid := ResourceBinding{ResourceKey: "admin:system:user", Operation: "read", FeatureKeys: []string{"admin:system:user:query"}}
	for _, bindings := range [][]ResourceBinding{ {valid, valid}, { {ResourceKey: "missing", Operation: "read", FeatureKeys: []string{}}}, { {ResourceKey: valid.ResourceKey, Operation: "read", FeatureKeys: []string{"foreign"}}}} {
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		if _, err := m.ResolveAssignment(ctx, "role", "update", bindings); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.ResolveAssignment(ctx, "user", "create", nil); err == nil {
		t.Fatal("lock failure")
	}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	m.definitions = nil
	if _, err := m.ResolveAssignment(ctx, "user", "create", nil); err == nil {
		t.Fatal("missing catalog")
	}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	m.definitions = assignmentCatalog{err: sqlmock.ErrCancelled}
	if _, err := m.ResolveAssignment(ctx, "user", "create", nil); err == nil {
		t.Fatal("catalog failure")
	}
	m.definitions = assignmentCatalog{value: catalog}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	if values, err := m.ResolveAssignment(ctx, "user", "create", []ResourceBinding{}); err != nil || len(values) != 0 {
		t.Fatal(values, err)
	}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	invalid := valid
	invalid.FeatureKeys = nil
	if _, err := m.ResolveAssignment(ctx, "user", "create", []ResourceBinding{invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatal("implicit all accepted", err)
	}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT code, word, config").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.ResolveAssignment(ctx, "user", "create", []ResourceBinding{valid}); err == nil {
		t.Fatal("candidate failure")
	}
}
