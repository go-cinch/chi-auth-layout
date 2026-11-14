package report

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestReadRecords(t *testing.T) {
	type record struct {
		ID    int64   `json:"id"`
		Value *string `json:"value,omitempty"`
	}
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	build := func(value *record, fields Fields) Projection {
		p := Projection{Fields: fields}
		p.Add("value", "stored_value", &value.Value)
		p.Add("id", "id", &value.ID)
		p.Replace("stored_value", "LEFT(stored_value, 160)")
		p.Require("id", &value.ID)
		return p
	}
	fields := Fields{"value"}
	query := `SELECT LEFT\(stored_value, 160\), id FROM records`
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"value", "id"}).AddRow(nil, 7))
	items, err := ReadRecords(t.Context(), queryOnly{database}, fields, build, " FROM records")
	if err != nil || len(items) != 1 || items[0].Value != nil || items[0].ID != 7 {
		t.Fatalf("%+v %v", items, err)
	}
	mock.ExpectQuery(query).WillReturnError(sqlmock.ErrCancelled)
	if _, err := ReadRecords(t.Context(), queryOnly{database}, fields, build, " FROM records"); !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatal(err)
	}
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"value", "id"}).AddRow("value", "invalid"))
	if _, err := ReadRecords(t.Context(), queryOnly{database}, fields, build, " FROM records"); err == nil {
		t.Fatal("scan error ignored")
	}
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"value", "id"}).AddRow("value", 7).RowError(0, sqlmock.ErrCancelled))
	if _, err := ReadRecords(t.Context(), queryOnly{database}, fields, build, " FROM records"); !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// This adapter deliberately offers only queries, not the database's mutation API.
type queryOnly struct{ database *sql.DB }

func (q queryOnly) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return q.database.QueryContext(ctx, query, args...)
}
