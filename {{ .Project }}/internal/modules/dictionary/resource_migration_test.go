package dictionary

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPermissionGroupOnlyUpdatesCannotChangeResources(t *testing.T) {
	before := json.RawMessage(`{"group_order":["admin:system:user"],"messages":{"en-US":{"frontend":{"groups":{"admin:system:user":"Users"},"resources":{"admin:system:user":"Users"}}}}}`)
	groups := json.RawMessage(`{"group_order":[],"messages":{"en-US":{"frontend":{"groups":{"admin:system:user":"People"},"resources":{"admin:system:user":"Users"}}}}}`)
	if !permissionGroupsOnly(before, groups) {
		t.Fatal("group-only update rejected")
	}
	if permissionGroupsOnly(before, json.RawMessage(strings.Replace(string(groups), `"resources":{"admin:system:user":"Users"}`, `"resources":{"admin:system:user":"Changed"}`, 1))) {
		t.Fatal("resource edit crossed group boundary")
	}
	if permissionGroupsOnly(before, json.RawMessage(`{"messages":null}`)) {
		t.Fatal("malformed update accepted")
	}
}
