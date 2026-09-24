package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrPasswordResetRequired = apperror.New("AUTH_PASSWORD_RESET_REQUIRED", "reset your password before continuing")
	ErrInvalidConfig         = errors.New("invalid authentication configuration")
	ErrUnauthorized          = errors.New("unauthorized")
)

type Identity struct {
	// Resolved from the database on every request; never trusted from a token.
	PasswordResetRequired bool
	UserID                int64
	Username              string
	Code                  string
	SessionID             string
	CredentialVersion     int64
}

type Claims struct {
	UserID            int64  `json:"uid"`
	Username          string `json:"username"`
	Code              string `json:"code"`
	SessionID         string `json:"sid,omitempty"`
	CredentialVersion int64  `json:"credential_version"`
	jwt.RegisteredClaims
}

type IdentityValidator interface {
	ValidateIdentity(context.Context, Identity) error
}

type Manager struct {
	issuer   string
	key      []byte
	ttl      time.Duration
	sessions IdentityValidator
}

type identityContextKey struct{}

func New(issuer, key string, ttl time.Duration, validators ...IdentityValidator) (*Manager, error) {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" || len([]byte(key)) < 32 || ttl <= 0 {
		return nil, ErrInvalidConfig
	}
	if len(validators) > 1 || (len(validators) == 1 && validators[0] == nil) {
		return nil, ErrInvalidConfig
	}
	var sessions IdentityValidator
	if len(validators) == 1 {
		sessions = validators[0]
	}
	return &Manager{issuer: issuer, key: []byte(key), ttl: ttl, sessions: sessions}, nil
}

func (m *Manager) Issue(identity Identity) (string, time.Time, error) {
	identity.Username = strings.TrimSpace(identity.Username)
	identity.Code = strings.TrimSpace(identity.Code)
	if identity.CredentialVersion <= 0 || identity.UserID <= 0 || identity.Username == "" || identity.Code == "" || (m.sessions != nil && identity.SessionID == "") {
		return "", time.Time{}, ErrUnauthorized
	}
	now := time.Now().UTC()
	expiredAt := now.Add(m.ttl)
	tokenIDBytes := make([]byte, 16)
	if _, err := rand.Read(tokenIDBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("generate authentication token id: %w", err)
	}
	claims := Claims{
		UserID:            identity.UserID,
		Username:          identity.Username,
		Code:              identity.Code,
		SessionID:         identity.SessionID,
		CredentialVersion: identity.CredentialVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiredAt),
			ID:        base64.RawURLEncoding.EncodeToString(tokenIDBytes),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(identity.UserID, 10),
		},
	}
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(m.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign authentication token: %w", err)
	}
	return value, expiredAt, nil
}

func (m *Manager) Parse(value string) (Identity, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		strings.TrimSpace(value),
		claims,
		func(*jwt.Token) (any, error) { return m.key, nil },
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(m.issuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS512.Alg()}),
	)
	if err != nil || token == nil || !token.Valid || claims.CredentialVersion <= 0 || claims.UserID <= 0 || strings.TrimSpace(claims.ID) == "" ||
		strings.TrimSpace(claims.Username) == "" || strings.TrimSpace(claims.Code) == "" ||
		claims.Subject != strconv.FormatInt(claims.UserID, 10) {
		return Identity{}, ErrUnauthorized
	}
	return Identity{UserID: claims.UserID, Username: claims.Username, Code: claims.Code, SessionID: claims.SessionID, CredentialVersion: claims.CredentialVersion}, nil
}

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
			return nil, ErrUnauthorized
		}
	} else if m.sessions != nil && m.sessions.ValidateIdentity(r.Context(), identity) != nil {
		return nil, ErrUnauthorized
	}
	if identity.PasswordResetRequired && !PasswordResetAllowedHTTP(r.Method, r.URL.Path) && !(r.Method == http.MethodGet && r.URL.Path == "/auth/permission") {
		return nil, ErrPasswordResetRequired
	}
	return r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity)), nil
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}

// PasswordResetAllowedHTTP is deliberately independent of role/permission configuration.
func PasswordResetAllowedHTTP(method, path string) bool {
	return (method == http.MethodPatch && path == "/auth/reset/pwd") ||
		(method == http.MethodPost && (path == "/auth/challenge" || path == "/auth/logout" || path == "/auth/pub/logout" || path == "/auth/pub/refresh"))
}
