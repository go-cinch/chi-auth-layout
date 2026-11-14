package action

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestActionListGroups(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()

	if _, err := m.ListGroups(ctx, strings.Repeat("x", 51)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\).*GROUP BY.*LIMIT 20").WillReturnRows(
		sqlmock.NewRows([]string{"action_group"}).AddRow("Action").AddRow("User"),
	)
	result, err := m.ListGroups(ctx, "")
	if err != nil || len(result.Items) != 2 || result.Items[0] != "Action" {
		t.Fatalf("list groups: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\).*LIKE.*GROUP BY.*LIMIT 20").WithArgs("%User%").WillReturnRows(
		sqlmock.NewRows([]string{"action_group"}).AddRow("User"),
	)
	result, err = m.ListGroups(ctx, " User ")
	if err != nil || len(result.Items) != 1 || result.Items[0] != "User" {
		t.Fatalf("search groups: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\)").WillReturnError(sql.ErrConnDone)
	if _, err := m.ListGroups(ctx, ""); err == nil {
		t.Fatal("expected database error")
	}
}

func TestActionListGroupsLocalizedSearch(t *testing.T) {
	m, mock := newTestModule(t)
	installActionTestLabels(t, m.catalogSource.(*localization.Module))
	ctx := i18n.WithLocale(t.Context(), "zh-CN")
	mock.ExpectQuery("SELECT MIN.*action_group = ANY").WithArgs("%消息%", pq.Array([]string{"admin:demo:record"})).WillReturnRows(sqlmock.NewRows([]string{"action_group"}).AddRow("admin:demo:record"))
	groups, err := m.ListGroups(ctx, "消息")
	if err != nil || len(groups.Items) != 1 || groups.Items[0] != "admin:demo:record" {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
}

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
