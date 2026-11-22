package auth

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func expectEnrollmentPreference(mock sqlmock.Sqlmock, skipped, everBound bool) {
	mock.ExpectQuery("SELECT metadata @>").WithArgs(int64(3), "EXP78RGH", int64(1)).WillReturnRows(sqlmock.NewRows([]string{"skipped", "ever_bound"}).AddRow(skipped, everBound))
}

func expectPasswordResetOrigin(mock sqlmock.Sqlmock, afterAdminReset bool) {
	mock.ExpectQuery("SELECT admin_password_reset_pending FROM t_user").
		WithArgs(int64(3), "EXP78RGH", int64(1), userStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"admin_password_reset_pending"}).AddRow(afterAdminReset))
}

func TestOptionalEnrollmentPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                                            string
		optional, skipped, everBound, required, canSkip bool
	}{
		{"mandatory", false, true, false, true, false},
		{"offer skip", true, false, false, true, true},
		{"remember skip", true, true, false, false, true},
		{"recovery", true, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, identity, _ := newVerificationTest(t)
			m.verification.config.TOTPEnrollmentOptional = tc.optional
			expectTOTPBound(mock, false)
			if tc.optional {
				expectEnrollmentPreference(mock, tc.skipped, tc.everBound)
			}
			required, canSkip, err := m.totpEnrollmentPolicy(ctx, identity)
			if err != nil || required != tc.required || canSkip != tc.canSkip {
				t.Fatal(required, canSkip, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEnrollmentStatusUsesDatabaseSkipPermission(t *testing.T) {
	for _, tc := range []struct {
		name      string
		everBound bool
	}{
		{"first enrollment may skip", false},
		{"previously bound account must rebind", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, _, _ := newVerificationTest(t)
			m.verification.config.TOTPEnrollmentOptional = true
			expectTOTPBound(mock, false)
			expectEnrollmentPreference(mock, false, tc.everBound)
			status, err := m.Status(ctx, StatusQuery{Purpose: "totp_enrollment", Fields: report.Fields{"requirements"}})
			if err != nil {
				t.Fatal(err)
			}
			allowed := status.Requirements.TOTPSkipAllowed
			if allowed == nil || *allowed == tc.everBound {
				t.Fatalf("unexpected skip permission for everBound=%v: %+v", tc.everBound, status.Requirements)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOptionalEnrollmentStillRequiresBoundFactorLogin(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	m.verification.config.TOTPEnrollmentOptional = true
	identity.Methods.SecondFactor = ""
	expectTOTPBound(mock, true)
	if _, _, err := m.totpEnrollmentPolicy(ctx, identity); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}

func TestSkipOnboardingEnforcesPolicyAndOneStep(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	identity.PasswordResetRequired = true
	ctx = verificationContextWithIdentity(t, ctx, identity)
	for _, input := range []OnboardingPreferences{
		{},
		{EmailBindingSkipped: true, TOTPEnrollmentSkipped: true},
		{PasswordResetSkipped: true, TOTPEnrollmentSkipped: true},
	} {
		if err := m.SkipOnboarding(ctx, input); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if err := m.SkipOnboarding(ctx, OnboardingPreferences{TOTPEnrollmentSkipped: true}); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
	expectPasswordResetOrigin(mock, true)
	m.SetPasswordResetPolicy(false, true)
	if err := m.SkipOnboarding(ctx, OnboardingPreferences{PasswordResetSkipped: true}); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal(err)
	}
	if err := m.SkipOnboarding(t.Context(), OnboardingPreferences{PasswordResetSkipped: true}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}

func TestSkipTOTPBindingPersistsWithCredentialAndHistoryChecks(t *testing.T) {
	m, mock, ctx, _, _ := newVerificationTest(t)
	m.verification.config.TOTPEnrollmentOptional = true
	for _, rows := range []int64{1, 0} {
		mock.ExpectExec("UPDATE t_user SET metadata.*totp_enrollment_skipped.*credential_version=.*totp_ever_bound IS FALSE.*NOT EXISTS").WithArgs(sqlmock.AnyArg(), int64(3), "EXP78RGH", int64(1), userStatusActive).WillReturnResult(sqlmock.NewResult(0, rows))
		err := m.SkipOnboarding(ctx, OnboardingPreferences{TOTPEnrollmentSkipped: true})
		if rows == 1 && err != nil || rows == 0 && !errors.Is(err, ErrTOTPEnrollmentRequired) {
			t.Fatal(err)
		}
	}
	mock.ExpectExec("UPDATE t_user SET metadata").WillReturnError(sql.ErrConnDone)
	if err := m.SkipOnboarding(ctx, OnboardingPreferences{TOTPEnrollmentSkipped: true}); !errors.Is(err, sql.ErrConnDone) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSkipPasswordRequiresTOTPAndPreservesCredentialVersion(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	identity.PasswordResetRequired = true
	ctx = verificationContextWithIdentity(t, ctx, identity)
	m.SetPasswordResetPolicy(true, false)
	expectPasswordResetOrigin(mock, true)
	expectTOTPBound(mock, false)
	if err := m.SkipOnboarding(ctx, OnboardingPreferences{PasswordResetSkipped: true}); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
	expectPasswordResetOrigin(mock, true)
	expectTOTPBound(mock, true)
	mock.ExpectExec("UPDATE t_user SET last_logged_in_at=CASE WHEN login_count=0.*login_count=CASE WHEN login_count=0 THEN 1 ELSE login_count END.*WHERE id=.*credential_version=.*status=").WithArgs(sqlmock.AnyArg(), int64(3), "EXP78RGH", int64(1), userStatusActive).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := m.SkipOnboarding(ctx, OnboardingPreferences{PasswordResetSkipped: true}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalOnboardingStatusAndSensitiveMethods(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	m.verification.config.TOTPEnrollmentOptional = true
	m.SetPasswordResetRequired(false)
	identity.PasswordResetRequired = true
	ctx = verificationContextWithIdentity(t, ctx, identity)
	expectTOTPBound(mock, false)
	expectEnrollmentPreference(mock, false, false)
	expectPasswordResetOrigin(mock, false)
	status, err := m.Status(ctx, StatusQuery{Purpose: "onboarding", Fields: report.Fields{"requirements"}})
	if err != nil || !*status.Requirements.TOTPSkipAllowed || !*status.Requirements.PasswordResetSkipAllowed {
		t.Fatal(status, err)
	}
	identity.PasswordResetRequired = false
	ctx = verificationContextWithIdentity(t, ctx, identity)
	expectTOTPBound(mock, false)
	expectEnrollmentPreference(mock, true, false)
	expectTOTPBound(mock, false)
	if err := m.checkSensitiveTarget(ctx, "DELETE", "/user/99"); err != nil {
		t.Fatal(err)
	}
	expectTOTPBound(mock, false)
	methods, err := m.sensitiveMethods(ctx, identity)
	if err != nil || methods.TOTP || methods.Email {
		t.Fatal(methods, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardingVisibilityPreservesRequiredPolicies(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		hidden, optional, recovery bool
	}{
		{"default visible", false, true, false},
		{"optional hidden", true, true, false},
		{"mandatory visible", true, false, false},
		{"recovery visible", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, identity, _ := newVerificationTest(t)
			attachTestEmail(t, m)
			m.emailCodes.config.RequireBinding = !tc.optional
			m.verification.config.TOTPEnrollmentOptional = tc.optional
			m.SetPasswordResetRequired(!tc.optional)
			m.SetOnboardingVisibility(OnboardingVisibility{TOTP: tc.hidden, PasswordReset: tc.hidden, Email: tc.hidden})
			identity.PasswordResetRequired = true
			ctx = verificationContextWithIdentity(t, ctx, identity)
			expectTOTPBound(mock, false)
			if tc.optional {
				expectEnrollmentPreference(mock, false, tc.recovery)
			}
			if !tc.optional {
				mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(""))
			}
			expectPasswordResetOrigin(mock, false)
			status, err := m.Status(ctx, StatusQuery{Purpose: "onboarding", Fields: report.Fields{"requirements"}})
			if err != nil {
				t.Fatal(err)
			}
			r := status.Requirements
			if *r.TOTPOnboardingHidden != (tc.hidden && tc.optional && !tc.recovery) || *r.PasswordResetOnboardingHidden != (tc.hidden && tc.optional) || *r.EmailOnboardingHidden != (tc.hidden && tc.optional) {
				t.Fatalf("unexpected visibility: %+v", r)
			}
			if !*r.TOTPEnrollmentRequired || !*r.PasswordResetRequired || *r.EmailBindingRequired != !tc.optional {
				t.Fatal("visibility changed enforcement")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
