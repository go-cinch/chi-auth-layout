package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ end }}
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func TestListFieldProjection(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	// Nil association modules make accidental enrichment fail immediately.
	m := &Module{store: &db.Store{DB: database}, limits: pagination.Limits{MaxP: 10000, MaxS: 10000}}
	input := ListUsersInput{Fields: report.Fields{"username"}}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT username FROM t_user")).WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow(""))
	value, err := m.List(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Items) != 1 {
		t.Fatalf("items: %+v", value)
	}
	encoded, err := json.Marshal(value.Items[0])
	if err != nil || string(encoded) != `{"username":""}` {
		t.Fatalf("projection: %s %v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, fields := range []report.Fields{
		{}, {"unknown"}, {"username", "username"}} {
		input.Fields = fields
		if _, err := m.List(t.Context(), input); err == nil {
			t.Fatal("accepted invalid selector")
		}
	}
{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else }}
	handler := m.HTTP()
{{ end }}
	for _, query := range []string{"f=", "f=unknown", "f=username,username", "f=username,", "f=username&f=username", "f=Key"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if response.Code != 400 {
			t.Fatalf("%s: %d %s", query, response.Code, response.Body.String())
		}
	}
	// An empty selected page retains the normal pagination envelope.
	input.Fields = report.Fields{"username"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))

	value, err = m.List(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(value)
	if err != nil || string(encoded) != `{"items":[],"t":0,"p":1,"s":1}` {
		t.Fatalf("empty page: %s %v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserListSelectedRoleSummaryBatchesWithoutPermissions(t *testing.T) {
	m, mock := newTestModule(t)
	m.store.DB.SetMaxOpenConns(1)
	size := int32(3)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, username, role_id FROM t_user")).WithArgs(size, int64(0)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "username", "role_id"}).AddRow(3, "c", 7).AddRow(2, "b", 7).AddRow(1, "a", nil)).RowsWillBeClosed()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, word FROM t_role WHERE id = ANY($1)")).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "word"}).AddRow(7, "Reader", "reader"))
	value, err := m.List(t.Context(), ListUsersInput{PageSize: &size, Fields: report.Fields{"id", "username", "role_summary"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value.Items)
	if err != nil || string(encoded) != `[{"id":3,"role_summary":{"id":7,"name":"Reader","word":"reader"},"username":"c"},{"id":2,"role_summary":{"id":7,"name":"Reader","word":"reader"},"username":"b"},{"id":1,"role_summary":null,"username":"a"}]` {
		t.Fatalf("%s %v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserListSelectedCodesSkipAllAssociations(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT action FROM t_user")).WillReturnRows(sqlmock.NewRows([]string{"action"}).AddRow("READ, WRITE,READ"))
	value, err := m.List(t.Context(), ListUsersInput{Fields: report.Fields{"action_codes"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value.Items[0])
	if err != nil || string(encoded) != `{"action_codes":["READ","WRITE"]}` {
		t.Fatalf("%s %v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserListSelectedMetadataRejectsInvalidData(t *testing.T) {
	for _, raw := range []string{`{`, `{"BadKey":true}`} {
		m, mock := newTestModule(t)
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
		mock.ExpectQuery("SELECT CASE WHEN .* metadata.* FROM t_user").WithArgs(sqlmock.AnyArg(), int32(1), int64(0)).WillReturnRows(sqlmock.NewRows([]string{"metadata"}).AddRow([]byte(raw)))
		if _, err := m.List(t.Context(), ListUsersInput{Fields: report.Fields{"metadata"}}); err == nil {
			t.Fatal("accepted invalid metadata")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
