package tests

import (
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

func TestTimezoneCatalogMigration(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	var raw []byte
	if err := store.DB.QueryRowContext(ctx, `SELECT value FROM t_dictionary WHERE dictionary_key='TIMEZONE_OPTIONS' AND enabled`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	catalog, err := dictionary.ParseTimezoneCatalog(raw)
	if err != nil || len(catalog.Items) != 36 || catalog.Items[0].Value != "Asia/Shanghai" {
		t.Fatal(catalog, err)
	}
}
