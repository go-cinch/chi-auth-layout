package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

// Called from the real authenticated email fixture; all data lives in its disposable schema.
func verifyUIPreferencePersistence(t *testing.T, ctx context.Context, store *db.Store, module *auth.Module, dictionaries *dictionary.Module) {
	t.Helper()
	module.SetUIPreferenceSource(dictionaries)
	defaults, err := dictionaries.UIPreferenceDefaults(ctx, dictionary.UIPreferenceClient)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(defaults)
	defer func() {
		if _, err := store.DB.ExecContext(context.Background(), `UPDATE t_dictionary SET value=$1::jsonb WHERE dictionary_key=$2`, string(original), dictionary.UIPreferenceDictionaryKey); err != nil {
			t.Error(err)
		}
	}()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE t_user SET metadata=metadata || '{"ui_preferences":{"other_client":{"keep":true}}}'::jsonb WHERE id=103`)
	change := func(raw string) {
		t.Helper()
		var patch map[string]dictionary.UIPreferences
		if err := json.Unmarshal([]byte(raw), &patch); err != nil {
			t.Fatal(err)
		}
		if _, err := module.UpdateUIPreferences(ctx, patch); err != nil {
			t.Fatal(err)
		}
	}
	stored := func() map[string]any {
		t.Helper()
		var raw []byte
		if err := store.DB.QueryRowContext(ctx, `SELECT metadata FROM t_user WHERE id=103`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		return value
	}
	info := func() dictionary.UIPreferences {
		t.Helper()
		value, err := module.InfoSelected(ctx, "admin", report.Fields{"ui_preferences"}, dictionary.UIPreferenceClient)
		if err != nil {
			t.Fatal(err)
		}
		return value.UIPreferences[dictionary.UIPreferenceClient]
	}
	if !reflect.DeepEqual(info(), defaults) {
		t.Fatal("empty overrides did not resolve defaults")
	}
	change(`{"vben_pc":{"theme":{"radius":"0.5","font_size":16},"custom":{"enable_form_fullscreen":false,"report_title":""},"tabbar":{"max_count":0}}}`)
	metadata := stored()
	if (metadata["language"] != "zh-CN" || metadata["timezone"] != "America/New_York") || metadata["existing"] != float64(42) || metadata["email_binding_skipped"] != true {
		t.Fatal("unrelated metadata lost")
	}
	raw, _ := json.Marshal(metadata["ui_preferences"].(map[string]any)["vben_pc"])
	if string(raw) != `{"custom":{"enable_form_fullscreen":false},"theme":{"radius":"0.5"}}` {
		t.Fatal("stored a full snapshot", string(raw))
	}
	defaults["theme"].(map[string]any)["font_size"] = float64(18)
	updated, _ := json.Marshal(defaults)
	exec(`UPDATE t_dictionary SET value=$1::jsonb WHERE dictionary_key=$2`, string(updated), dictionary.UIPreferenceDictionaryKey)
	resolved := info()["theme"].(map[string]any)
	if resolved["font_size"] != float64(18) || resolved["radius"] != "0.5" {
		t.Fatal("dictionary merge is stale", resolved)
	}
	results := make(chan error, 2)
	for _, raw := range []string{`{"vben_pc":{"theme":{"mode":"light"}}}`, `{"vben_pc":{"custom":{"report_title":"parallel"}}}`} {
		go func(raw string) {
			var patch map[string]dictionary.UIPreferences
			_ = json.Unmarshal([]byte(raw), &patch)
			_, err := module.UpdateUIPreferences(ctx, patch)
			results <- err
		}(raw)
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	resolvedAll := info()
	if resolvedAll["theme"].(map[string]any)["mode"] != "light" || resolvedAll["custom"].(map[string]any)["report_title"] != "parallel" {
		t.Fatal("concurrent edits lost")
	}
	change(`{"vben_pc":{"theme":{"radius":null}}}`)
	if info()["theme"].(map[string]any)["radius"] != defaults["theme"].(map[string]any)["radius"] {
		t.Fatal("leaf reset")
	}
	change(`{"vben_pc":null}`)
	metadata = stored()
	clients := metadata["ui_preferences"].(map[string]any)
	if _, exists := clients["vben_pc"]; exists || clients["other_client"] == nil || (metadata["language"] != "zh-CN" || metadata["timezone"] != "America/New_York") {
		t.Fatal("reset crossed client/language boundary")
	}
	if !reflect.DeepEqual(info(), defaults) {
		t.Fatal("reset failed")
	}
}
