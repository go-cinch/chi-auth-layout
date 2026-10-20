package localization

import "testing"

func TestRetiredResourcePolicyCannotBeSilentlyIgnored(t *testing.T) {
	for _, raw := range []string{`{"defaults":null}`, `{"defaults":{}}`, `{"resources":{"admin:system:user":{"collection":null}}}`, `{"resources":{"admin:system:user":{"collection":true}}}`, `{`, `{"resources":false}`} {
		if err := rejectRetiredResourceFields([]byte(raw)); err == nil {
			t.Fatalf("accepted retired or malformed policy %s", raw)
		}
	}
	for _, raw := range []string{"", `{}`, `{"resources":{"admin:system:user":{"frontend_visible":false}}}`} {
		if err := rejectRetiredResourceFields([]byte(raw)); err != nil {
			t.Fatal(raw, err)
		}
	}
}
