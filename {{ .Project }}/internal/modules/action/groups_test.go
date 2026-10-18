package action

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestPermissionGroupMovesPreserveActionWordsAndRejectOccupiedRemoval(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectExec("UPDATE t_action SET action_group").WithArgs("admin:system:old", "admin:system:new", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(pq.Array([]string{"admin:system:unused"})).WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(false))
	allowed, err := m.ReconcilePermissionGroups(t.Context(), map[string]string{"admin:system:old": "admin:system:new"}, []string{"admin:system:unused"})
	if err != nil || !allowed {
		t.Fatalf("move: %v %v", allowed, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(pq.Array([]string{"admin:system:new"})).WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(true))
	if allowed, err := m.ReconcilePermissionGroups(t.Context(), nil, []string{"admin:system:new"}); err != nil || allowed {
		t.Fatalf("occupied removal: %v %v", allowed, err)
	}
	failure := errors.New("write failed")
	mock.ExpectExec("UPDATE t_action SET action_group").WillReturnError(failure)
	if _, err := m.ReconcilePermissionGroups(t.Context(), map[string]string{"admin:system:old": "admin:system:new"}, nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if allowed, err := m.ReconcilePermissionGroups(t.Context(), nil, nil); err != nil || !allowed {
		t.Fatalf("no-op: %v %v", allowed, err)
	}
}
