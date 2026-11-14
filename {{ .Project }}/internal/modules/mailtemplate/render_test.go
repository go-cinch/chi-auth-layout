package mailtemplate

import (
	"strings"
	"testing"
)

func TestRenderTranslationEscapesContextsAndDoesNotRecurse(t *testing.T) {
	input := testInput()
	input.Content.HTML = "<a href=\"\x7b\x7burl\x7d\x7d\">\x7b\x7bcode\x7d\x7d</a>"
	input.Values["url"] = "javascript:alert(1)"
	input.Values["code"] = "\x7b\x7burl\x7d\x7d"
	_, _, html, err := renderTranslation(input.Content, input.Values)
	if err != nil || strings.Contains(html, "javascript:") || !strings.Contains(html, "\x7b\x7burl\x7d\x7d") {
		t.Fatal("unsafe or recursive substitution", err)
	}
}

func TestRenderedAccountMasking(t *testing.T) {
	token := "\x7b\x7baccount\x7d\x7d"
	for _, tc := range []struct{ input, want string }{
		{"example-user", "ex***"}, {"用户张三", "用户***"}, {"a", "a***"}, {"ab", "ab***"}, {"👩界用户", "👩界***"},
	} {
		values := map[string]string{"account": tc.input}
		subject, plain, body, err := renderTranslation(Translation{Subject: token, HTML: "<p>" + token + "</p>", Text: token}, values)
		if err != nil || subject != tc.want || plain != tc.want || body != "<p>"+tc.want+"</p>" {
			t.Fatalf("account masking: %q %q %q %v", subject, plain, body, err)
		}
		if values["account"] != tc.input {
			t.Fatal("renderer mutated caller input")
		}
	}
}
