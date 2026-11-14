package authn

import (
	"context"
	"net/http"
	"strings"
)

const (
	VerificationRequiredHeader     = "X-Verification-Required"
	MFATicketHeader       = "X-MFA-Ticket"
	MFAExpiresAtHeader    = "X-MFA-Expires-At"
	StepUpChallengeHeader    = "X-Step-Up-Challenge"
	StepUpCredentialHeader   = "X-Step-Up-Credential"
	TOTPCodeHeader         = "X-TOTP-Code"
	RecoveryCodeHeader = "X-Recovery-Code"
)

func (m *Manager) AuthenticateRequest(r *http.Request) (*http.Request, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, ErrUnauthorized
	}
	identity, err := m.Parse(parts[1])
	if err != nil {
		return nil, ErrUnauthorized
	}
	if resolver, ok := m.sessions.(interface {
		ResolveIdentity(context.Context, Identity) (Identity, error)
	}); ok {
		identity, err = resolver.ResolveIdentity(r.Context(), identity)
		if err != nil {
			return nil, err
		}
	} else if m.sessions != nil {
		if err := m.sessions.ValidateIdentity(r.Context(), identity); err != nil {
			return nil, err
		}
	}
	if identity.PasswordResetRequired && (m.restrictedHTTPAllowed == nil || !m.restrictedHTTPAllowed(r.Method, r.URL.Path)) {
		return nil, ErrPasswordResetRequired
	}
	return r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity)), nil
}

// SetHTTPSecurityPolicy installs a business-owned guard enforced independently
// of permission switches, before idempotency or business side effects.
func (m *Manager) SetHTTPSecurityPolicy(policy func(*http.Request) error) {
	m.httpSecurityPolicy = policy
}

func (m *Manager) CheckHTTPSecurity(r *http.Request) error {
	if m.httpSecurityPolicy == nil {
		return nil
	}
	return m.httpSecurityPolicy(r)
}

// SetRestrictedHTTPPolicy installs a capability-owned policy during startup.
// A missing policy denies all requests by identities restricted to recovery.
func (m *Manager) SetRestrictedHTTPPolicy(allow func(method, path string) bool) {
	m.restrictedHTTPAllowed = allow
}
