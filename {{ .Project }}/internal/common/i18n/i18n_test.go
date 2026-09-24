package i18n

import (
	"context"
	"testing"
)

func TestLanguageNegotiation(t *testing.T) {
	for _, test := range []struct{ header, want string }{
		{"", English}, {"en", English}, {"en-US", English}, {"zh", Chinese}, {"zh-CN", Chinese}, {"zh-Hans", Chinese},
		{"zh-CN;q=0.2,en-US;q=0.9", English}, {"en;q=0.2,zh;q=0.9", Chinese}, {"fr-FR", English}, {"invalid_!", English}, {"zh;q=0,en;q=1", English},
	} {
		if got := Match(test.header); got != test.want {
			t.Errorf("Match(%q)=%q, want %q", test.header, got, test.want)
		}
	}
	ctx := WithLocale(context.Background(), Chinese)
	if Locale(ctx, "en") != Chinese || Locale(context.Background(), "zh") != Chinese {
		t.Fatal("request locale not resolved")
	}
}

func TestCatalogsAndFallback(t *testing.T) {
	for code, english := range catalogs[English] {
		if english == "" || catalogs[Chinese][code] == "" {
			t.Errorf("missing translation: %s", code)
		}
		if got := Translate("unsupported", code, "fallback"); got != english {
			t.Errorf("missing English fallback: %s", code)
		}
	}
	if len(catalogs[English]) != len(catalogs[Chinese]) {
		t.Fatal("catalog keys differ")
	}
	if Translate(Chinese, "USER_NOT_FOUND", "fallback") != "用户不存在" {
		t.Fatal("Chinese translation missing")
	}
	if Translate(Chinese, "CUSTOM", "custom fallback") != "custom fallback" {
		t.Fatal("diagnostic fallback missing")
	}
}
