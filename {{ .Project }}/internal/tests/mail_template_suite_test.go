package tests

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/mailtemplate"
)

func TestApprovedMailSuiteContractsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	module := mailtemplate.New(store, limits)
	contracts := map[string][]string{
		"auth.sign_in_code":                     {"account", "code", "expires_at"},
		"auth.email_binding_code":               {"account", "code", "expires_at"},
		"auth.sensitive_operation_code":         {"account", "authorization_duration", "code", "expires_at"},
		"auth.totp_reset_code":                  {"account", "code", "expires_at"},
		"auth.confirm_email_unlinking":          {"account", "action_url", "expires_at"},
		"auth.email_unlinked":                   {"account", "audit_url"},
		"auth.email_linked":                     {"account", "audit_url"},
		"auth.totp_linked":                      {"account", "audit_url"},
		"auth.sensitive_operation_notification": {"account", "activity_ended_at", "activity_started_at", "activity_summary", "audit_url"},
	}
	placeholder := regexp.MustCompile(`\x7b\x7b\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\x7d\x7d`)
	for id, expected := range contracts {
		var pk int64
		if err := store.DB.QueryRowContext(t.Context(), "SELECT id FROM t_mail_template WHERE template_id=$1", id).Scan(&pk); err != nil {
			t.Fatal(err)
		}
		record, err := module.Get(t.Context(), pk)
		if err != nil || record.HasDraft || record.Revision != 1 || record.PublishedVersion != 1 {
			t.Fatal("suite is not published", id, err)
		}
		snapshot, err := module.GetVersion(t.Context(), pk, record.PublishedVersion)
		if err != nil || !reflect.DeepEqual(record.Content, snapshot.Content) {
			t.Fatal("snapshot mismatch", id, err)
		}
		for _, locale := range []string{"zh-CN", "en-US"} {
			translation := record.Content[locale]
			keys := map[string]bool{}
			for _, match := range placeholder.FindAllStringSubmatch(translation.Subject+translation.HTML+translation.Text, -1) {
				keys[match[1]] = true
			}
			actual := make([]string, 0, len(keys))
			for key := range keys {
				actual = append(actual, key)
			}
			sort.Strings(actual)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("%s/%s parameters %v, want %v", id, locale, actual, expected)
			}
			if strings.Count(translation.HTML, "\n") < 20 {
				t.Fatal("HTML was not formatted", id, locale)
			}
			if strings.Contains(translation.HTML, "<button") || strings.Contains(translation.HTML, "<script") || strings.Contains(translation.HTML, "light-dark(") {
				t.Fatal("mockup-only markup in email", id)
			}
			values := map[string]string{
				"account": "<real-account>", "code": "765432", "expires_at": "2031-02-03T04:05:06+08:00",
				"authorization_duration": "17 minutes", "action_url": "https://example.test/confirm#token=runtime",
				"audit_url": "https://example.test/audit?username=runtime", "activity_started_at": "runtime-start",
				"activity_ended_at": "runtime-end", "activity_summary": "Created: 3\nDeleted: 2",
			}
			_, text, html, err := module.RenderPublished(t.Context(), id, locale, values)
			if err != nil || !strings.Contains(html, "&lt;r***") || !strings.Contains(text, "<r***") || placeholder.MatchString(html) {
				t.Fatal("runtime rendering failed", id, locale, err)
			}
			if strings.Contains(html, "example-user") || strings.Contains(text, "482916") {
				t.Fatal("preview data leaked into delivery", id)
			}
			if strings.HasSuffix(id, "_code") && (strings.Contains(text, "请勿向他人透露验证码") || strings.Contains(text, "Do not share this code")) {
				t.Fatal("removed disclaimer restored", id)
			}
			if keys["audit_url"] || keys["action_url"] {
				parameter := "action_url"
				if keys["audit_url"] {
					parameter = "audit_url"
				}
				if strings.Count(translation.HTML, "\x7b\x7b"+parameter+"\x7d\x7d") != 2 {
					t.Fatal("button URL fallback missing", id)
				}
				if !strings.Contains(html, `target="_blank"`) || !strings.Contains(html, `rel="noopener noreferrer"`) {
					t.Fatal("new-tab link missing", id)
				}
			}
		}
	}
}

func TestMailTemplateBaselinePublicationAndRestart(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	var templates, versions, initial int
	err := store.DB.QueryRowContext(ctx, `SELECT
        (SELECT count(*) FROM t_mail_template),
        (SELECT count(*) FROM t_mail_template_version),
        (SELECT count(*) FROM t_mail_template_version WHERE version=1 AND published_by=0)`).Scan(&templates, &versions, &initial)
	if err != nil || templates != 9 || versions != 9 || initial != 9 {
		t.Fatal("expected nine initial publications", templates, versions, initial, err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_mail_template SET name='Keep draft',has_draft=TRUE,revision=2 WHERE template_id='auth.sign_in_code'`); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateUp(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	var name string
	var draft bool
	var version, revision int
	if err := store.DB.QueryRowContext(ctx, `SELECT name,has_draft,published_version,revision FROM t_mail_template WHERE template_id='auth.sign_in_code'`).Scan(&name, &draft, &version, &revision); err != nil {
		t.Fatal(err)
	}
	if name != "Keep draft" || !draft || version != 1 || revision != 2 {
		t.Fatal("restart overwrote a saved draft")
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM t_mail_template_version`).Scan(&versions); err != nil || versions != 9 {
		t.Fatal("restart created seed publications", versions, err)
	}
}
