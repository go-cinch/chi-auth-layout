package auth

import (
	"context"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// Each request acknowledges exactly one onboarding step. These flags never
// grant permissions or assert that a password or authenticator was changed.
type OnboardingPreferences struct {
	EmailBindingSkipped  bool `json:"email_binding_skipped,omitempty"`
	TOTPEnrollmentSkipped bool `json:"totp_enrollment_skipped,omitempty"`
	PasswordResetSkipped bool `json:"password_reset_skipped,omitempty"`
}

func (p OnboardingPreferences) valid() bool {
	count := 0
	for _, selected := range []bool{p.EmailBindingSkipped, p.TOTPEnrollmentSkipped, p.PasswordResetSkipped} {
		if selected {
			count++
		}
	}
	return count == 1
}

// OnboardingVisibility controls presentation only; required policies still win.
type OnboardingVisibility struct {
	TOTP           bool
	PasswordReset bool
	Email         bool
}

func (m *Module) SetOnboardingVisibility(visibility OnboardingVisibility) {
	m.onboardingVisibility = visibility
}

func (m *Module) SetPasswordResetRequired(required bool) {
	m.passwordResetOptional = !required
}

func (m *Module) SkipOnboarding(ctx context.Context, input OnboardingPreferences) error {
	if !input.valid() {
		return ErrInvalid
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if input.TOTPEnrollmentSkipped {
		if !m.totpEnabled() || !m.verification.config.TOTPEnrollmentOptional {
			return ErrTOTPEnrollmentRequired
		}
		// The row update serializes against enrollment/recovery and administrator
		// credential updates. Never allow an old session or a recovered factor to skip.
		result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET metadata = jsonb_set(metadata, '{totp_enrollment_skipped}', 'true'::jsonb), updated_at=$1
            WHERE id=$2 AND code=$3 AND credential_version=$4 AND status=$5 AND totp_ever_bound IS FALSE
            AND NOT EXISTS (SELECT 1 FROM t_user_totp WHERE user_id=t_user.id)`, time.Now().UnixMilli(), identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrTOTPEnrollmentRequired
		}
		return nil
	}
	if input.PasswordResetSkipped && !m.passwordResetOptional {
		return authn.ErrPasswordResetRequired
	}
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return err
	}
	if required {
		return ErrTOTPEnrollmentRequired
	}
	if input.PasswordResetSkipped {
		// A skip completes the first-login gate without changing the password or
		// credential version. An administrator reset sets login_count=0 again.
		result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET
            last_logged_in_at=CASE WHEN login_count=0 THEN $1 ELSE last_logged_in_at END,
            login_count=CASE WHEN login_count=0 THEN 1 ELSE login_count END,
            metadata=jsonb_set(metadata, '{password_reset_skipped}', 'true'::jsonb), updated_at=$1
            WHERE id=$2 AND code=$3 AND credential_version=$4 AND status=$5`, time.Now().UnixMilli(), identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrUnauthorized
		}
		return nil
	}
	if identity.PasswordResetRequired {
		return authn.ErrPasswordResetRequired
	}
	return m.SkipEmailBinding(ctx)
}
