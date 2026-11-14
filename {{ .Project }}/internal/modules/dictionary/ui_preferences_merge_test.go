package dictionary

import (
	"reflect"
	"testing"
)

func TestSparseMergeUIPreferencesAndReset(t *testing.T) {
	defaults := seedUIPreferences(t)
	original := values(`{"theme":{"radius":"0.5","mode":"light"},"custom":{"report_title":"mine"}}`)
	patch := values(`{"theme":{"mode":"auto","font_size":18},"custom":{"report_title":"","enable_form_fullscreen":false},"tabbar":{"max_count":0}}`)
	stored, effective, err := ApplyUIPreferences(defaults, original, patch)
	if err != nil {
		t.Fatal(err)
	}
	expected := values(`{"theme":{"radius":"0.5","mode":"auto","font_size":18},"custom":{"enable_form_fullscreen":false}}`)
	if !reflect.DeepEqual(stored, expected) {
		t.Fatal(stored)
	}
	if effective["theme"].(map[string]any)["radius"] != "0.5" || effective["custom"].(map[string]any)["report_title"] != "" {
		t.Fatal(effective)
	}
	if original["theme"].(map[string]any)["mode"] != "light" {
		t.Fatal("mutated input")
	}
	stored, _, _ = ApplyUIPreferences(defaults, stored, values(`{"theme":{"mode":null,"font_size":16},"custom":null}`))
	if !reflect.DeepEqual(stored, values(`{"theme":{"radius":"0.5"}}`)) {
		t.Fatal(stored)
	}
	stored, effective, _ = ApplyUIPreferences(defaults, stored, nil)
	if len(stored) != 0 || !reflect.DeepEqual(effective, defaults) {
		t.Fatal("reset failed")
	}
	if _, _, err := ApplyUIPreferences(defaults, nil, values(`{"language":"zh-CN"}`)); err == nil {
		t.Fatal("unknown field")
	}
	sanitized := MergeUIPreferences(defaults, values(`{"theme":{"mode":"wrong","font_size":18},"unknown":true}`))
	if sanitized["theme"].(map[string]any)["mode"] != defaults["theme"].(map[string]any)["mode"] {
		t.Fatal(sanitized)
	}
	changed := seedUIPreferences(t)
	changed["theme"].(map[string]any)["radius"] = "0.75"
	result := MergeUIPreferences(changed, values(`{"theme":{"mode":"auto"}}`))
	if result["theme"].(map[string]any)["radius"] != "0.75" {
		t.Fatal("untouched default did not update")
	}
}
