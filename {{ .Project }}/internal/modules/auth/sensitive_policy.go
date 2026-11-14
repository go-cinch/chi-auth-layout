package auth

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/action"
)

// SensitiveHTTP is deliberately independent of mutable permission catalogs.
// Deleting a catalog binding must never disable step-up enforcement.
func SensitiveHTTP(method, target string) bool {
	target = path.Clean(target)
	if method == http.MethodDelete {
		return true
	}
	if method != http.MethodPost && method != http.MethodPatch && method != http.MethodPut {
		return false
	}
	// Draft creation, autosave, restoration and discard do not publish content.
	if method == http.MethodPost && target == "/mail-template" ||
		method == http.MethodPatch && matchRouteTemplate("/mail-template/{id}", target) {
		return false
	}
	for _, root := range []string{"/mail-template", "/dictionary", "/action", "/role", "/user-group", "/user"} {
		if target == root || strings.HasPrefix(target, root+"/") {
			return true
		}
	}
	return false
}

func (m *Module) CheckHTTPSecurity(r *http.Request) error {
	if r.URL.Path == m.httpPath+m.PermissionEndpoint() {
		return nil
	} // gateway checks its actual target in the handler
	if err := m.checkSensitiveRequest(r, r.Method, r.URL.Path); err != nil {
		return err
	}
	if err := m.checkEmailTarget(r.Context(), r.Method, r.URL.Path); err != nil {
		return err
	}
	if m.sensitiveEmailEnabled() && m.sensitiveTarget(r.Method, r.URL.Path) {
		identity, ok := authn.FromContext(r.Context())
		if !ok {
			return ErrUnauthorized
		}
		// Record only the actual business mutation, never a gateway authorization or failed retry.
		operation := r.Method + " " + path.Clean(r.URL.Path)
		if relative, owned := RelativeAuthHTTPPath(path.Clean(r.URL.Path), m.httpPath); owned && ownsAuthHTTPMethod(relative, r.Method) {
			// Normalize Auth-owned routes so a mount below /role or /user is not a grant mutation.
			operation = r.Method + " /auth" + relative
		}
		ctx := db.WithMutationRecorder(r.Context(), action.WithPermissionMutation, func(tx context.Context) error {
			return m.emailCodes.recordSensitiveOperation(tx, identity.UserID, operation)
		})
		*r = *r.WithContext(ctx)
	}
	return nil
}

func (m *Module) sensitiveTarget(method, target string) bool {
	relative, owned := RelativeAuthHTTPPath(path.Clean(target), m.httpPath)
	if owned && ownsAuthHTTPMethod(relative, method) {
		return method == http.MethodPatch && relative == "/change/pwd" ||
			method == http.MethodPost && (matchRouteTemplate("/totp/reset/{id}", relative) || relative == "/totp/setup" || relative == "/totp/confirm" || relative == "/email/bind")
	}
	return SensitiveHTTP(method, target)
}

func (m *Module) checkSensitiveTarget(ctx context.Context, method, target string) error {
	if !m.totpEnabled() && !m.sensitiveEmailEnabled() {
		return nil
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	target = path.Clean(target)
	relative, owned := RelativeAuthHTTPPath(target, m.httpPath)
	owned = owned && ownsAuthHTTPMethod(relative, method)
	enrollmentAllowed := owned && ((method == "PATCH" && relative == "/info") || (method == "POST" && (relative == "/challenge" || relative == "/totp/challenge" || relative == "/totp/setup" || relative == "/totp/confirm" || relative == "/email/sensitive-code" || relative == "/logout")) || (method == "GET" && (relative == "/totp" || relative == "/status")))
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return err
	}
	if required && !enrollmentAllowed {
		return ErrTOTPEnrollmentRequired
	}
	// Auth owns its route policy even when mounted below a business resource.
	// Binding another credential requires proof from an existing verified factor.
	// Password reset, testing and logout keep their dedicated policies.
	if !m.sensitiveTarget(method, target) {
		return nil
	}
	totpAvailable := false
	if !required {
		totpAvailable, err = m.availableTOTP(ctx, identity)
		if err != nil {
			return err
		}
	}
	var email string
	if !totpAvailable {
		if !m.sensitiveEmailEnabled() {
			return nil
		}
		email, err = m.sensitiveEmail(ctx, identity, false)
		if err != nil || email == "" {
			return err
		}
	}
	if m.verification == nil || m.verification.cipher == nil {
		return ErrVerificationUnavailable
	}
	proof, err := m.verification.proof(ctx, identity)
	if err != nil {
		return err
	}
	if proof.EmailBinding != "" {
		if !m.sensitiveEmailEnabled() {
			return ErrStepUpRequired
		}
		if totpAvailable {
			email, err = m.sensitiveEmail(ctx, identity, false)
			if err != nil {
				return err
			}
		}
		if email == "" || m.emailCodes.digest("sensitive-binding", email) != proof.EmailBinding {
			return ErrStepUpRequired
		}
	} else if !totpAvailable {
		return ErrStepUpRequired
	}
	auditctx.StepUp(ctx, proof.Fingerprint, proof.VerifiedAt)
	return nil
}

// The same business request carries the code. Check before idempotency and
// mutations; an existing fixed-window grant avoids consuming a code twice.
func (m *Module) checkSensitiveRequest(r *http.Request, method, target string) error {
	err := m.checkSensitiveTarget(r.Context(), method, target)
	if !errors.Is(err, ErrStepUpRequired) {
		return err
	}
	codes := r.Header.Values(authn.TOTPCodeHeader)
	ids, credentials := r.Header.Values(authn.StepUpChallengeHeader), r.Header.Values(authn.StepUpCredentialHeader)
	if len(codes) != 0 {
		if !m.totpEnabled() || len(codes) != 1 || !validSensitiveCode(codes[0]) || len(ids) != 0 || len(credentials) != 0 {
			return ErrVerificationInvalid
		}
		_, err := m.VerifySensitiveTOTP(r.Context(), codes[0])
		return err
	}
	if len(ids) == 0 && len(credentials) == 0 {
		return err
	}
	if len(ids) != 1 || len(credentials) != 1 {
		return ErrVerificationInvalid
	}
	return m.verifySensitiveCredential(r.Context(), ids[0], credentials[0])
}
