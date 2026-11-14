package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type SensitiveProof struct {
	EmailBinding      string `json:"email_binding,omitempty"`
	UserID            int64  `json:"user_id"`
	SessionID         string `json:"session_id"`
	CredentialVersion int64  `json:"credential_version"`
	Fingerprint       string `json:"fingerprint"`
	VerifiedAt        int64  `json:"verified_at"`
	ExpiredAt         int64  `json:"expired_at"`
}

func (m *Verification) proof(ctx context.Context, identity authn.Identity) (*SensitiveProof, error) {
	var proof SensitiveProof
	err := m.get(ctx, "sensitive:"+identity.SessionID, &proof, false)
	if errors.Is(err, ErrVerificationInvalid) {
		return nil, ErrStepUpRequired
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if proof.UserID != identity.UserID || proof.SessionID != identity.SessionID || proof.CredentialVersion != identity.CredentialVersion || proof.Fingerprint == "" || proof.VerifiedAt > now || proof.ExpiredAt <= now || proof.ExpiredAt-proof.VerifiedAt != m.config.SensitiveLifetime.Milliseconds() {
		return nil, ErrStepUpRequired
	}
	return &proof, nil
}

func (m *Module) clearSensitiveProof(ctx context.Context, sessionID string) error {
	if m.verification == nil || m.verification.state == nil || !validSessionID(sessionID) {
		return nil
	}
	_, err := m.verification.state.Take(ctx, "sensitive:"+sessionID)
	if err == nil || errors.Is(err, ErrVerificationInvalid) {
		return nil
	}
	return fmt.Errorf("clear sensitive verification: %w", err)
}

func (m *Module) validateVerificationSession(ctx context.Context, identity authn.Identity) error {
	_, err := m.sessions.authenticatedSession(ctx, identity)
	if errors.Is(err, ErrSessionNotFound) {
		return ErrUnauthorized
	}
	return err
}

func (m *Module) VerifySensitiveTOTP(ctx context.Context, code string) (*SensitiveProof, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.PasswordResetRequired {
		return nil, ErrUnauthorized
	}
	if !m.totpEnabled() {
		return nil, ErrVerificationUnavailable
	}
	if err := m.validateVerificationSession(ctx, identity); err != nil {
		return nil, err
	}
	if err := m.verification.consumeTOTP(ctx, identity, code); err != nil {
		return nil, err
	}
	return m.grantSensitive(ctx, identity, "")
}

func (m *Module) grantSensitive(ctx context.Context, identity authn.Identity, emailBinding string) (*SensitiveProof, error) {
	fingerprint, err := randomSessionValue()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	proof := &SensitiveProof{EmailBinding: emailBinding, UserID: identity.UserID, SessionID: identity.SessionID, CredentialVersion: identity.CredentialVersion, Fingerprint: fingerprint, VerifiedAt: now.UnixMilli(), ExpiredAt: now.Add(m.verification.config.SensitiveLifetime).UnixMilli()}
	if err := m.verification.put(ctx, "sensitive:"+identity.SessionID, proof, m.verification.config.SensitiveLifetime); err != nil {
		return nil, err
	}
	// An already authenticated verification can finish after logout. Recheck
	// after publishing so that a late writer withdraws the revoked session's
	// grant, while a later logout removes a grant published before revocation.
	if err := m.validateVerificationSession(ctx, identity); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return nil, errors.Join(err, m.clearSensitiveProof(cleanupCtx, identity.SessionID))
	}
	auditctx.StepUp(ctx, proof.Fingerprint, proof.VerifiedAt)
	return proof, nil
}
