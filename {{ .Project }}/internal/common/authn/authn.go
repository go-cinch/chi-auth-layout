package authn

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

var (
	ErrPasswordResetRequired = apperror.New("AUTH_PASSWORD_RESET_REQUIRED", "reset your password before continuing")
	ErrInvalidConfig         = errors.New("invalid authentication configuration")
	ErrUnauthorized          = errors.New("unauthorized")
)

type Manager struct {
	httpSecurityPolicy    func(*http.Request) error
	restrictedHTTPAllowed func(method, path string) bool
	issuer                string
	key                   []byte
	ttl                   time.Duration
	sessions              IdentityValidator
}

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
	manager := &Manager{issuer: issuer, key: []byte(key), ttl: ttl, sessions: sessions}
	if policy, ok := sessions.(interface{ AllowRestrictedHTTP(string, string) bool }); ok {
		manager.restrictedHTTPAllowed = policy.AllowRestrictedHTTP
	}
	return manager, nil
}
