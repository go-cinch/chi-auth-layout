package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"github.com/DATA-DOG/go-sqlmock"
)

const testLocalization = `{"schema_version":2,"default_locale":"fr-FR","locales":[{"tag":"fr-FR","name":"Français","enabled":true}],"messages":{"fr-FR":{"backend":{"TEST":"Bonjour"}}}}`

func TestLocalizationStartupRefreshAndRemoval(t *testing.T) {
	localizer := localization.New()
	t.Cleanup(localizer.Reset)
	m, _, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	if err := m.RefreshLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	expect := func(raw string) {
		mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(raw)))
	}
	expect(testLocalization)
	if err := m.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	if localizer.Match("fr") != "fr-FR" || localizer.Translate("fr-FR", "TEST", "") != "Bonjour" {
		t.Fatal("catalog not installed")
	}
	expect(testLocalization)
	if err := m.RefreshLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	expect(strings.ReplaceAll(testLocalization, "Bonjour", "Salut"))
	if err := m.RefreshLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	if localizer.Translate("fr-FR", "TEST", "") != "Salut" {
		t.Fatal("runtime translation stale")
	}
	expect(`{"schema_version":9}`)
	if err := m.RefreshLocalization(t.Context()); err == nil {
		t.Fatal("accepted invalid stored catalog")
	}
	if localizer.Translate("fr-FR", "TEST", "") != "Salut" {
		t.Fatal("invalid refresh discarded last good catalog")
	}
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnError(errors.New("database unavailable"))
	if err := m.RefreshLocalization(t.Context()); err == nil {
		t.Fatal("lost refresh error")
	}
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnError(sql.ErrNoRows)
	if err := m.RefreshLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	if localizer.Match("fr") != i18n.English {
		t.Fatal("disabled dictionary not removed")
	}
}

func TestInvalidLocalizationIsNotCommitted(t *testing.T) {
	m, _, mock := newTestModule(t)
	invalid := json.RawMessage(`{"schema_version":9}`)
	if _, err := m.Create(t.Context(), CreateDictionaryInput{Key: LocalizationKey, Name: "Translations", Value: invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("create invalid catalog: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT dictionary_key, value FROM t_dictionary").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"dictionary_key", "value"}).AddRow(LocalizationKey, []byte(testLocalization)))
	mock.ExpectExec("UPDATE t_dictionary SET").WillReturnResult(sqlmock.NewResult(0, 1))
	expectDictionaryGet(mock, 3, LocalizationKey, "Translations", string(invalid), true)
	mock.ExpectRollback()
	if _, err := m.Update(t.Context(), 3, UpdateDictionaryInput{Value: &invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("update invalid catalog: %v", err)
	}
}

func TestLocalizationRefreshesAfterCommittedWrite(t *testing.T) {
	localizer := localization.New()
	t.Cleanup(localizer.Reset)
	m, _, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(testLocalization)))
	if err := m.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	updated := json.RawMessage(strings.ReplaceAll(testLocalization, "Bonjour", "Salut"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT dictionary_key, value FROM t_dictionary").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"dictionary_key", "value"}).AddRow(LocalizationKey, []byte(testLocalization)))
	mock.ExpectExec("UPDATE t_dictionary SET").WillReturnResult(sqlmock.NewResult(0, 1))
	expectDictionaryGet(mock, 3, LocalizationKey, "Translations", string(updated), true)
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(updated)))
	if _, err := m.Update(t.Context(), 3, UpdateDictionaryInput{Value: &updated}); err != nil {
		t.Fatal(err)
	}
	if localizer.Translate("fr-FR", "TEST", "") != "Salut" {
		t.Fatal("committed update not immediately visible")
	}
}

func TestSeedLocalizationCatalog(t *testing.T) {
	files, err := fs.Glob(db.SQLFiles, db.SQLRoot+"/*-04-dictionary-default-data.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("dictionary migration: %v %v", files, err)
	}
	raw, err := db.SQLFiles.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`'I18N_CATALOG',\s*'[^']+',\s*'((?:[^']|'')*)'::jsonb`).FindStringSubmatch(string(raw))
	if len(match) != 2 {
		t.Fatal("catalog seed missing")
	}
	catalog, err := localization.Parse([]byte(strings.ReplaceAll(match[1], "''", "'")))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Messages["zh-CN"].Backend["AUTH_INFO_SCOPE_REQUIRED"] != "请指定权限作用域" {
		t.Fatal("scope error translation missing")
	}
}

func TestLocalizationRefreshDeadlineIncludesWaitingForPoll(t *testing.T) {
	m, _, _ := newTestModule(t)
	// An in-flight poll owns the gate; a write's refresh must remain bounded and
	// must not query the database after its deadline merely because the gate frees.
	m.localizationGate <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.RefreshLocalization(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("refresh error: %v", err)
		}
	case <-time.After(time.Second):
		<-m.localizationGate
		t.Fatal("refresh ignored its deadline waiting for the poll")
	}
	<-m.localizationGate
	if err := m.RefreshLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
}
