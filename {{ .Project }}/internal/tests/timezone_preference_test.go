package tests

import "testing"

func TestGlobalTimezoneBaseline(t *testing.T) {
	store := permissionDatabase(t)
	var legacy bool
	if err := store.DB.QueryRowContext(t.Context(), `SELECT value #> '{app,timezone}' IS NOT NULL FROM t_dictionary WHERE dictionary_key='UI_PREFERENCES_VBEN_PC'`).Scan(&legacy); err != nil || legacy {
		t.Fatal("client defaults must not own global timezone", legacy, err)
	}
	var overrides int
	if err := store.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM t_user WHERE metadata ? 'timezone' OR metadata #> '{ui_preferences,vben_pc,app,timezone}' IS NOT NULL`).Scan(&overrides); err != nil || overrides != 0 {
		t.Fatal("fresh users must follow the system timezone", overrides, err)
	}
}
