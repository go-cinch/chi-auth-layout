package authn_test

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
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
		return errors.New("session revoked")
	}
	return nil
}

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestManager(t *testing.T) {
	manager, err := authn.New("test-service", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := authn.Identity{CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH"}
	token, expiredAt, err := manager.Issue(want)
	if err != nil || token == "" || !expiredAt.After(time.Now()) {
		t.Fatalf("issue: %q %v %v", token, expiredAt, err)
	}
	secondToken, _, err := manager.Issue(want)
	if err != nil || secondToken == token {
		t.Fatalf("second issue returned duplicate token: %v", err)
	}
	got, err := manager.Parse(token)
	if err != nil || got != want {
		t.Fatalf("parse: %#v %v", got, err)
	}
	request := httptest.NewRequest("GET", "/user", nil)
	request.Header.Set("Authorization", "bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := authn.FromContext(request.Context())
	if !ok || got != want {
		t.Fatalf("context identity: %#v %v", got, ok)
	}
}

func TestManagerRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct {
		issuer string
		key    string
		ttl    time.Duration
	}{
		{"", testKey, time.Hour},
		{"test", "short", time.Hour},
		{"test", testKey, 0},
	} {
		if _, err := authn.New(test.issuer, test.key, test.ttl); err == nil {
			t.Fatalf("accepted config: %#v", test)
		}
	}
	manager, err := authn.New("test", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authn.New("test", testKey, time.Hour, nil); err == nil {
		t.Fatal("nil session validator was accepted")
	}
	if _, _, err := manager.Issue(authn.Identity{}); err == nil {
		t.Fatal("accepted empty identity")
	}
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer invalid", "Bearer one two"} {
		request := httptest.NewRequest("GET", "/user", nil)
		request.Header.Set("Authorization", header)
		if _, err := manager.AuthenticateRequest(request); err == nil {
			t.Fatalf("accepted authorization header %q", header)
		}
	}
	other, err := authn.New("other", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := other.Issue(authn.Identity{CredentialVersion: 1, UserID: 1, Username: "user", Code: "CODE0001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(token); err == nil {
		t.Fatal("accepted token from another issuer")
	}
}

func TestManagerValidatesSession(t *testing.T) {
	validator := &sessionValidator{valid: true}
	manager, err := authn.New("test", testKey, time.Hour, validator)
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "session-1"}
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
	if _, err := manager.AuthenticateRequest(request); err == nil {
		t.Fatal("revoked session was accepted")
	}
	if _, _, err := manager.Issue(authn.Identity{CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH"}); err == nil {
		t.Fatal("sessionless identity was accepted")
	}
}

func TestManagerRejectsUnversionedJWT(t *testing.T) {
	manager, err := authn.New("test", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int64{0, -1} {
		claims := authn.Claims{UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "sid", CredentialVersion: version,
			RegisteredClaims: jwt.RegisteredClaims{Issuer: "test", Subject: "7", ID: "token-id", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testKey))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Parse(token); !errors.Is(err, authn.ErrUnauthorized) {
			t.Fatalf("accepted version %d: %v", version, err)
		}
	}
}
