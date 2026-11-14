package authn_test

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestManagerRejectsUnversionedJWT(t *testing.T) {
	manager, err := authn.New("test", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int64{0, -1} {
		claims := authn.Claims{Methods: authn.Methods{Primary: authn.Password}, UserID: 7, Username: "operator", Code: "ABCDEFGH", SessionID: "sid", CredentialVersion: version,
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

func TestParseRequiresKnownLoginMethods(t *testing.T) {
	manager, err := authn.New("test", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, methods := range []any{
		nil,
		map[string]string{"primary": "other"},
		map[string]string{"primary": "email_code", "second_factor": "email_code"},
		map[string]string{"second_factor": "totp"},
	} {
		claims := jwt.MapClaims{
			"iss": "test", "sub": "1", "uid": 1, "username": "user", "code": "CODE0001",
			"sid": "sid", "credential_version": 1, "jti": "id", "exp": time.Now().Add(time.Hour).Unix(),
		}
		if methods != nil {
			claims["authn"] = methods
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testKey))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Parse(token); !errors.Is(err, authn.ErrUnauthorized) {
			t.Fatalf("accepted unsupported authn %v: %v", methods, err)
		}
	}
}
