package authn

import (
	"context"
)

type Method string

const (
	Password  Method = "password"
	EmailCode Method = "email_code"
	TOTP      Method = "totp"
)

// Methods records the factors actually verified for this login session.
type Methods struct {
	Primary      Method `json:"primary"`
	SecondFactor Method `json:"second_factor,omitempty"`
}

func (m Methods) Valid() bool {
	return (m.Primary == Password || m.Primary == EmailCode) &&
		(m.SecondFactor == "" || m.SecondFactor == TOTP)
}

type Identity struct {
	Methods Methods
	// Resolved from the database on every request; never trusted from a token.
	PasswordResetRequired bool
	UserID                int64
	Username              string
	Code                  string
	SessionID             string
	CredentialVersion     int64
}

type IdentityValidator interface {
	// Return ErrUnauthorized for an invalid identity; preserve storage and context errors.
	ValidateIdentity(context.Context, Identity) error
}

type identityContextKey struct{}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}
