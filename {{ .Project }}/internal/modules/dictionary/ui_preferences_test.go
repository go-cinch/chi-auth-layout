package dictionary

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func values(raw string) UIPreferences {
	var result UIPreferences
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		panic(err)
	}
	return result
}

func TestPreferenceContract(t *testing.T) {
	base := seedUIPreferences(t)
	encoded, _ := json.Marshal(base)
	if _, err := ParseUIPreferenceDefaults(encoded); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `{}`, `[]`, `broken`, `{"extra":true}`} {
		if _, err := ParseUIPreferenceDefaults([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{
		`{"theme":{"mode":"auto","radius":"0","font_size":15,"color_primary":"#abcdef","builtin_type":"custom"}}`,
		`{"theme":{"font_size":22},"sidebar":{"width":160},"tabbar":{"max_count":30},"custom":{"default_table_size":10}}`,
		`{"app":{"watermark_content":"","watermark":false},"sidebar":{"width":320},"tabbar":{"max_count":0}}`,
		`{"copyright":{"company_site_link":"https://example.com","icp_link":""},"custom":{"default_table_size":200,"report_title":"report","enable_form_fullscreen":false}}`,
		`{"theme":null}`, `{"theme":{"mode":null}}`, `{}`,
	} {
		if err := ValidateUIPreferencePatch(values(raw)); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{
		`null`, `{"app":{"locale":"en-US"}}`, `{"app":{"access_mode":"backend"}}`, `{"theme":true}`,
		`{"theme":{"mode":"unknown"}}`, `{"theme":{"radius":0.5}}`, `{"theme":{"font_size":23}}`, `{"theme":{"font_size":16.5}}`, `{"theme":{"font_size":"16"}}`,
		`{"theme":{"font_size":14}}`, `{"sidebar":{"width":159}}`, `{"sidebar":{"width":321}}`, `{"tabbar":{"max_count":-1}}`,
		`{"custom":{"default_table_size":9}}`, `{"custom":{"default_table_size":201}}`,
		`{"app":{"watermark":"false"}}`, `{"app":{"timezone":"wrong/timezone"}}`, `{"sidebar":{"width":0}}`, `{"tabbar":{"max_count":31}}`,
		`{"custom":{"default_table_size":25}}`, `{"copyright":{"company_site_link":"javascript:alert(1)"}}`, `{"copyright":{"icp_link":"https://user:pass@example.com"}}`,
		`{"theme":{"color_primary":"url(https://example.com)"}}`, `{"widget":{"order":[]}}`, `{"widget":{"fullscreen_button_position":"fixed"}}`,
		`{"theme":{"mode":42}}`,
	} {
		if err := ValidateUIPreferencePatch(values(raw)); err == nil {
			t.Fatal("accepted", raw)
		}
	}

	order := base["widget"].(map[string]any)["order"].([]any)
	reordered := append([]any(nil), order...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	if err := ValidateUIPreferencePatch(UIPreferences{"widget": map[string]any{"order": reordered}}); err != nil {
		t.Fatal(err)
	}
	reordered[0] = reordered[1]
	if err := ValidateUIPreferencePatch(UIPreferences{"widget": map[string]any{"order": reordered}}); err == nil {
		t.Fatal("duplicate order")
	}
}

func dictionarySeed(t *testing.T, tag string) []byte {
	t.Helper()
	files, err := filepath.Glob("../../infra/db/migrations/*-04-dictionary-default-data.sql")
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "$"+tag+"$")
	if len(parts) != 3 {
		t.Fatalf("missing %s seed", tag)
	}
	return []byte(parts[1])
}

func seedUIPreferences(t *testing.T) UIPreferences {
	t.Helper()
	value, err := ParseUIPreferenceDefaults(dictionarySeed(t, "preferences"))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestUIPreferenceDefaults(t *testing.T) {
	m, _, mock := newTestModule(t)
	if _, err := m.UIPreferenceDefaults(t.Context(), "other"); err != ErrInvalid {
		t.Fatal(err)
	}
	baseline, _ := json.Marshal(seedUIPreferences(t))
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(UIPreferenceDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(baseline, true))
	if _, err := m.UIPreferenceDefaults(t.Context(), UIPreferenceClient); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnError(sql.ErrNoRows)
	if _, err := m.UIPreferenceDefaults(t.Context(), UIPreferenceClient); !errors.Is(err, ErrRequiredMissing) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(UIPreferenceDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{}`, false))
	if _, err := m.UIPreferenceDefaults(t.Context(), UIPreferenceClient); !errors.Is(err, ErrRequiredDisabled) {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnError(sql.ErrConnDone)
	if _, err := m.UIPreferenceDefaults(t.Context(), UIPreferenceClient); err == nil {
		t.Fatal("missing database error")
	}
	mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{}`, true))
	if _, err := m.UIPreferenceDefaults(t.Context(), UIPreferenceClient); err == nil {
		t.Fatal("invalid defaults")
	}
	if err := m.prepareValue(&Dictionary{Key: UIPreferenceDictionaryKey, Value: baseline}); err != nil {
		t.Fatal(err)
	}
	if err := m.prepareValue(&Dictionary{Key: UIPreferenceDictionaryKey, Value: json.RawMessage(`{"app":{"locale":"en-US"}}`)}); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
