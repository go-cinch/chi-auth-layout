package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type catalogAdministration struct {
	allowed bool
	locks   int
}

func (p *catalogAdministration) RequireAllPermissions(context.Context) error {
	if p.allowed {
		return nil
	}
	return authn.ErrPermissionAdministrationRequired
}

func (p *catalogAdministration) LockPermissionMutation(context.Context) error { p.locks++; return nil }

func TestCatalogMutationCannotBypassPermissionAdministration(t *testing.T) {
	localizer := localization.New()
	m, _, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	policy := &catalogAdministration{}
	m.SetPermissionAdministration(policy)
	encoded, _ := json.Marshal(localizer.Current())
	raw := json.RawMessage(encoded)
	mock.ExpectBegin()
	mock.ExpectRollback()
	if _, err := m.Create(t.Context(), CreateDictionaryInput{Key: LocalizationKey, Name: "Catalog", Value: raw}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatal(err)
	}
	rename, disabled := "RENAMED_CATALOG", false
	for _, input := range []UpdateDictionaryInput{
		{Value: &raw}, {Key: &rename}, {Enabled: &disabled},
	} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array([]int64{3}), LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()
		if _, err := m.Update(t.Context(), 3, input); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
			t.Fatal(err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array([]int64{3, 5}), LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	if err := m.Delete(t.Context(), 3, 5); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatal(err)
	}
	if policy.locks != 4 {
		t.Fatal("catalog writes did not participate in mutation locking")
	}
}

func TestOrdinaryDictionaryEditsRemainIndependent(t *testing.T) {
	m, _, mock := newTestModule(t)
	policy := &catalogAdministration{}
	m.SetPermissionAdministration(policy)
	mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array([]int64{8}), LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	if err := m.guardCatalogMutation(t.Context(), []int64{8}, ""); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array([]int64{8}), LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	if err := m.guardCatalogMutation(t.Context(), []int64{8}, LocalizationKey); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatal(err)
	}
	policy.allowed = true
	mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array([]int64{3}), LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	if err := m.guardCatalogMutation(t.Context(), []int64{3}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionGroupOnlyUpdatesCannotChangeResources(t *testing.T) {
	before := json.RawMessage(`{"group_order":["admin:system:user"],"messages":{"en-US":{"frontend":{"groups":{"admin:system:user":"Users"},"resources":{"admin:system:user":"Users"}}}}}`)
	groups := json.RawMessage(`{"group_order":[],"messages":{"en-US":{"frontend":{"groups":{"admin:system:user":"People"},"resources":{"admin:system:user":"Users"}}}}}`)
	if !permissionGroupsOnly(before, groups) {
		t.Fatal("group-only update rejected")
	}
	if permissionGroupsOnly(before, json.RawMessage(strings.Replace(string(groups), `"resources":{"admin:system:user":"Users"}`, `"resources":{"admin:system:user":"Changed"}`, 1))) {
		t.Fatal("resource edit crossed group boundary")
	}
	if permissionGroupsOnly(before, json.RawMessage(`{"messages":null}`)) {
		t.Fatal("malformed update accepted")
	}
}
