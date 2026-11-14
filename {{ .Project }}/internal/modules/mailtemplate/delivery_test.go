package mailtemplate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
)

func TestRenderPublishedReadsEachAttempt(t *testing.T) {
	m, mock := testModule(t)
	for _, subject := range []string{"v1", "v2"} {
		content := sampleContent()
		for locale, value := range content {
			value.Subject = subject
			value.Text = "Code: \x7b\x7bcode\x7d\x7d"
			value.PreviewValues = map[string]string{"code": "sample-secret"}
			content[locale] = value
		}
		raw, _ := json.Marshal(content)
		mock.ExpectQuery("SELECT v.content.*").WithArgs("custom.login").WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow(raw))
		got, text, html, err := m.RenderPublished(t.Context(), "custom.login", "zh-CN", map[string]string{"code": "<actual>"})
		if err != nil || got != subject || !strings.Contains(text, "<actual>") || !strings.Contains(html, "&lt;actual&gt;") || strings.Contains(text, "sample-secret") {
			t.Fatal(got, err)
		}
	}
}

func TestRenderPublishedFailsClosed(t *testing.T) {
	m, mock := testModule(t)
	for _, failure := range []error{sql.ErrNoRows, sql.ErrConnDone} {
		mock.ExpectQuery("SELECT v.content.*").WithArgs("custom.login").WillReturnError(failure)
		_, _, _, err := m.RenderPublished(t.Context(), "custom.login", "zh-CN", nil)
		want := failure
		if errors.Is(failure, sql.ErrNoRows) {
			want = ErrNotFound
		}
		if !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"{", `{}`, `{"zh-CN":{"subject":"Test","html":"<p>Code: \u007b\u007bcode\u007d\u007d</p>","preview_values":{"code":"sample"}}}`} {
		mock.ExpectQuery("SELECT v.content.*").WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow(raw))
		if _, _, _, err := m.RenderPublished(t.Context(), "custom.login", "zh-CN", nil); err == nil {
			t.Fatal("invalid publication rendered")
		}
	}
	for _, id := range []string{"", "INVALID", "../bad"} {
		if _, _, _, err := m.RenderPublished(t.Context(), id, "zh-CN", nil); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
}

func TestRenderPublishedSelectsOnlyAccountLanguage(t *testing.T) {
	for _, language := range []string{"zh-CN", "en-US", "", "fr-FR"} {
		t.Run(language, func(t *testing.T) {
			m, mock := testModule(t)
			content := sampleContent()
			for locale, translation := range content {
				translation.Subject = locale
				translation.Text = locale
				translation.HTML = "<p>" + locale + "</p>"
				content[locale] = translation
			}
			raw, _ := json.Marshal(content)
			mock.ExpectQuery("SELECT v.content.*").WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow(raw))
			subject, plain, html, err := m.RenderPublished(t.Context(), "custom.login", language, nil)
			want := "zh-CN"
			if language == "en-US" { want = language }
			if err != nil || subject != want || plain != want || html != `<div lang="`+want+`"><p>`+want+`</p></div>` {
				t.Fatalf("wrong single-language rendering: %q %q %q %v", subject, plain, html, err)
			}
		})
	}
}
