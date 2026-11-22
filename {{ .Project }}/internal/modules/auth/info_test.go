package auth

import (
	"database/sql"
	"encoding/json"
	"slices"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestSelfLanguagePreference(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	for _, language := range []string{"zh-CN", "en-US"} {
		mock.ExpectExec("UPDATE t_user.*SET metadata = jsonb_set.*credential_version = ").
			WithArgs(language, sqlmock.AnyArg(), int64(3), "EXP78RGH", int64(1), userStatusActive).
			WillReturnResult(sqlmock.NewResult(0, 1))
		response := emailHTTP(t, m, ctx, "PATCH", "/info", map[string]any{"metadata": map[string]string{"language": language}})
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		mock.ExpectQuery("SELECT u.id, COALESCE").WithArgs(int64(3), "EXP78RGH").
			WillReturnRows(sqlmock.NewRows([]string{"id", "language", "timezone", "email", "verification", "password"}).AddRow(3, language, "", true, false, true))
		info, err := m.InfoSelected(ctx, "admin", []string{"metadata"})
		if err != nil || info.Metadata.Language != language || !info.Metadata.EmailBindingSkipped {
			t.Fatal(info, err)
		}
	}
	for _, metadata := range []any{
		map[string]any{"language": ""}, map[string]any{"language": "unregistered"},
		map[string]any{"language": 42}, map[string]any{"language": nil},
		map[string]any{"language": "en-US", "email_binding_skipped": true},
		map[string]any{"language": "en-US", "admin": true},
	} {
		response := emailHTTP(t, m, ctx, "PATCH", "/info", map[string]any{"metadata": metadata})
		if response.Code != 400 {
			t.Fatal(metadata, response.Code, response.Body.String())
		}
	}
	if response := emailHTTP(t, m, t.Context(), "PATCH", "/info", map[string]any{"metadata": map[string]string{"language": "en-US"}}); response.Code != 401 {
		t.Fatal(response.Code)
	}
	mock.ExpectExec("UPDATE t_user").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.UpdateLanguage(ctx, "en-US"); err != ErrUnauthorized {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE t_user").WillReturnError(sql.ErrConnDone)
	if err := m.UpdateLanguage(ctx, "en-US"); err == nil {
		t.Fatal("database failure ignored")
	}
	identity, _ := authn.FromContext(ctx)
	identity.PasswordResetRequired = true
	if err := m.UpdateLanguage(verificationContextWithIdentity(t, ctx, identity), "en-US"); err != authn.ErrPasswordResetRequired {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicOptionsUseConfiguredLanguages(t *testing.T) {
	m, _, _ := newAuthTestModule(t)
	localizer := m.catalogSource.(*localization.Module)
	catalog := localizer.Current()
	catalog.DefaultLocale = "fr-FR"
	catalog.Locales = []localization.Language{
{Tag: "fr-FR", Name: "Français", Enabled: true}, {Tag: "en-US", Name: "English", Enabled: false}}
	delete(catalog.Messages, "zh-CN")
	catalog.Messages["fr-FR"] = localization.Messages{Frontend: map[string]any{}}
	raw, _ := json.Marshal(catalog)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	response := emailHTTP(t, m, t.Context(), "GET", "/pub/opt", nil)
	var options SystemProperties
	if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	if options.DefaultLocale != catalog.DefaultLocale || len(options.Locales) != 1 || options.Locales[0].Name != "Français" {
		t.Fatal(options)
	}
	for _, locale := range options.Locales {
		if !locale.Enabled {
			t.Fatal("disabled option exposed")
		}
	}
}

func TestSelfTimezonePreference(t *testing.T) {
    m, manager, mock := newAuthTestModule(t)
    ctx := authenticatedContext(t, manager, m.sessions)
    for _, timezone := range []string{"UTC", "Asia/Tokyo", "America/New_York", "Europe/London"} {
        mock.ExpectExec("UPDATE t_user.*SET metadata = jsonb_set.*timezone.*credential_version = ").
            WithArgs(timezone, sqlmock.AnyArg(), int64(3), "EXP78RGH", int64(1), userStatusActive).
            WillReturnResult(sqlmock.NewResult(0, 1))
        response := emailHTTP(t, m, ctx, "PATCH", "/info", map[string]any{"metadata": map[string]string{"timezone": timezone}})
        if response.Code != 200 { t.Fatal(response.Code, response.Body.String()) }
        mock.ExpectQuery("SELECT u.id, COALESCE").WithArgs(int64(3), "EXP78RGH").
            WillReturnRows(sqlmock.NewRows([]string{"id", "language", "timezone", "email", "verification", "password"}).AddRow(3, "zh-CN", timezone, true, false, true))
        info, err := m.InfoSelected(ctx, "admin", []string{"metadata"})
        if err != nil || info.Metadata.Timezone != timezone || info.Metadata.Language != "zh-CN" { t.Fatal(info, err) }
    }
    for _, metadata := range []any{
        map[string]any{"timezone":""}, map[string]any{"timezone":"Local"}, map[string]any{"timezone":"+08:00"},
        map[string]any{"timezone":"Invalid/Timezone"}, map[string]any{"timezone":"Asia/Tokyo "}, map[string]any{"timezone":42}, map[string]any{"timezone":nil},
        map[string]any{"timezone":"Asia/Tokyo","language":"zh-CN"}, map[string]any{"timezone":"Asia/Tokyo","email_binding_skipped":true},
        map[string]any{"timezone":"Asia/Tokyo","ui_preferences":map[string]any{"vben_pc":map[string]any{}}},
    } {
        if response := emailHTTP(t,m,ctx,"PATCH","/info",map[string]any{"metadata":metadata});response.Code!=400 {t.Fatal(metadata,response.Code,response.Body.String())}
    }
    if _, ok := authn.FromContext(t.Context());ok {t.Fatal("unexpected test identity")}
    if err:=m.UpdateTimezone(t.Context(),"UTC");err!=ErrUnauthorized {t.Fatal(err)}
    identity,_:=authn.FromContext(ctx);identity.PasswordResetRequired=true
    if err:=m.UpdateTimezone(verificationContextWithIdentity(t,ctx,identity),"UTC");err!=authn.ErrPasswordResetRequired {t.Fatal(err)}
    mock.ExpectExec("UPDATE t_user").WillReturnResult(sqlmock.NewResult(0,0))
    if err:=m.UpdateTimezone(ctx,"UTC");err!=ErrUnauthorized {t.Fatal(err)}
    mock.ExpectExec("UPDATE t_user").WillReturnError(sql.ErrConnDone)
    if err:=m.UpdateTimezone(ctx,"UTC");err==nil {t.Fatal("database failure ignored")}
}

func TestProfileActionsFollowScopedTransportGrants(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	for _, granted := range []bool{false, true} {
		rows := permissionRows()
		if granted {
			config := permissionConfig(m.catalogSource.(*localization.Module), "POST", "/totp/test")
			rows.AddRow("admin:read:system:user", config)
		}
		mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(rows)
		p, err := m.userInfoPermission(ctx, 3, "admin")
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		if granted {
			want = []string{"test_totp"}
		}
		if !slices.Equal(p.ProfileActions, want) {
			t.Fatalf("granted=%v: got %v, want %v", granted, p.ProfileActions, want)
		}
	}
}
