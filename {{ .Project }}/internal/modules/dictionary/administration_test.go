package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

func expectCatalogTarget(mock sqlmock.Sqlmock, protected bool, ids ...int64) {
	mock.ExpectQuery("SELECT EXISTS.*t_dictionary").WithArgs(pq.Array(ids), pq.Array([]string{LocalizationKey, UIPreferenceDictionaryKey})).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(protected))
}

func TestProtectedDictionaryMutations(t *testing.T) {
	for _, publisher := range []bool{false, true} {
		m, _, mock := newTestModule(t)
		m.SetPermissionAdministration(&catalogAdministration{allowed: publisher})
		rename, disabled := "NEW_KEY", false
		raw := json.RawMessage(`{}`)
		for _, input := range []UpdateDictionaryInput{
			{Key: &rename}, {Enabled: &disabled}, {Value: raw}, {ExpectedValue: raw, Value: raw},
		} {
			mock.ExpectBegin()
			expectCatalogTarget(mock, true, 3)
			mock.ExpectRollback()
			if _, err := m.Update(t.Context(), 3, input); !errors.Is(err, ErrCatalogProtected) {
				t.Fatalf("publisher=%v, input=%+v, err=%v", publisher, input, err)
			}
		}
		for _, ids := range [][]int64{
			{3}, {5, 3},
		} {
			mock.ExpectBegin()
			expectCatalogTarget(mock, true, ids...)
			mock.ExpectRollback()
			if err := m.Delete(t.Context(), ids...); !errors.Is(err, ErrCatalogProtected) {
				t.Fatal(err)
			}
		}
		if !publisher {
			mock.ExpectBegin()
			mock.ExpectRollback()
			if _, err := m.PublishCatalog(t.Context(), PublishCatalogInput{ExpectedValue: raw, Value: raw}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
				t.Fatal(err)
			}
		}
	}
}
