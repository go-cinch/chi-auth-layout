package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

const sensitiveCredentialType = "sensitive+jwe"

type sensitiveCredential struct {
	ChallengeID    string `json:"challenge_id"`
	Method         string `json:"method"`
	Code           string `json:"verification_code"`
	BrowserUUID    string `json:"browser_uuid,omitempty"`
	VerificationID string `json:"verification_id,omitempty"`
}

func sensitiveScope(identity authn.Identity) string {
	return sensitiveCredentialType + ":" + identity.SessionID
}

func validSensitiveCode(code string) bool {
	return len(code) == 6 && strings.Trim(code, "0123456789") == ""
}

func (m *Module) sensitiveEmailEnabled() bool {
	return m.emailCodes != nil && m.emailCodes.sender.Ready()
}

func (m *Module) sensitiveEmail(ctx context.Context, identity authn.Identity, lock bool) (string, error) {
	query := `SELECT COALESCE(CASE WHEN email_verified_at IS NOT NULL THEN email END, '') FROM t_user WHERE id=$1 AND credential_version=$2 AND status=$3`
	if lock {
		query += " FOR NO KEY UPDATE"
	}
	var email string
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.CredentialVersion, userStatusActive).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnauthorized
	}
	return email, err
}

func (m *Module) verifySensitiveCredential(ctx context.Context, challengeID, encrypted string) error {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.SessionID == "" {
		return ErrUnauthorized
	}
	if err := m.allowEnrollmentSensitive(ctx, identity); err != nil { return err }
	raw, err := m.credentials.decrypt(challengeID, encrypted, sensitiveCredentialType, ErrVerificationInvalid)
	if err != nil {
		return err
	}
	var proof sensitiveCredential
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&proof) != nil || d.Decode(new(any)) != io.EOF || proof.ChallengeID != challengeID || !validSensitiveCode(proof.Code) {
		return ErrVerificationInvalid
	}
	if err := m.credentials.consumeChallenge(ctx, challengeID, sensitiveScope(identity), identity.UserID, ErrVerificationInvalid); err != nil {
		return err
	}
	if proof.Method != "email" {
		return ErrVerificationInvalid
	}
	return m.verifySensitiveEmail(ctx, identity, proof)
}

func (m *Module) verifySensitiveEmail(ctx context.Context, identity authn.Identity, proof sensitiveCredential) error {
	if !m.sensitiveEmailEnabled() || !browserUUIDPattern.MatchString(proof.BrowserUUID) || !validSessionID(proof.VerificationID) {
		return ErrVerificationInvalid
	}
	if err := m.validateVerificationSession(ctx, identity); err != nil {
		return err
	}
	var binding string
	err := m.store.Tx(ctx, func(tx context.Context) error {
		email, err := m.sensitiveEmail(tx, identity, true)
		if err != nil || email == "" {
			return err
		}
		valid, err := m.emailCodes.Consume(tx, proof.VerificationID, proof.BrowserUUID, "sensitive", identity.SessionID, email, proof.Code, identity.UserID, identity.CredentialVersion)
		if valid {
			binding = m.emailCodes.digest("sensitive-binding", email)
		}
		return err
	})
	if err != nil {
		return err
	}
	if binding == "" {
		return ErrVerificationInvalid
	}
	_, err = m.grantSensitive(ctx, identity, binding)
	return err
}

// A reset restriction permits mailbox verification only for required enrollment.
func (m *Module) allowEnrollmentSensitive(ctx context.Context, identity authn.Identity) error {
	if !identity.PasswordResetRequired { return nil }
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil { return err }
	if !required { return authn.ErrPasswordResetRequired }
	return nil
}
