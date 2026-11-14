package dictionary

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestCatalog(t *testing.T) {
	baseline := seedTimezones(t)
	if len(baseline.Items) != 36 || baseline.Items[0].Value != "Asia/Shanghai" {
		t.Fatal(baseline)
	}
	for _, raw := range []string{`null`, `{}`, `{"items":null}`, `{"items":[],"offset":8}`, `{"items":[]} {}`} {
		if _, err := ParseTimezoneCatalog([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	for _, change := range []func(*TimezoneOption){
		func(o *TimezoneOption) { o.Value = "Local" },
		func(o *TimezoneOption) { o.Value = "+08:00" },
		func(o *TimezoneOption) { o.Value = "" },
		func(o *TimezoneOption) { o.Value = "Asia/Invalid" },
		func(o *TimezoneOption) { o.Name = map[string]string{"zh-CN": "上海"} },
		func(o *TimezoneOption) { o.Region = "invalid" },
	} {
		value := seedTimezones(t)
		change(&value.Items[0])
		raw, _ := json.Marshal(value)
		if _, err := ParseTimezoneCatalog(raw); err == nil {
			t.Fatal(string(raw))
		}
	}
	duplicate := seedTimezones(t)
	duplicate.Items = append(duplicate.Items, duplicate.Items[0])
	raw, _ := json.Marshal(duplicate)
	if _, err := ParseTimezoneCatalog(raw); err == nil {
		t.Fatal("duplicate accepted")
	}
	baseline.Items[0].Sort = 999
	baseline.Items[1].Enabled = false
	baseline.Items[3].Sort = -1
	result := availableTimezones(baseline)
	if len(result.Items) != 35 || result.Items[0].Value != "Asia/Taipei" || result.Items[len(result.Items)-1].Value != "Asia/Shanghai" {
		t.Fatal(result)
	}
	if len(baseline.Items) != 36 {
		t.Fatal("mutated input")
	}
	empty, err := ParseTimezoneCatalog([]byte(`{"items":[]}`))
	if err != nil || availableTimezones(empty).Items == nil {
		t.Fatal(empty, err)
	}
}

func seedTimezones(t *testing.T) TimezoneCatalog {
	t.Helper()
	value, err := ParseTimezoneCatalog(dictionarySeed(t, "timezones"))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestTimezoneOptions(t *testing.T) {
	m, _, mock := newTestModule(t)
	value := seedTimezones(t)
	value.Items[1].Enabled = false
	raw, _ := json.Marshal(value)
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(TimezoneDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(raw, true))
	result, err := m.TimezoneOptions(t.Context())
	if err != nil || len(result.Items) != 35 || result.Items[0].Value != "Asia/Shanghai" {
		t.Fatal(result, err)
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnError(sql.ErrNoRows)
	result, err = m.TimezoneOptions(t.Context())
	if !errors.Is(err, ErrRequiredMissing) {
		t.Fatal(result, err)
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(TimezoneDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{}`, false))
	if _, err := m.TimezoneOptions(t.Context()); !errors.Is(err, ErrRequiredDisabled) {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnError(sql.ErrConnDone)
	if _, err := m.TimezoneOptions(t.Context()); err == nil {
		t.Fatal("database error suppressed")
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{}`, true))
	if _, err := m.TimezoneOptions(t.Context()); err == nil {
		t.Fatal("invalid catalog accepted")
	}
	if err := m.prepareValue(&Dictionary{Key: TimezoneDictionaryKey, Value: raw}); err != nil {
		t.Fatal(err)
	}
	if err := m.prepareValue(&Dictionary{Key: TimezoneDictionaryKey, Value: json.RawMessage(`{}`)}); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
