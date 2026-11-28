package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/infra/db"
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
	if _, err := m.Create(t.Context(), CreateDictionaryInput{Key: LocalizationKey, Name: "Translations", Value: invalid}); !errors.Is(err, ErrCatalogProtected) {
		t.Fatalf("create invalid catalog: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, value = .* FOR UPDATE").WithArgs(LocalizationKey, testLocalization).WillReturnRows(sqlmock.NewRows([]string{"id", "matches"}).AddRow(3, true))
	mock.ExpectRollback()
	if _, err := m.PublishCatalog(t.Context(), PublishCatalogInput{ExpectedValue: json.RawMessage(testLocalization), Value: invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("update invalid catalog: %v", err)
	}
}

func TestMenuPathUniquenessOnSubmit(t *testing.T) {
	m, _, mock := newTestModule(t)
	catalog, err := localization.Parse([]byte(testLocalization))
	if err != nil {
		t.Fatal(err)
	}
	catalog.Scopes = []string{"admin", "mini"}
	catalog.ResourceCatalog = localization.ResourceCatalog{
		Resources: map[string]localization.Resource{}, Buttons: map[string]localization.Feature{},
	}
	add := func(scope, name string) {
		parent := scope + ":system"
		catalog.ResourceCatalog.Resources[parent] = localization.Resource{Kind: "directory"}
		catalog.ResourceCatalog.Resources[parent+":"+name] = localization.Resource{
			Kind: "resource", Parent: parent,
			Entries: map[string]localization.ResourceEntry{scope + ":menu:" + name: {Path: "/system/user"}},
		}
	}
	encode := func() []byte {
		raw, err := json.Marshal(catalog)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	add("admin", "user")
	add("mini", "user")
	if err := m.prepareLocalization(&Dictionary{Value: encode()}); err != nil {
		t.Fatalf("same path in independent scopes: %v", err)
	}
	add("admin", "alias")
	raw := encode()
	if _, err := localization.Parse(raw); err != nil {
		t.Fatalf("legacy catalog must remain readable: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, value = .* FOR UPDATE").WithArgs(LocalizationKey, testLocalization).WillReturnRows(sqlmock.NewRows([]string{"id", "matches"}).AddRow(3, true))
	mock.ExpectRollback()
	if _, err := m.PublishCatalog(t.Context(), PublishCatalogInput{Value: raw, ExpectedValue: json.RawMessage(testLocalization)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate path published: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	mock.ExpectQuery("SELECT id, value = .* FOR UPDATE").WithArgs(LocalizationKey, testLocalization).WillReturnRows(sqlmock.NewRows([]string{"id", "matches"}).AddRow(3, true))
	mock.ExpectExec("UPDATE t_dictionary SET").WillReturnResult(sqlmock.NewResult(0, 1))
	expectDictionaryGet(mock, 3, LocalizationKey, "Translations", string(updated), true)
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow([]byte(updated)))
	if _, err := m.PublishCatalog(t.Context(), PublishCatalogInput{ExpectedValue: json.RawMessage(testLocalization), Value: updated}); err != nil {
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
