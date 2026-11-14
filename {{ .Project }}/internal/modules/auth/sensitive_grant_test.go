package auth

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestSensitiveVerificationCannotRestoreLoggedOutGrant(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	record, err := m.sessions.read(ctx, identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	refreshToken := m.sessions.token(record).RefreshToken
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := totpCode(secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	expectVerificationLock(mock, 1)
	expectTOTPFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	store := &delayedSensitiveStore{VerificationStateStore: state, entered: make(chan struct{}), release: make(chan struct{})}
	m.verification.state = store
	defer func() {
		select {
		case <-store.release:
		default:
			close(store.release)
		}
	}()
	auditCtx, auditState := auditctx.WithState(ctx)
	result := make(chan error, 1)
	go func() { _, err := m.VerifySensitiveTOTP(auditCtx, code); result <- err }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("verification did not reach grant write")
	}
	if err := m.Logout(context.Background(), refreshToken); err != nil {
		t.Fatal(err)
	}
	close(store.release)
	select {
	case err := <-result:
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatal("late verification accepted a revoked session", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late verification did not return")
	}
	if _, err := state.Get(ctx, "sensitive:"+identity.SessionID); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("late verifier restored sensitive grant", err)
	}
	if event := auditState.Snapshot(); event.StepUpFingerprint != "" || event.StepUpVerifiedAt != 0 {
		t.Fatal("late verifier authorized sensitive audit metadata")
	}
	if state.attempts[verificationAttemptKey(identity)] != 1 {
		t.Fatal("logout cleared the consumed verification attempt")
	}
	if _, err := m.VerifySensitiveTOTP(ctx, code); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked session verified again", err)
	}
}

func TestSensitiveProofSessionIsolationAndFixedExpiry(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	if _, err := m.verification.proof(ctx, identity); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal(err)
	}
	secret, _ := newTOTPSecret()
	code, _ := totpCode(secret, time.Now().Unix()/30)
	expectVerificationLock(mock, 1)
	expectTOTPFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	auditCtx, auditState := auditctx.WithState(ctx)
	proof, err := m.VerifySensitiveTOTP(auditCtx, code)
	if err != nil {
		t.Fatal(err)
	}
	if proof.ExpiredAt-proof.VerifiedAt != time.Hour.Milliseconds() || auditState.Snapshot().StepUpFingerprint != proof.Fingerprint {
		t.Fatal("missing proof audit or wrong lifetime")
	}
	before := state.values["sensitive:"+identity.SessionID]
	for range 2 {
		expectTOTPBound(mock, true)
		if err := m.checkSensitiveTarget(auditCtx, "DELETE", "/user/7"); err != nil {
			t.Fatal(err)
		}
	}
	if state.values["sensitive:"+identity.SessionID] != before {
		t.Fatal("ordinary operation renewed proof")
	}
	other := identity
	other.SessionID = "different"
	if _, err := m.verification.proof(ctx, other); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal("proof leaked to another session")
	}
	other = identity
	other.CredentialVersion++
	if _, err := m.verification.proof(ctx, other); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal("proof survived credential reset")
	}
	proof.VerifiedAt = time.Now().Add(-2 * time.Hour).UnixMilli()
	proof.ExpiredAt = proof.VerifiedAt + time.Hour.Milliseconds()
	if err := m.verification.put(ctx, "sensitive:"+identity.SessionID, proof, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := m.verification.proof(ctx, identity); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal("expired proof accepted")
	}
	state.fail = errors.New("redis unavailable")
	if _, err := m.verification.proof(ctx, identity); !errors.Is(err, state.fail) {
		t.Fatal("Redis error failed open")
	}
}

func TestTOTPLoginTicketDoesNotGrantSensitiveAccess(t *testing.T) {
	for _, debug := range []bool{false, true} {
		name := "totp"
		if debug {
			name = "debug"
		}
		t.Run(name, func(t *testing.T) {
			m, mock, ctx, identity, _ := newVerificationTest(t)
			m.verification.config.TOTPDebug = debug
			identity.SessionID = ""
			identity.Methods.SecondFactor = ""
			pending, err := m.beginMFALogin(ctx, identity, false)
			if err != nil || !pending.MFARequired || pending.AccessToken != "" || pending.RefreshToken != "" {
				t.Fatal("login ticket leaked authenticated session")
			}
			secret, _ := newTOTPSecret()
			code, _ := totpCode(secret, time.Now().Unix()/30)
			if debug {
				code = "999999"
			}
			expectVerificationLock(mock, 1)
			expectTOTPFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(2))
			result, err := m.CompleteTOTPLogin(ctx, pending.MFATicket, code)
			if err != nil || result.AccessToken == "" {
				t.Fatal("Verification login failed", err)
			}
			authenticated, err := m.authenticator.Parse(result.AccessToken)
			if err != nil || authenticated.Methods != (authn.Methods{Primary: authn.Password, SecondFactor: authn.TOTP}) {
				t.Fatal("wrong authentication methods after Verification login", authenticated.Methods, err)
			}
			if _, err := m.verification.proof(ctx, authenticated); !errors.Is(err, ErrStepUpRequired) {
				t.Fatal("login created a sensitive grant")
			}
			if _, err := m.CompleteTOTPLogin(ctx, pending.MFATicket, code); !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal("consumed ticket accepted")
			}
			if _, err := m.CompleteTOTPLogin(ctx, "bad", code); !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal("short ticket accepted")
			}
		})
	}
}

func TestSensitiveCustomLifetimeAndRefresh(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	const lifetime = 90*time.Second + 250*time.Millisecond
	m.verification.config.SensitiveLifetime = lifetime
	secret, _ := newTOTPSecret()
	code, _ := totpCode(secret, time.Now().Unix()/30)
	expectVerificationLock(mock, 1)
	expectTOTPFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	proof, err := m.VerifySensitiveTOTP(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	key := "sensitive:" + identity.SessionID
	if proof.ExpiredAt-proof.VerifiedAt != lifetime.Milliseconds() || state.ttls[key] != lifetime {
		t.Fatal("custom proof deadline and Redis TTL diverged")
	}
	if _, err := m.verification.proof(ctx, identity); err != nil {
		t.Fatal("custom proof lifetime rejected", err)
	}
	before := state.values[key]
	session, err := m.sessions.read(ctx, identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow(identity.Username, identity.Code, userStatusActive, identity.CredentialVersion, int64(1)))
	expectTOTPBound(mock, true)
	if _, err := m.Refresh(ctx, m.sessions.token(session).RefreshToken); err != nil {
		t.Fatal(err)
	}
	if state.values[key] != before || state.ttls[key] != lifetime {
		t.Fatal("token refresh extended the sensitive window")
	}
	for _, boundary := range []string{"expired", "future", "old_default", "shortened"} {
		invalid := *proof
		switch boundary {
		case "expired":
			invalid.ExpiredAt = time.Now().UnixMilli()
			invalid.VerifiedAt = invalid.ExpiredAt - lifetime.Milliseconds()
		case "future":
			invalid.VerifiedAt = time.Now().Add(time.Second).UnixMilli()
			invalid.ExpiredAt = invalid.VerifiedAt + lifetime.Milliseconds()
		case "old_default":
			invalid.ExpiredAt = invalid.VerifiedAt + time.Hour.Milliseconds()
		case "shortened":
			invalid.ExpiredAt--
		}
		if err := m.verification.put(ctx, key, &invalid, lifetime); err != nil {
			t.Fatal(err)
		}
		if _, err := m.verification.proof(ctx, identity); !errors.Is(err, ErrStepUpRequired) {
			t.Fatal("invalid configured-window proof accepted", boundary, err)
		}
	}
	if err := m.verification.put(ctx, key, proof, lifetime); err != nil {
		t.Fatal(err)
	}
	m.verification.config.SensitiveLifetime = 2 * time.Second
	if _, err := m.verification.proof(ctx, identity); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal("old proof accepted after window configuration changed", err)
	}
}
