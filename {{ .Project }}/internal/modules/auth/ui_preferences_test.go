package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/url"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

type uiDefaultsStub struct {
	calls int
	err   error
}

func (s *uiDefaultsStub) UIPreferenceDefaults(context.Context, string) (dictionary.UIPreferences, error) {
	s.calls++
	return dictionary.UIPreferences{"theme": map[string]any{"mode": "dark", "radius": "0.25"}}, s.err
}
func uiChanges(raw string) map[string]dictionary.UIPreferences {
	var value map[string]dictionary.UIPreferences
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}

func TestUIPreferenceSelectorsAndPublicDefaults(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	source := &uiDefaultsStub{}
	m.SetUIPreferenceSource(source)
	ctx := authenticatedContext(t, manager, m.sessions)
	for _, path := range []string{"/pub/opt?ui=unknown", "/pub/opt?ui=vben_pc&ui=vben_pc", "/info?f=ui_preferences", "/info?f=id&ui=vben_pc", "/info?ui=vben_pc"} {
		response := emailHTTP(t, m, ctx, "GET", path, nil)
		if response.Code != 400 {
			t.Fatal(path, response.Code, response.Body.String())
		}
	}
	response := emailHTTP(t, m, t.Context(), "GET", "/pub/opt?ui=vben_pc", nil)
	var public SystemProperties
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &public) != nil || public.UIPreferences["vben_pc"] == nil {
		t.Fatal(response.Code, response.Body.String())
	}
	mock.ExpectQuery("SELECT u.id, COALESCE").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"id", "overrides"}).AddRow(3, `{"theme":{"mode":"light"}}`))
	info, err := m.InfoSelected(ctx, "admin", []string{"ui_preferences"}, "vben_pc")
	if err != nil || info.UIPreferences["vben_pc"]["theme"].(map[string]any)["mode"] != "light" {
		t.Fatal(info, err)
	}
	if source.calls != 2 {
		t.Fatal(source.calls)
	}
	source.err = errors.New("offline")
	if response := emailHTTP(t, m, ctx, "GET", "/pub/opt?ui=vben_pc", nil); response.Code != 500 {
		t.Fatal(response.Code)
	}
	if _, err := preferenceClient(url.Values{"ui": {}}); err == nil {
		t.Fatal("empty selector")
	}
	m.SetUIPreferenceSource(nil)
	if _, err := m.uiDefaults(ctx, "vben_pc"); err == nil {
		t.Fatal("missing dependency")
	}
	if _, err := m.uiDefaults(ctx, "other"); err != ErrInvalid {
		t.Fatal(err)
	}
}

func TestUpdateUIPreferences(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	source := &uiDefaultsStub{}
	m.SetUIPreferenceSource(source)
	ctx := authenticatedContext(t, manager, m.sessions)
	for _, raw := range []string{`{}`, `{"other":{}}`, `{"vben_pc":{},"other":{}}`, `{"vben_pc":{"app":{"locale":"en-US"}}}`} {
		if _, err := m.UpdateUIPreferences(ctx, uiChanges(raw)); err != ErrInvalid {
			t.Fatal(raw, err)
		}
	}
	if _, err := m.UpdateUIPreferences(t.Context(), uiChanges(`{"vben_pc":{}}`)); err != ErrUnauthorized {
		t.Fatal(err)
	}
	identity, _ := authn.FromContext(ctx)
	identity.PasswordResetRequired = true
	if _, err := m.UpdateUIPreferences(verificationContextWithIdentity(t, ctx, identity), uiChanges(`{"vben_pc":{}}`)); err != authn.ErrPasswordResetRequired {
		t.Fatal(err)
	}
	for _, reset := range []bool{false, true} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COALESCE.*FOR UPDATE").WithArgs(int64(3), "EXP78RGH", int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"overrides"}).AddRow(`{"theme":{"mode":"light"}}`))
		encoded := `{"vben_pc":{"theme":{"mode":"light","radius":"0.5"}}}`
		patch := uiChanges(`{"vben_pc":{"theme":{"radius":"0.5"}}}`)
		if reset {
			encoded = `{}`
			patch = uiChanges(`{"vben_pc":null}`)
		}
		mock.ExpectExec("UPDATE t_user SET metadata = jsonb_set").WithArgs(encoded, sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		var result *UIPreferencesResult
		var err error
		if !reset {
			response := emailHTTP(t, m, ctx, "PATCH", "/info", map[string]any{"metadata": map[string]any{"ui_preferences": patch}})
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			err = json.Unmarshal(response.Body.Bytes(), &result)
		} else {
			result, err = m.UpdateUIPreferences(ctx, patch)
		}
		if err != nil || result.UIPreferences["vben_pc"] == nil {
			t.Fatal(result, err)
		}
	}
	for _, failure := range []string{"source", "missing", "query", "write", "commit"} {
		mock.ExpectBegin()
		source.err = nil
		switch failure {
		case "source":
			source.err = sql.ErrConnDone
		case "missing":
			mock.ExpectQuery("SELECT COALESCE").WillReturnError(sql.ErrNoRows)
		case "query":
			mock.ExpectQuery("SELECT COALESCE").WillReturnError(sql.ErrConnDone)
		default:
			mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"overrides"}).AddRow(`{}`))
			if failure == "write" {
				mock.ExpectExec("UPDATE t_user").WillReturnError(sql.ErrConnDone)
			} else {
				mock.ExpectExec("UPDATE t_user").WillReturnResult(sqlmock.NewResult(0, 1))
			}
		}
		if failure == "commit" {
			mock.ExpectCommit().WillReturnError(sql.ErrConnDone)
		} else {
			mock.ExpectRollback()
		}
		if _, err := m.UpdateUIPreferences(ctx, uiChanges(`{"vben_pc":{}}`)); err == nil {
			t.Fatal(failure)
		}
	}
}
