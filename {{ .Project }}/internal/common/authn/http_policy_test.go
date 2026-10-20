package authn_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

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
	token, _, err := manager.Issue(authn.Identity{UserID: 1, Username: "policy", Code: "ABCDEFGH", CredentialVersion: 1, SessionID: "session"})
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
