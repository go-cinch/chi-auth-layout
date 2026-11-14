package auth

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/url"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestTOTPEnrollmentConfirmsAndRotatesCredentials(t *testing.T) {
	for _, debug := range []bool{false, true} {
		name := "totp"
		if debug {
			name = "debug"
		}
		t.Run(name, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			m.verification.config.TOTPDebug = debug
			m.verification.config.TOTPAccountNamePrefix = "acme admin"
			m.verification.config.TOTPPeriod = time.Minute
			m.verification.config.TOTPEnrollmentTTL = 90 * time.Second
			started := time.Now()
			mock.ExpectQuery("SELECT password FROM").WithArgs(int64(3), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
			expectTOTPBound(mock, false)
			setup, err := m.BeginTOTPEnrollment(ctx, "pass")
			if err != nil {
				t.Fatal(err)
			}
			if setup.URI == "" || setup.Secret == "" {
				t.Fatal("missing enrollment material")
			}
			uri, err := url.Parse(setup.URI)
			if err != nil || uri.Path != "/test:acme admin - readonly" || uri.Query().Get("issuer") != "test" || uri.Query().Get("secret") != setup.Secret {
				t.Fatal("enrollment did not use the configured account name prefix")
			}
			if uri.Query().Get("period") != "60" || state.ttls["enrollment:"+identity.SessionID] != 90*time.Second || setup.ExpiredAt < started.Add(90*time.Second).UnixMilli() || setup.ExpiredAt > time.Now().Add(90*time.Second).UnixMilli() {
				t.Fatal("QR parameters, response expiry and storage TTL do not match configuration")
			}
			// Simulate a restart/config change after scanning the 60-second QR code.
			m.verification.config.TOTPPeriod = 30 * time.Second
			// A malformed code cannot commit a half-completed enrollment.
			if _, err := m.ConfirmTOTPEnrollment(ctx, "invalid"); !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal(err)
			}
			code, _ := totpCode(setup.Secret, time.Now().Unix()/60)
			if debug {
				code = "999999"
			}
			expectVerificationLock(mock, 1)
			expectTOTPBound(mock, false)
			mock.ExpectExec("INSERT INTO t_user_totp").WithArgs(identity.UserID, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(60)).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("DELETE FROM t_user_recovery_code").WillReturnResult(sqlmock.NewResult(0, 0))
			for range 10 {
				mock.ExpectExec("INSERT INTO t_user_recovery_code").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			mock.ExpectExec("UPDATE t_user SET credential_version").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			result, err := m.ConfirmTOTPEnrollment(ctx, code)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.RecoveryCodes) != 10 || result.TOTPEnrollmentRequired {
				t.Fatal("incomplete enrollment result")
			}
			next, err := m.authenticator.Parse(result.AccessToken)
			if err != nil || next.CredentialVersion != 2 || next.SessionID == identity.SessionID {
				t.Fatal("binding did not replace session")
			}
			if _, err := m.verification.proof(ctx, next); !errors.Is(err, ErrStepUpRequired) {
				t.Fatal("binding granted sensitive access")
			}
		})
	}
}

func TestTOTPEnrollmentPolicyAndUnavailableStore(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	expectTOTPBound(mock, false)
	if err := m.checkSensitiveTarget(ctx, "GET", "/info"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal("mandatory setup bypass", err)
	}
	expectTOTPBound(mock, false)
	if err := m.checkSensitiveTarget(ctx, "POST", "/totp/setup"); err != nil {
		t.Fatal("setup blocked", err)
	}
	expectTOTPBound(mock, false)
	if _, err := m.TOTPStatus(ctx); err != nil {
		t.Fatal(err)
	}
	expectTOTPBound(mock, false)
	if err := m.checkSensitiveTarget(ctx, "DELETE", "/msg/inbox/1"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal("unbound deletion allowed", err)
	}
	expectTOTPBound(mock, false)
	if err := m.checkSensitiveTarget(ctx, "GET", "/dictionary"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
	m.verification.state = nil
	if err := m.verification.put(ctx, "a", identity, time.Minute); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal(err)
	}
	if _, err := m.verification.proof(ctx, identity); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal(err)
	}
	if err := m.verification.attempt(ctx, authn.Identity{UserID: 3, CredentialVersion: 1}); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal(err)
	}
}

func TestTOTPLoginEnrollmentGrantIsolation(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	expectPassword := func() {
		mock.ExpectQuery("SELECT password FROM").WithArgs(identity.UserID, identity.CredentialVersion).WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
	}
	expectPassword()
	if _, err := m.BeginTOTPEnrollment(ctx, ""); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatal("ordinary session skipped password", err)
	}
	if err := m.grantTOTPEnrollment(ctx, identity, identity.SessionID); err != nil {
		t.Fatal(err)
	}
	expectTOTPBound(mock, false)
	status, err := m.TOTPStatus(ctx)
	if err != nil || status.PasswordRequired {
		t.Fatal("login session still requires password", err)
	}
	expectPassword()
	expectTOTPBound(mock, false)
	if _, err := m.BeginTOTPEnrollment(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"other session", "other user", "old version", "disabled policy"} {
		t.Run(scenario, func(t *testing.T) {
			other := identity
			switch scenario {
			case "other session":
				other.SessionID = "another"
			case "other user":
				other.UserID++
			case "old version":
				other.CredentialVersion++
			case "disabled policy":
				m.verification.config.TOTPEnabled = false
			}
			allowed, err := m.canEnrollWithoutPassword(ctx, other)
			m.verification.config.TOTPEnabled = true
			if err != nil || allowed {
				t.Fatal("grant escaped its login session", err)
			}
		})
	}
	// Explicit passwords are always checked, including personal-center requests.
	expectPassword()
	if _, err := m.BeginTOTPEnrollment(ctx, "wrong"); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatal(err)
	}
	grant := enrollmentPending{Identity: identity, ExpiredAt: time.Now().Add(-time.Second).UnixMilli()}
	if err := m.verification.put(ctx, "enrollment-grant:"+identity.SessionID, grant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if allowed, err := m.canEnrollWithoutPassword(ctx, identity); err != nil || allowed {
		t.Fatal("expired grant accepted", err)
	}
	state.fail = errors.New("redis unavailable")
	if allowed, err := m.canEnrollWithoutPassword(ctx, identity); err == nil || allowed {
		t.Fatal("store failure bypassed password")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPEnrollmentRejectsMissingOrInvalidPeriod(t *testing.T) {
	for _, seconds := range []int64{0, -1, 9223372037} {
		m, mock, ctx, identity, _ := newVerificationTest(t)
		pending := enrollmentPending{Identity: identity, Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", PeriodSeconds: seconds, ExpiredAt: time.Now().Add(time.Minute).UnixMilli()}
		if err := m.verification.put(ctx, "enrollment:"+identity.SessionID, pending, time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ConfirmTOTPEnrollment(ctx, "123456"); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("invalid enrollment period accepted", seconds, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
