package dictionary

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"reflect"

	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

const actionNameCatalog = `{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true},{"tag":"zh-CN","name":"Chinese","enabled":true}],"messages":{"en-US":{"frontend":{"actions":{"admin:grant:system:permission":"Grant access","unrelated":"Keep"}}},"zh-CN":{"frontend":{"actions":{"admin:grant:system:permission":"权限分配"}}}},"extension":{"revision":9007199254740993}}`

func TestActionNameRenamePreservesOtherLocalesAndCatalogExtensions(t *testing.T) {
	m, cache, mock := newTestModule(t)
	mock.ExpectQuery("SELECT id, value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"id", "value"}).AddRow(3, []byte(actionNameCatalog)))
	want := strings.ReplaceAll(actionNameCatalog, "admin:grant:system:permission", "admin:grant:system:permission-admin")
	want = strings.Replace(want, "Grant access", "Manage permissions", 1)
	mock.ExpectExec("UPDATE t_dictionary SET value").WithArgs(int64(3), jsonDocumentArg(want), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	id, err := m.WriteActionNames(t.Context(), "admin:grant:system:permission", "admin:grant:system:permission-admin", map[string]string{"en-US": " Manage permissions "})
	if err != nil || id != 3 {
		t.Fatalf("rename: %d %v", id, err)
	}
	m.RefreshActionNames(t.Context(), id)
	if cache.invalidations != 1 {
		t.Fatal("name update did not invalidate dictionary cache")
	}
}

func TestActionNameUpdateRejectsUnknownLanguagesBeforeWriting(t *testing.T) {
	m, _, mock := newTestModule(t)
	mock.ExpectQuery("SELECT id, value FROM t_dictionary").WithArgs(LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"id", "value"}).AddRow(3, []byte(actionNameCatalog)))
	if _, err := m.WriteActionNames(t.Context(), "", "admin:read:system:user", map[string]string{"fr-FR": "Not registered"}); !errors.Is(err, apperror.InvalidBody) {
		t.Fatal(err)
	}
}

type jsonDocumentArg []byte

func (want jsonDocumentArg) Match(value driver.Value) bool {
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return false
	}
	decode := func(raw []byte) (any, error) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		err := decoder.Decode(&value)
		return value, err
	}
	a, err := decode(want)
	if err != nil {
		return false
	}
	b, err := decode(raw)
	return err == nil && reflect.DeepEqual(a, b)
}
