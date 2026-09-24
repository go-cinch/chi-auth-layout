package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestIdentityValidator(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, scenario := range []string{"valid", "reset required", "stale", "pending", "locked", "missing", "database failure", "renamed", "changed code", "wrong session user", "wrong session version", "revoked"} {
			t.Run(scenario+map[bool]string{false: "/authorization-off", true: "/authorization-on"}[enabled], func(t *testing.T) {
				m, _, mock := newAuthTestModule(t)
				identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
				issued, err := m.sessions.Issue(t.Context(), identity, false)
				if err != nil {
					t.Fatal(err)
				}
				identity.SessionID = issued.SessionID
				switch scenario {
				case "wrong session user":
					identity.UserID++
				case "wrong session version":
					identity.CredentialVersion++
				case "revoked":
					if err := m.sessions.Revoke(t.Context(), identity.SessionID); err != nil {
						t.Fatal(err)
					}
				default:
					query := mock.ExpectQuery("SELECT username, code, status, credential_version").WithArgs(int64(3), "EXP78RGH")
					status, version, username, code := userStatusActive, int64(1), "readonly", "EXP78RGH"
					switch scenario {
					case "missing":
						query.WillReturnError(sql.ErrNoRows)
					case "database failure":
						query.WillReturnError(sqlmock.ErrCancelled)
					default:
						switch scenario {
						case "stale":
							version = 2
						case "pending":
							status = userStatusPending
						case "locked":
							status = userStatusLocked
						case "renamed":
							username = "renamed"
						case "changed code":
							code = "NEWCODE1"
						}
						count := int64(1)
						if scenario == "reset required" {
							count = 0
						}
						query.WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow(username, code, status, version, count))
					}
				}
				manager, err := authn.New("test", authTestKey, time.Hour, NewIdentityValidator(m.store, m.sessions, Switches{PasswordResetRequired: true}))
				if err != nil {
					t.Fatal(err)
				}
				cfg := &config.Config{}
				cfg.Auth.Authorization.Enabled = enabled
				router, err := server.NewRouter(cfg, manager, nil, m)
				if err != nil {
					t.Fatal(err)
				}
				// A protected business route authenticates even when permission checks are disabled.
				if scenario == "valid" && enabled {
					mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"resource", "menu", "btn"}).AddRow("PATCH|/auth/change/pwd", "", ""))
				}
				token, _, err := manager.Issue(identity)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPatch, "/auth/change/pwd", strings.NewReader("{"))
				request.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				want := http.StatusUnauthorized
				if scenario == "valid" {
					want = http.StatusBadRequest
				}
				if scenario == "reset required" {
					want = http.StatusForbidden
				}
				if response.Code != want {
					t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestIdentityValidatorDisablesPasswordResetRequirement(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
	issued, err := m.sessions.Issue(t.Context(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = issued.SessionID
	mock.ExpectQuery("SELECT username, code, status, credential_version").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(0)),
	)
	validator := NewIdentityValidator(m.store, m.sessions, Switches{})
	resolved, err := validator.ResolveIdentity(t.Context(), identity)
	if err != nil || resolved.PasswordResetRequired {
		t.Fatalf("resolved identity: %#v %v", resolved, err)
	}
}
