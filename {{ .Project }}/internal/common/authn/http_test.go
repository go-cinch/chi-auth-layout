package authn_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type sessionValidator struct {
	valid bool
}

func (v *sessionValidator) ValidateIdentity(context.Context, authn.Identity) error {
	if !v.valid {
		return authn.ErrUnauthorized
	}
	return nil
}

func TestManagerValidatesSession(t *testing.T) {
	validator := &sessionValidator{valid: true}
	manager, err := authn.New("test", testKey, time.Hour, validator)
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1"}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/user", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if _, err := manager.AuthenticateRequest(request); err != nil {
		t.Fatal(err)
	}
	validator.valid = false
	if _, err := manager.AuthenticateRequest(request); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("revoked session error = %v", err)
	}
	if _, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH"}); err == nil {
		t.Fatal("sessionless identity was accepted")
	}
}

type identityValidatorFunc func(context.Context, authn.Identity) error

func (f identityValidatorFunc) ValidateIdentity(ctx context.Context, identity authn.Identity) error {
	return f(ctx, identity)
}

type identityResolver struct{ identityValidatorFunc }

func (v identityResolver) ResolveIdentity(ctx context.Context, identity authn.Identity) (authn.Identity, error) {
	return identity, v.ValidateIdentity(ctx, identity)
}

func TestManagerPreservesIdentityErrors(t *testing.T) {
	storageErr := errors.New("identity storage unavailable")
	for _, resolve := range []bool{false, true} {
		for _, want := range []error{nil, authn.ErrUnauthorized, storageErr, context.Canceled, context.DeadlineExceeded} {
			t.Run(fmt.Sprintf("resolver=%t/error=%v", resolve, want), func(t *testing.T) {
				ctx := t.Context()
				identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1"}
				validate := identityValidatorFunc(func(gotCtx context.Context, got authn.Identity) error {
					if gotCtx != ctx || got != identity {
						t.Fatalf("identity validation lost request context or identity")
					}
					if want == nil {
						return nil
					}
					return fmt.Errorf("validate stored identity: %w", want)
				})
				var validator authn.IdentityValidator = validate
				if resolve {
					validator = identityResolver{validate}
				}
				manager, err := authn.New("test", testKey, time.Hour, validator)
				if err != nil {
					t.Fatal(err)
				}
				token, _, err := manager.Issue(identity)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest("GET", "/user", nil).WithContext(ctx)
				request.Header.Set("Authorization", "Bearer "+token)
				result, err := manager.AuthenticateRequest(request)
				if !errors.Is(err, want) {
					t.Fatalf("authentication error = %v, want %v", err, want)
				}
				if want != nil && result != nil {
					t.Fatal("failed authentication returned an authenticated request")
				}
			})
		}
	}
}

type restrictedIdentity struct{}

func (restrictedIdentity) ValidateIdentity(context.Context, authn.Identity) error { return nil }

func (restrictedIdentity) ResolveIdentity(_ context.Context, identity authn.Identity) (authn.Identity, error) {
	identity.PasswordResetRequired = true
	return identity, nil
}

func TestRestrictedHTTPUsesInjectedPolicy(t *testing.T) {
	manager, err := authn.New("policy", testKey, time.Minute, restrictedIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 1, Username: "policy", Code: "ABCDEFGH", CredentialVersion: 1, SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/custom/recovery", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if _, err := manager.AuthenticateRequest(request); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal("missing policy must deny", err)
	}
	manager.SetRestrictedHTTPPolicy(func(method, path string) bool { return method == "POST" && path == "/custom/recovery" })
	if _, err := manager.AuthenticateRequest(request); err != nil {
		t.Fatal(err)
	}
	request.Method = "GET"
	if _, err := manager.AuthenticateRequest(request); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal(err)
	}
}
