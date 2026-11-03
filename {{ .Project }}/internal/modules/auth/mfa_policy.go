package auth

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
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
	for _, root := range []string{"/dictionary", "/action", "/role", "/user-group", "/user"} {
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
	return m.checkMFARequest(r, r.Method, r.URL.Path)
}

func (m *Module) checkMFATarget(ctx context.Context, method, target string) error {
	if !m.mfaEnabled() {
		return nil
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	target = path.Clean(target)
	relative, owned := RelativeAuthHTTPPath(target, m.httpPath)
	owned = owned && authHTTPMethod(relative) == method
	enrollmentAllowed := owned && ((method == "POST" && (relative == "/mfa/challenge" || relative == "/mfa/setup" || relative == "/mfa/confirm" || relative == "/logout")) || (method == "GET" && relative == "/mfa"))
	required, err := m.mfaEnrollmentRequired(ctx, identity)
	if err != nil {
		return err
	}
	if required && !enrollmentAllowed {
		return ErrMFAEnrollmentRequired
	}
	// Auth owns its route policy even when mounted below a business resource.
	// Enrollment, password reset, testing and logout never require step-up.
	sensitive := SensitiveHTTP(method, target)
	if owned {
		sensitive = (method == http.MethodPatch && relative == "/change/pwd") ||
			(method == http.MethodPost && matchRouteTemplate("/mfa/reset/{id}", relative))
	}
	if !sensitive {
		return nil
	}
	proof, err := m.mfa.proof(ctx, identity)
	if err != nil {
		return err
	}
	auditctx.MFA(ctx, proof.Fingerprint, proof.VerifiedAt)
	return nil
}

// The same business request carries the code. Check before idempotency and
// mutations; an existing fixed-window grant avoids consuming a code twice.
func (m *Module) checkMFARequest(r *http.Request, method, target string) error {
	err := m.checkMFATarget(r.Context(), method, target)
	if !errors.Is(err, ErrMFARequired) {
		return err
	}
	code := strings.TrimSpace(r.Header.Get(authn.MFACodeHeader))
	if code == "" {
		return err
	}
	_, err = m.VerifySensitiveMFA(r.Context(), code)
	return err
}
