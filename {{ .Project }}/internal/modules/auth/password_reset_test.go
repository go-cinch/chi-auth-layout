package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-jose/go-jose/v4"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func encryptedResetInput(t *testing.T, c *Credentials, id, password, typ string) EncryptedPasswordResetInput {
	t.Helper()
	enc, err := jose.NewEncrypter(credentialContentAlgorithm, jose.Recipient{Algorithm: credentialKeyAlgorithm, Key: &c.privateKey.PublicKey, KeyID: c.keyID}, (&jose.EncrypterOptions{}).WithType(jose.ContentType(typ)))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"challenge_id": id, "new_password": password})
	object, err := enc.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	value, err := object.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return EncryptedPasswordResetInput{ChallengeID: id, Credential: value}
}

func TestResetCredentialBinding(t *testing.T) {
	c := newCredentialTestManager(t, time.Minute)
	for _, scenario := range []string{"valid", "wrong user", "wrong purpose", "wrong type", "empty password"} {
		t.Run(scenario, func(t *testing.T) {
			challenge, err := c.PasswordResetChallenge(t.Context(), 3)
			if err != nil {
				t.Fatal(err)
			}
			userID, typ, password := int64(3), passwordResetCredentialType, "new-secret"
			switch scenario {
			case "wrong user":
				userID = 4
			case "wrong type":
				typ = passwordCredentialType
			case "wrong purpose":
				challenge, err = c.PasswordChangeChallenge(t.Context(), 3)
			case "empty password":
				password = ""
			}
			if err != nil {
				t.Fatal(err)
			}
			input := encryptedResetInput(t, c, challenge.ChallengeID, password, typ)
			opened, err := c.OpenPasswordReset(t.Context(), userID, input)
			if scenario == "valid" {
				if err != nil || opened.NewPassword != password {
					t.Fatalf("open: %#v %v", opened, err)
				}
				if _, err := c.OpenPasswordReset(t.Context(), 3, input); !errors.Is(err, ErrInvalidPasswordCredential) {
					t.Fatalf("replay: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidPasswordCredential) {
				t.Fatalf("open: %v", err)
			}
		})
	}
}

func TestPasswordResetTransactions(t *testing.T) {
	for _, scenario := range []string{"success", "same password", "already reset", "stale", "locked", "query error", "update error", "commit error"} {
		t.Run(scenario, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			ctx := authenticatedContext(t, manager, m.sessions)
			oldIdentity, _ := authn.FromContext(ctx)
			mock.ExpectBegin()
			query := mock.ExpectQuery("SELECT password, credential_version, login_count, status FROM t_user.*FOR UPDATE").WithArgs(int64(3), "EXP78RGH")
			version, count, status := int64(1), int64(0), userStatusActive
			password := "new-secret"
			switch scenario {
			case "stale":
				version = 2
			case "locked":
				status = userStatusLocked
			case "already reset":
				count = 1
			case "same password":
				password = "cinch123"
			}
			if scenario == "query error" {
				query.WillReturnError(sqlmock.ErrCancelled)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"password", "credential_version", "login_count", "status"}).AddRow(passwordHash(t, "cinch123"), version, count, status))
			}
			switch scenario {
			case "success", "update error", "commit error":
				update := mock.ExpectExec("UPDATE t_user SET password.*credential_version.*login_count = 1.*last_logged_in_at").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH")
				if scenario == "update error" {
					update.WillReturnError(sqlmock.ErrCancelled)
					mock.ExpectRollback()
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					commit := mock.ExpectCommit()
					if scenario == "commit error" {
						commit.WillReturnError(sqlmock.ErrCancelled)
					}
				}
			default:
				mock.ExpectRollback()
			}
			result, err := m.ResetPassword(ctx, PasswordResetInput{NewPassword: password})
			if scenario != "success" {
				if err == nil || result != nil {
					t.Fatalf("reset unexpectedly succeeded: %#v %v", result, err)
				}
				return
			}
			if err != nil || result.PasswordResetRequired {
				t.Fatalf("reset: %#v %v", result, err)
			}
			identity, err := manager.Parse(result.AccessToken)
			if err != nil || identity.CredentialVersion != 2 || identity.SessionID == oldIdentity.SessionID {
				t.Fatalf("new identity: %#v %v", identity, err)
			}
			if _, err := m.sessions.ConsumeRefresh(ctx, result.RefreshToken); err != nil {
				t.Fatal(err)
			}
		})
	}
	m, manager, _ := newAuthTestModule(t)
	if _, err := m.ResetPassword(context.Background(), PasswordResetInput{NewPassword: "secret1"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	ctx := authenticatedContext(t, manager, m.sessions)
	if _, err := m.ResetPassword(ctx, PasswordResetInput{NewPassword: "   "}); !errors.Is(err, ErrInvalidPasswordReset) {
		t.Fatal(err)
	}
}

func TestResetHTTPRestrictedSession(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	manager, err := authn.New("test", authTestKey, time.Hour, NewIdentityValidator(m.store, m.sessions, Switches{PasswordResetRequired: true}))
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
	session, err := m.sessions.Issue(t.Context(), identity, true)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Auth.Authorization.Enabled = enabled
		expectPermission := func() {
			if enabled {
				mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
					sqlmock.NewRows([]string{"resource", "menu", "btn"}).AddRow("POST|/auth/challenge\nPATCH|/auth/reset/pwd", "", ""),
				)
			}
		}
		handler, err := server.NewRouter(cfg, manager, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		request := func(method, path, body string, want int) *httptest.ResponseRecorder {
			t.Helper()
			mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(0)))
			if authn.PasswordResetAllowedHTTP(method, path) {
				expectPermission()
			}
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set(permissionHeaderMethod, "GET")
			r.Header.Set(permissionHeaderPath, "/user")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%s %s = %d %s", method, path, w.Code, w.Body.String())
			}
			return w
		}
		for _, path := range []string{"/auth/info", "/auth/permission"} {
			request("GET", path, "", 403)
		}
		request("PATCH", "/auth/change/pwd", "{}", 403)
		request("POST", "/auth/challenge", `{"purpose":"password_change"}`, 403)
		response := request("POST", "/auth/challenge", `{"purpose":"password_reset"}`, 200)
		var challenge CredentialChallenge
		if err := json.Unmarshal(response.Body.Bytes(), &challenge); err != nil {
			t.Fatal(err)
		}
		request("PATCH", "/auth/reset/pwd", `{"new_password":"plaintext"}`, 400)
		request("PATCH", "/auth/reset/pwd", `{} {}`, 400)
		request("PATCH", "/auth/reset/pwd", `{"challenge_id":"bad","credential":"bad"}`, 400)
		input := encryptedResetInput(t, m.credentials, challenge.ChallengeID, "new-secret", passwordResetCredentialType)
		body, _ := json.Marshal(input)
		// Authentication and configured permissions precede the password transaction.
		mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(0)))
		expectPermission()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT password, credential_version, login_count, status").WillReturnRows(sqlmock.NewRows([]string{"password", "credential_version", "login_count", "status"}).AddRow(passwordHash(t, "cinch123"), int64(1), int64(0), userStatusActive))
		mock.ExpectExec("UPDATE t_user SET password").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		r := httptest.NewRequest(http.MethodPatch, "/auth/reset/pwd", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("reset HTTP: %d %s", w.Code, w.Body.String())
		}
		var result LoginResult
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.PasswordResetRequired || result.AccessToken == "" {
			t.Fatal("invalid reset response")
		}
		renewed, err := m.sessions.ConsumeRefresh(t.Context(), result.RefreshToken)
		if err != nil || !renewed.RememberMe {
			t.Fatalf("remember me lost: %#v %v", renewed, err)
		}
	}
}
