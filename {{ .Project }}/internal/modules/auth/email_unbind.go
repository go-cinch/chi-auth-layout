package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

const unlinkCredentialType = "email-unbind+jwe"

var (
	ErrUnlinkInvalid   = apperror.New("AUTH_UNLINK_INVALID", "this unlink request has expired; start again")
	ErrUnlinkFactor    = apperror.New("AUTH_UNLINK_FACTOR", "link an authenticator before unlinking with a code")
	ErrUnlinkThrottled = apperror.New("AUTH_UNLINK_THROTTLED", "too many password attempts; try again later")
)

func validUnlinkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	// The configured route may be a Vue hash route. The secret is appended as its own fragment.
	if u.Fragment != "" && (!strings.HasPrefix(u.Fragment, "/") || strings.ContainsAny(u.Fragment, "?#")) {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())
}

type EmailUnlinkStatus struct {
	EmailCodeStatus
	Bound    bool   `json:"bound"`
	CanSend  bool   `json:"can_send"`
	Fallback string `json:"fallback"`
}

// A TOTP is carried in X-TOTP-Code. The password fallback uses a JWE.
type EmailUnlinkVerificationInput struct {
	ChallengeID string `json:"challenge_id,omitempty"`
	Credential  string `json:"credential,omitempty"`
}

type EmailUnlinkProof struct {
	ChallengeID      string `json:"challenge_id"`
	Password         string `json:"password,omitempty"`
	VerificationCode string `json:"-"`
}

type EmailUnlinkResult struct {
	AlreadyConfirmed bool `json:"already_confirmed"`
}

func (c *Credentials) openUnlink(ctx context.Context, userID int64, input EmailUnlinkVerificationInput, codes []string) (EmailUnlinkProof, error) {
	var proof EmailUnlinkProof
	if len(codes) != 0 {
		if len(codes) != 1 || input.ChallengeID != "" || input.Credential != "" {
			return proof, ErrInvalidCredential
		}
		if !validSensitiveCode(codes[0]) {
			return proof, ErrVerificationInvalid
		}
		return EmailUnlinkProof{VerificationCode: codes[0]}, nil
	}
	if input.ChallengeID == "" || input.Credential == "" {
		return proof, ErrInvalidCredential
	}
	raw, err := c.decrypt(input.ChallengeID, input.Credential, unlinkCredentialType, ErrInvalidCredential)
	if err != nil {
		return proof, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&proof) != nil || decoder.Decode(new(any)) != io.EOF || proof.ChallengeID != input.ChallengeID || proof.Password == "" {
		return proof, ErrInvalidCredential
	}
	return proof, c.consumeChallenge(ctx, input.ChallengeID, unlinkCredentialType, userID, ErrInvalidCredential)
}

type unlinkAccount struct{ email, fallback string }

func (m *Module) unlinkAccount(ctx context.Context, identity authn.Identity, lock bool) (unlinkAccount, error) {
	var a unlinkAccount
	query := `SELECT COALESCE(u.email,''), CASE WHEN EXISTS(SELECT 1 FROM t_user_totp f WHERE f.user_id=u.id) THEN 'verification' WHEN u.totp_ever_bound IS FALSE THEN 'password' ELSE 'enroll' END FROM t_user u WHERE u.id=$1 AND u.code=$2 AND u.credential_version=$3 AND u.status=$4 AND u.login_count>0`
	if lock {
		query += " FOR NO KEY UPDATE OF u"
	}
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive).Scan(&a.email, &a.fallback)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrUnauthorized
	}
	return a, err
}

func (m *Module) emailUnlinkStatus(ctx context.Context, identity authn.Identity, uuid, ip string) (*EmailUnlinkStatus, error) {
	if !browserUUIDPattern.MatchString(uuid) {
		return nil, ErrInvalid
	}
	a, err := m.unlinkAccount(ctx, identity, false)
	if err != nil {
		return nil, err
	}
	e := m.emailCodes
	state := &EmailUnlinkStatus{Bound: a.email != "", CanSend: e != nil && e.config.UnlinkURL != "" && e.sender.Ready(), Fallback: a.fallback}
	state.ServerTime = time.Now().UnixMilli()
	if !state.Bound || e == nil {
		return state, nil
	}
	state.RetryAt, err = e.retryAt(ctx, e.cooldownKeys(uuid, ip, a.email))
	if err != nil {
		return nil, err
	}
	err = e.store.SQL(ctx).QueryRowContext(ctx, `SELECT verification_id,expired_at FROM t_auth_email_code WHERE user_id=$1 AND credential_version=$2 AND recipient=$3 AND purpose='unlink' AND NOT consumed AND expired_at>$4 ORDER BY created_at DESC LIMIT 1`, identity.UserID, identity.CredentialVersion, a.email, state.ServerTime).Scan(&state.VerificationID, &state.ExpiredAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return state, err
}

func (m *Module) verifyUnlinkProof(ctx context.Context, identity authn.Identity, proof EmailUnlinkProof) error {
	return m.store.Tx(ctx, func(tx context.Context) error {
		a, err := m.unlinkAccount(tx, identity, true)
		if err != nil {
			return err
		}
		if a.email == "" {
			return ErrUnlinkInvalid
		}
		switch a.fallback {
		case "verification":
			if proof.VerificationCode == "" || proof.Password != "" {
				return ErrVerificationInvalid
			}
			if m.verification == nil {
				return ErrVerificationUnavailable
			}
			verifier := m.verification
			if verifier.cipher == nil {
				verifier, err = newVerification(m.store, verifier.state, verifier.config)
				if err != nil {
					return ErrVerificationUnavailable
				}
			}
			if err = verifier.attempt(tx, identity); err != nil {
				return err
			}
			if err = verifier.consumeLockedTOTPCode(tx, identity, proof.VerificationCode); err != nil {
				return err
			}
		case "password":
			if proof.Password == "" || proof.VerificationCode != "" {
				return ErrIncorrectPassword
			}
			count, err := m.passwordGuard.FailureCount(tx, identity.UserID)
			if err != nil {
				return err
			}
			if m.passwordGuard.LockRequired(count) {
				return ErrUnlinkThrottled
			}
			var hash string
			if err := m.store.SQL(tx).QueryRowContext(tx, `SELECT password FROM t_user WHERE id=$1`, identity.UserID).Scan(&hash); err != nil {
				return err
			}
			if !VerifyPassword(hash, proof.Password) {
				if _, err = m.passwordGuard.RecordFailure(tx, identity.UserID); err != nil {
					return err
				}
				return ErrIncorrectPassword
			}
		default:
			return ErrUnlinkFactor
		}
		if err := m.clearBoundEmail(tx, identity.UserID); err != nil { return err }
        if m.sensitiveEmailEnabled() { return m.emailCodes.unlinkNotice(tx, identity.UserID, identity.CredentialVersion, a.email) }
        return nil
	})
}

// The caller holds the account lock. Existing signed-in sessions retain their
// identity; pending email credentials for this address stop working immediately.
func (m *Module) clearBoundEmail(ctx context.Context, userID int64) error {
	now := time.Now().UnixMilli()
	if _, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET email=NULL,email_verified_at=NULL,updated_at=$2 WHERE id=$1`, userID, now); err != nil {
		return err
	}
	_, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_auth_email_code SET consumed=TRUE,updated_at=$2 WHERE user_id=$1 AND purpose IN ('login','unlink','sensitive','totp_reset','sensitive_notice','totp_bound','totp_rebound','email_linked') AND NOT consumed`, userID, now)
	return err
}

func (m *Module) confirmUnlinkToken(ctx context.Context, token string) (EmailUnlinkResult, error) {
	var result EmailUnlinkResult
	e := m.emailCodes
	if e == nil {
		return result, ErrEmailUnavailable
	}
	id, mac, ok := strings.Cut(token, ".")
	if !ok || !validSessionID(id) || !hmac.Equal([]byte(mac), []byte(e.digest("unlink-link", id))) {
		return result, ErrUnlinkInvalid
	}
	var uid int64
	err := m.store.Tx(ctx, func(tx context.Context) error {
		db := m.store.SQL(tx)
		var version int64
		var current string
		// Always lock the owner before the ticket, including during resends.
		err := db.QueryRowContext(tx, `SELECT u.id,u.credential_version,COALESCE(u.email,'') FROM t_auth_email_code t JOIN t_user u ON u.id=t.user_id WHERE t.verification_id=$1 AND t.purpose='unlink' AND u.status=$2 FOR NO KEY UPDATE OF u`, id, userStatusActive).Scan(&uid, &version, &current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnlinkInvalid
		}
		if err != nil {
			return err
		}
		var recipient string
		var sent, consumed bool
		var expires, issuedVersion int64
		err = db.QueryRowContext(tx, `SELECT recipient,credential_version,expired_at,delivered,consumed,unlink_confirmed FROM t_auth_email_code WHERE verification_id=$1 FOR UPDATE`, id).Scan(&recipient, &issuedVersion, &expires, &sent, &consumed, &result.AlreadyConfirmed)
		if err != nil {
			return err
		}
		if result.AlreadyConfirmed {
			return nil
		}
		if consumed || expires <= time.Now().UnixMilli() || current != recipient || issuedVersion != version {
			return ErrUnlinkInvalid
		}
		if !sent {
			return ErrEmailUnavailable
		}
		if err = m.clearBoundEmail(tx, uid); err != nil {
			return err
		}
		if _, err = db.ExecContext(tx, `UPDATE t_auth_email_code SET unlink_confirmed=TRUE, updated_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint WHERE verification_id=$1`, id); err != nil {
			return err
		}
		notice, err := randomSessionValue()
		if err != nil {
			return err
		}
		_, err = db.ExecContext(tx, `INSERT INTO t_auth_email_code(verification_id,browser_uuid,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts) SELECT $2,browser_uuid,client_ip,'unlink_notice',identifier,user_id,recipient,credential_version,'',$3,0 FROM t_auth_email_code WHERE verification_id=$1`, id, notice, time.Now().Add(24*time.Hour).UnixMilli())
		if err != nil { return emailInsertError(err) }; return m.emailCodes.sealEmail(tx, notice)
	})
	if err == nil {
		operation := "confirm_email_unlink"
		if result.AlreadyConfirmed {
			operation = "view_email_unlink_result"
		}
		auditctx.ResourceOperation(ctx, operation, strconv.FormatInt(uid, 10))
	}
	return result, err
}
