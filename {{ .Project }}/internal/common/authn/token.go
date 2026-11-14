package authn

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"strconv"
	"strings"
	"time"
)

type Claims struct {
	Methods           Methods `json:"authn"`
	UserID            int64   `json:"uid"`
	Username          string  `json:"username"`
	Code              string  `json:"code"`
	SessionID         string  `json:"sid,omitempty"`
	CredentialVersion int64   `json:"credential_version"`
	jwt.RegisteredClaims
}

func (m *Manager) Issue(identity Identity) (string, time.Time, error) {
	identity.Username = strings.TrimSpace(identity.Username)
	identity.Code = strings.TrimSpace(identity.Code)
	if !identity.Methods.Valid() || identity.CredentialVersion <= 0 || identity.UserID <= 0 || identity.Username == "" || identity.Code == "" || (m.sessions != nil && identity.SessionID == "") {
		return "", time.Time{}, ErrUnauthorized
	}
	now := time.Now()
	expiredAt := now.Add(m.ttl)
	tokenIDBytes := make([]byte, 16)
	if _, err := rand.Read(tokenIDBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("generate authentication token id: %w", err)
	}
	claims := Claims{
		Methods:           identity.Methods,
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
	if err != nil || token == nil || !token.Valid || !claims.Methods.Valid() || claims.CredentialVersion <= 0 || claims.UserID <= 0 || strings.TrimSpace(claims.ID) == "" ||
		strings.TrimSpace(claims.Username) == "" || strings.TrimSpace(claims.Code) == "" ||
		claims.Subject != strconv.FormatInt(claims.UserID, 10) {
		return Identity{}, ErrUnauthorized
	}
	return Identity{Methods: claims.Methods, UserID: claims.UserID, Username: claims.Username, Code: claims.Code, SessionID: claims.SessionID, CredentialVersion: claims.CredentialVersion}, nil
}
