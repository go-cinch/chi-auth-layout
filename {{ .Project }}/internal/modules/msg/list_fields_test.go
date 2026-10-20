package msg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
	input := ListInput{Fields: report.Fields{"title"}}
	mock.ExpectQuery("SELECT COUNT.*FROM t_msg").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT m.title FROM t_msg")).WillReturnRows(sqlmock.NewRows([]string{"title"}).AddRow(""))
	value, err := m.ListSent(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Items) != 1 {
		t.Fatalf("items: %+v", value)
	}
	encoded, err := json.Marshal(value.Items[0])
	if err != nil || string(encoded) != `{"title":""}` {
		t.Fatalf("projection: %s %v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, fields := range []report.Fields{
		{}, {"unknown"}, {"title", "title"}} {
		input.Fields = fields
		if _, err := m.ListSent(t.Context(), input); err == nil {
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
	for _, query := range []string{"f=", "f=unknown", "f=title,title", "f=title,", "f=title&f=title", "f=Key"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sent?"+query, nil))
		if response.Code != 400 {
			t.Fatalf("%s: %d %s", query, response.Code, response.Body.String())
		}
	}
	// An empty selected page retains the normal pagination envelope.
	input.Fields = report.Fields{"title"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_msg").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))

	value, err = m.ListSent(t.Context(), input)
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

func TestMessagePreviewAndPersonalReceiptProjection(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := &Module{store: &db.Store{DB: database}, limits: pagination.Limits{MaxP: 10000, MaxS: 10000}}
	for _, sent := range []bool{false, true} {
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
		receipt := "r.read_at"
		if sent {
			receipt = "NULL::bigint"
		}
		mock.ExpectQuery(regexp.QuoteMeta("SELECT m.id, LEFT(m.content, 160), " + receipt + " FROM")).WillReturnRows(sqlmock.NewRows([]string{"id", "summary", "read_at"}).AddRow(1, "短摘要", nil))
		input := ListInput{Fields: report.Fields{"summary", "read_at", "id"}}
		var value *ListResult
		if sent {
			value, err = m.ListSent(t.Context(), input)
		} else {
			value, err = m.List(t.Context(), 7, input)
		}
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(value.Items[0])
		if err != nil || string(encoded) != `{"id":1,"read_at":null,"summary":"短摘要"}` {
			t.Fatalf("%s %v", encoded, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
